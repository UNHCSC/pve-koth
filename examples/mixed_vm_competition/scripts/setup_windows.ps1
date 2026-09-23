$ErrorActionPreference = "Stop"

Set-ItemProperty -LiteralPath "HKLM:\System\CurrentControlSet\Control\Terminal Server" -Name "fDenyTSConnections" -Type DWord -Value 0
Enable-NetFirewallRule -DisplayGroup "Remote Desktop"
Set-Service -Name "TermService" -StartupType Automatic
Start-Service -Name "TermService"

$sshInstall = "C:\Program Files\OpenSSH"
if (-not (Get-Service -Name "sshd" -ErrorAction SilentlyContinue)) {
    $archive = Join-Path $env:TEMP "OpenSSH-Win64.zip"
    $extract = Join-Path $env:TEMP "pve-koth-openssh"
    Remove-Item -LiteralPath $archive, $extract -Recurse -Force -ErrorAction SilentlyContinue
    & curl.exe --location --fail --silent --show-error --output $archive "https://github.com/PowerShell/Win32-OpenSSH/releases/latest/download/OpenSSH-Win64.zip"
    if ($LASTEXITCODE -ne 0) { throw "OpenSSH download failed with exit code $LASTEXITCODE" }
    Expand-Archive -LiteralPath $archive -DestinationPath $extract -Force
    New-Item -ItemType Directory -Path $sshInstall -Force | Out-Null
    Copy-Item -Path (Join-Path $extract "OpenSSH-Win64\*") -Destination $sshInstall -Recurse -Force
    & (Join-Path $sshInstall "install-sshd.ps1")
    Remove-Item -LiteralPath $archive, $extract -Recurse -Force -ErrorAction SilentlyContinue
}

Set-Service -Name "sshd" -StartupType Automatic
Start-Service -Name "sshd"
& schtasks.exe /Create /TN "PVE KOTH Start OpenSSH" /SC ONSTART /DELAY 0000:15 /RU SYSTEM /RL HIGHEST /TR "sc.exe start sshd" /F | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Failed to create the OpenSSH startup task" }
if (-not (Get-NetFirewallRule -Name "OpenSSH-Server-In-TCP" -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -Name "OpenSSH-Server-In-TCP" -DisplayName "OpenSSH Server (sshd)" -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22 | Out-Null
} else {
    Enable-NetFirewallRule -Name "OpenSSH-Server-In-TCP"
}

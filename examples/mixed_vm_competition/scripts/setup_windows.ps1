$ErrorActionPreference = "Stop"

Set-ItemProperty -LiteralPath "HKLM:\System\CurrentControlSet\Control\Terminal Server" -Name "fDenyTSConnections" -Type DWord -Value 0
Enable-NetFirewallRule -DisplayGroup "Remote Desktop"
Set-Service -Name "TermService" -StartupType Automatic
Start-Service -Name "TermService"

$icmpRule = "PVE-KOTH-ICMPv4-Echo"
if (-not (Get-NetFirewallRule -Name $icmpRule -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -Name $icmpRule -DisplayName "PVE KOTH ICMPv4 Echo" -Enabled True -Direction Inbound -Protocol ICMPv4 -IcmpType 8 -Action Allow | Out-Null
} else {
    Enable-NetFirewallRule -Name $icmpRule
}

$sshInstall = "C:\Program Files\OpenSSH"
if (-not (Get-Service -Name "sshd" -ErrorAction SilentlyContinue)) {
    $archive = Join-Path $env:TEMP "OpenSSH-Win64.zip"
    $extract = Join-Path $env:TEMP "pve-koth-openssh"
    Remove-Item -LiteralPath $archive, $extract -Recurse -Force -ErrorAction SilentlyContinue
    & curl.exe --location --fail --silent --show-error --output $archive "https://github.com/PowerShell/Win32-OpenSSH/releases/latest/download/OpenSSH-Win64.zip"

    if ($LASTEXITCODE -ne 0) {
        throw "OpenSSH download failed with exit code $LASTEXITCODE"
    }

    Expand-Archive -LiteralPath $archive -DestinationPath $extract -Force
    New-Item -ItemType Directory -Path $sshInstall -Force | Out-Null
    $sshSource = Join-Path $extract "OpenSSH-Win64"
    $sessionSource = Join-Path $sshSource "sshd-session.exe"
    $sessionDestination = Join-Path $sshInstall "sshd-session.exe"
    for ($attempt = 1; $attempt -le 3; $attempt++) {
        Copy-Item -Path (Join-Path $sshSource "*") -Destination $sshInstall -Recurse -Force
        
        if ((Get-FileHash $sessionSource).Hash -eq (Get-FileHash $sessionDestination).Hash) {
            break
        }
        
        if ($attempt -eq 3) {
            throw "OpenSSH binary verification failed after $attempt attempts"
        }
    }

    & powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File (Join-Path $sshInstall "install-sshd.ps1")
    $installExitCode = $LASTEXITCODE
    if ($installExitCode -ne 0 -and -not (Get-Service -Name "sshd" -ErrorAction SilentlyContinue)) {
        throw "OpenSSH service installation failed with exit code $installExitCode"
    }

    Remove-Item -LiteralPath $archive, $extract -Recurse -Force -ErrorAction SilentlyContinue
}

Set-Service -Name "sshd" -StartupType Automatic
$sshd = Join-Path $sshInstall "sshd.exe"
$sshKeygen = Join-Path $sshInstall "ssh-keygen.exe"
& $sshd -t 2>$null

if ($LASTEXITCODE -ne 0) {
    Remove-Item -Path "C:\ProgramData\ssh\ssh_host_*_key*" -Force -ErrorAction SilentlyContinue
    & $sshKeygen -A
    if ($LASTEXITCODE -ne 0) {
        throw "OpenSSH host-key generation failed with exit code $LASTEXITCODE"
    }
}

& $sshd -t
if ($LASTEXITCODE -ne 0) {
    throw "OpenSSH configuration validation failed with exit code $LASTEXITCODE"
}

Start-Service -Name "sshd"
& schtasks.exe /Create /TN "PVE KOTH Start OpenSSH" /SC ONSTART /DELAY 0000:15 /RU SYSTEM /RL HIGHEST /TR "sc.exe start sshd" /F | Out-Null

if ($LASTEXITCODE -ne 0) {
    throw "Failed to create the OpenSSH startup task"
}

if (-not (Get-NetFirewallRule -Name "OpenSSH-Server-In-TCP" -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -Name "OpenSSH-Server-In-TCP" -DisplayName "OpenSSH Server (sshd)" -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22 | Out-Null
} else {
    Enable-NetFirewallRule -Name "OpenSSH-Server-In-TCP"
}

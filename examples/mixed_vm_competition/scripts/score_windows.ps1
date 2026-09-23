$results = [ordered]@{
    rdp_service = (Get-Service -Name "TermService" -ErrorAction SilentlyContinue).Status -eq "Running"
    rdp_listener = [bool](Get-NetTCPConnection -State Listen -LocalPort 3389 -ErrorAction SilentlyContinue)
    ssh_service = (Get-Service -Name "sshd" -ErrorAction SilentlyContinue).Status -eq "Running"
    ssh_listener = [bool](Get-NetTCPConnection -State Listen -LocalPort 22 -ErrorAction SilentlyContinue)
}

$results | ConvertTo-Json -Compress

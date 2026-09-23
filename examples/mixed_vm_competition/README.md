# Mixed Guest Example

This package provisions one Ubuntu LXC guest and one Windows 11 full VM per team. The Windows setup script enables Remote Desktop and installs and enables the official portable Win32-OpenSSH server. The Windows guest needs outbound HTTPS access to GitHub during setup. Setup and scoring execute through the Proxmox console for LXC and QEMU Guest Agent for Windows, so neither RDP nor SSH must work before setup starts.

The deployed `kothadmin` password is set to `ChangeMe!234` through QEMU Guest Agent. Change it before using this example outside an isolated lab. The server must allow `koth-template-windows-11-pro-n-workstation` under `vm_restrictions.templates`, and the configured LXC template and `team` storage pool must also be allowed.

# Preparing QEMU VM Templates

PVE KOTH creates full virtual machines by cloning prepared Proxmox VE QEMU templates. Create one template for each operating system and edition that a competition may request, such as Fedora Server, Ubuntu Desktop, Windows 11, or Windows Server.

Keep templates deliberately small. A template should provide a bootable, clone-safe operating system, working VirtIO devices, a working QEMU Guest Agent, and a temporary local administrator credential. Competition setup scripts are responsible for installing and configuring challenge software.

PVE KOTH uses QEMU Guest Agent as its bootstrap and command-execution channel. SSH, WinRM, cloud-init, Cloudbase-Init, and a working guest network are not required for the first bootstrap command.

The Windows-specific instructions and links were last checked on September 22, 2026.

## Template contract

Every template must meet these requirements:

- The VM boots without installation media attached.
- The QEMU Guest Agent is installed, enabled, and starts automatically.
- The guest-agent option is enabled in the Proxmox VM configuration.
- The boot disk and network adapter use drivers installed in the guest.
- The initial network configuration uses DHCP or has no address that can conflict with another clone.
- A local root or administrator credential is known to the operator. Do not enable remote root login merely for PVE KOTH.
- The guest has Bash on Linux or Windows PowerShell on Windows.
- The OS does not automatically suspend or hibernate while a competition is running.
- The installation is generalized so clones do not retain machine-specific identity.
- The VM is fully shut down before it is converted to a Proxmox template.

The initial credential is temporary. PVE KOTH should rotate it during clone bootstrap before running competition setup scripts. Never reuse a production password in a template or include one in this repository.

## Create the VM in Proxmox

Create a normal QEMU VM in the Proxmox web interface. The exact CPU, memory, and disk sizes are not important because competition configuration can override them, but the template disk must not be larger than the smallest disk that a competition may request. Proxmox can grow a cloned disk but cannot safely shrink it.

Recommended common settings:

- Use a unique VMID reserved for templates.
- Use `VirtIO SCSI single` as the SCSI controller.
- Put the boot disk on `SCSI0` and enable discard and IO thread when supported by the selected storage.
- Use a VirtIO network adapter attached to the competition bridge.
- Enable **Options > QEMU Guest Agent**.
- Disable **Start at boot** for the template.
- Do not add PCI passthrough or node-local devices unless every target node has an equivalent resource mapping.

Install and update the operating system before doing the operating-system specific cleanup below.

## Linux templates

### Install the guest agent

On Debian or Ubuntu:

```bash
sudo apt update
sudo apt install -y qemu-guest-agent
sudo systemctl enable --now qemu-guest-agent
```

On Fedora, CentOS Stream, or RHEL-compatible systems:

```bash
sudo dnf install -y qemu-guest-agent
sudo systemctl enable --now qemu-guest-agent
```

Confirm that the service is enabled and running:

```bash
systemctl is-enabled qemu-guest-agent
systemctl is-active qemu-guest-agent
```

From a Proxmox node, verify host-to-guest communication while the VM is running:

```bash
qm agent <vmid> ping
qm guest exec <vmid> -- /bin/sh -c 'id && uname -a'
```

Both commands must succeed. A successful network ping to the VM is not a substitute for this test.

### Prepare networking

Leave the template NIC on DHCP. PVE KOTH will use the guest agent to install the competition hostname, static address, gateway, and DNS settings after cloning.

Record which network configuration system the template uses:

- Fedora and most current Linux desktops normally use NetworkManager.
- Ubuntu Server commonly uses Netplan backed by `systemd-networkd`.
- Minimal distributions may use `systemd-networkd` directly.

The guest specification must select the matching PVE KOTH network configurator. Do not leave a hard-coded address, interface MAC address, or interface-specific rule in the template.

### Set the temporary administrator credential

Set a temporary root password if the distribution enables the root account:

```bash
sudo passwd root
```

On distributions such as Ubuntu, it is also acceptable to keep root locked and provide a temporary local user with passwordless or password-based `sudo`. QEMU Guest Agent commands do not authenticate with this password, so remote root login and an SSH server are not required.

### Prevent automatic sleep

Desktop editions should not suspend while they are being scored:

```bash
sudo systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target
```

Also disable desktop-environment sleep policies if they override systemd.

### Generalize the installation

Apply all desired operating-system updates before generalizing the image. Then remove identity and transient state:

```bash
sudo rm -f /etc/ssh/ssh_host_*
sudo truncate -s 0 /etc/machine-id
sudo rm -f /var/lib/dbus/machine-id
sudo ln -s /etc/machine-id /var/lib/dbus/machine-id
sudo rm -rf /tmp/* /var/tmp/*
sudo journalctl --rotate
sudo journalctl --vacuum-time=1s
sudo fstrim -av
```

If OpenSSH is installed, verify that the distribution regenerates missing host keys during boot. If it does not, enable the distribution's host-key generation unit or add a one-shot unit that runs `ssh-keygen -A` before `sshd`.

Clearing `/etc/machine-id` must be the final in-guest operation. Shut down without rebooting so the template retains an empty machine ID:

```bash
sudo poweroff
```

## Windows templates

### Licensing and Microsoft accounts

The default PVE KOTH template workflow does not embed a Windows product key and does not associate the template with a Microsoft account.

During Windows Setup, select **I don't have a product key** when prompted. This only skips entering a key; it does not grant a Windows license or activate the installation. Microsoft states that a digital license or product key is needed for activation. Each operator remains responsible for ensuring that every deployed VM is used under appropriate Microsoft license or evaluation terms. See [Activate Windows](https://support.microsoft.com/en-us/windows/activation/activate-windows). There is no implied "license-free Windows" mode in PVE KOTH; the default simply keeps activation credentials out of the reusable template.

For time-limited testing, Microsoft also publishes a [Windows 11 Enterprise evaluation](https://www.microsoft.com/en-us/evalcenter/evaluate-windows-11-enterprise). Its current download page specifies a 90-day evaluation and lists Microsoft account sign-in as a prerequisite. It therefore does not automatically satisfy the default no-Microsoft-account policy described here. Read its current terms and prerequisites and make that choice deliberately; evaluation media and terms can change independently of PVE KOTH.

An operator who wants activated guests may instead:

- Enter a valid key during installation.
- Activate the reference installation before generalization when the license permits cloning.
- Run an organization-approved KMS, MAK, subscription activation, or other activation process from a competition setup script.

Do not publish keys in a competition archive or commit them to source control. A key or digital entitlement for the reference VM does not automatically grant rights for every clone.

Likewise, do not sign the reusable template into a personal Microsoft account. Operators who require a Microsoft account can enroll each deployed guest after cloning or provide their own organization-specific unattended setup. PVE KOTH itself requires only a temporary local administrator account. Microsoft documents the post-deployment option under [Change from a local account to a Microsoft account](https://support.microsoft.com/en-us/accounts-billing/manage/change-from-a-local-account-to-a-microsoft-account-in-windows).

### Configure Windows VM hardware

For Windows 11, use hardware that satisfies Windows 11 requirements:

- Set the guest OS type to Microsoft Windows 11/2022.
- Use Q35 with OVMF (UEFI).
- Add an EFI disk.
- Add a TPM 2.0 device on suitable storage.
- Use a VirtIO SCSI boot disk and VirtIO network adapter.
- Attach both the Windows installation ISO and a current VirtIO-Win ISO.
- Enable the Proxmox QEMU Guest Agent option.

Use supported hardware rather than bypassing Windows hardware checks. The [Proxmox administration guide](https://pve.proxmox.com/pve-docs/pve-admin-guide.pdf) documents QEMU VM templates, VirtIO devices, and QEMU Guest Agent behavior.

### Install without an online account

Windows OOBE behavior changes between releases. Do not rely on undocumented commands such as `OOBE\\BYPASSNRO`, registry edits, or URI-launch shortcuts. Use Microsoft's supported Audit Mode and unattended-setup mechanisms instead.

1. Install Windows from official installation media. Microsoft publishes the current multi-edition ISO on its [Download Windows 11](https://www.microsoft.com/en-us/software-download/windows11) page.
2. Select **I don't have a product key** unless this template should be activated.
3. Select the required Windows edition carefully. Activation is edition specific.
4. Load the VirtIO SCSI driver from the VirtIO-Win ISO when Windows Setup asks for an installation disk. The directory is normally under `vioscsi\\w11\\amd64` for Windows 11.
5. At the first OOBE screen, press `Ctrl+Shift+F3` to enter Audit Mode. Windows restarts and signs in using the built-in Administrator account without creating a Microsoft account. Microsoft documents this workflow in [Boot Windows to Audit Mode or OOBE](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/boot-windows-to-audit-mode-or-oobe?view=windows-11).

In Audit Mode, create `C:\Windows\Panther\Unattend\koth-unattend.xml` with an answer file like the following. Replace `REPLACE_WITH_TEMPORARY_PASSWORD` with the template's temporary password before using it.

```xml
<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend">
  <settings pass="oobeSystem">
    <component name="Microsoft-Windows-International-Core"
               processorArchitecture="amd64"
               publicKeyToken="31bf3856ad364e35"
               language="neutral"
               versionScope="nonSxS"
               xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State"
               xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <InputLocale>en-US</InputLocale>
      <SystemLocale>en-US</SystemLocale>
      <UILanguage>en-US</UILanguage>
      <UserLocale>en-US</UserLocale>
    </component>
    <component name="Microsoft-Windows-Shell-Setup"
               processorArchitecture="amd64"
               publicKeyToken="31bf3856ad364e35"
               language="neutral"
               versionScope="nonSxS"
               xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State"
               xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <OOBE>
        <HideEULAPage>true</HideEULAPage>
        <HideOEMRegistrationScreen>true</HideOEMRegistrationScreen>
        <HideOnlineAccountScreens>true</HideOnlineAccountScreens>
        <HideWirelessSetupInOOBE>true</HideWirelessSetupInOOBE>
        <ProtectYourPC>3</ProtectYourPC>
      </OOBE>
      <UserAccounts>
        <LocalAccounts>
          <LocalAccount wcm:action="add">
            <Name>kothadmin</Name>
            <DisplayName>KOTH Administrator</DisplayName>
            <Description>Temporary PVE KOTH bootstrap account</Description>
            <Group>Administrators</Group>
            <Password>
              <Value>REPLACE_WITH_TEMPORARY_PASSWORD</Value>
              <PlainText>true</PlainText>
            </Password>
          </LocalAccount>
        </LocalAccounts>
      </UserAccounts>
    </component>
  </settings>
</unattend>
```

Microsoft documents `HideEULAPage` for suppressing the license-terms page during OOBE, `HideOnlineAccountScreens` for deployments that should not require an email-address sign-in, and `UserAccounts` for creating local users during `oobeSystem`. Microsoft limits OEM and system-builder use of `HideEULAPage` to testing before shipment; this guide uses it for disposable competition lab VMs, not devices shipped to customers. Hiding the page does not waive the license terms or make an unlicensed installation licensed:

- [HideEULAPage](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/microsoft-windows-shell-setup-oobe-hideeulapage)
- [HideOnlineAccountScreens](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/microsoft-windows-shell-setup-oobe-hideonlineaccountscreens)
- [Automate OOBE](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/automate-oobe)
- [Local account groups](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/microsoft-windows-shell-setup-useraccounts-localaccounts-localaccount-group)

The answer file contains a plaintext bootstrap password. Restrict access to the reference VM and Proxmox storage, use a unique template password, and have PVE KOTH rotate it immediately after cloning.

### Install VirtIO drivers and QEMU Guest Agent

While still in Audit Mode, run the VirtIO-Win guest tools installer from the attached VirtIO-Win ISO. The installer filename is normally `virtio-win-guest-tools.exe`. It installs the paravirtualized drivers needed by the VM. Current installation guidance and the current ISO location are maintained by the [VirtIO-Win project](https://virtio-win.github.io/Knowledge-Base/Driver-installation.html).

Explicitly add and install the VirtIO serial driver from an elevated PowerShell or Command Prompt. QEMU Guest Agent uses this device to communicate with Proxmox. For Windows 11 with the VirtIO-Win ISO mounted as `D:`, run:

```powershell
pnputil.exe /add-driver D:\vioserial\w11\amd64\*.inf /install
```

Use the actual VirtIO-Win drive letter and the directory matching the guest OS if they differ. Confirm that `pnputil` reports the driver package as added or already present. Microsoft documents the command and `/install` behavior in the [PnPUtil command syntax](https://learn.microsoft.com/en-us/windows-hardware/drivers/devtest/pnputil-command-syntax).

Install the 64-bit QEMU Guest Agent separately if the guest-tools installer did not install it. The MSI is normally located at:

```text
<virtio-drive>:\guest-agent\qemu-ga-x86_64.msi
```

From an elevated PowerShell window, confirm that the service exists, starts, and uses automatic startup:

```powershell
$service = Get-Service -Name "QEMU-GA"
Set-Service -Name $service.Name -StartupType Automatic
Start-Service -Name $service.Name
Get-Service -Name $service.Name
```

If `QEMU-GA` is not found, locate the installed name with:

```powershell
Get-Service | Where-Object {
    $_.Name -match "qemu" -or $_.DisplayName -match "qemu"
}
```

From the Proxmox host, verify the agent and privileged command execution:

```bash
qm agent <vmid> ping
qm guest exec <vmid> -- powershell.exe -NoProfile -NonInteractive -Command "whoami"
```

### Finish Windows preparation

Install Windows updates and reboot as needed. Avoid installing applications from Microsoft Store into only the Audit Mode user profile; Microsoft notes that per-user Store applications can prevent Sysprep from generalizing an image.

Disable sleep and hibernation so a desktop template remains available during a competition:

```powershell
powercfg.exe /hibernate off
powercfg.exe /change standby-timeout-ac 0
powercfg.exe /change monitor-timeout-ac 0
```

Leave the VirtIO network adapter on DHCP. Do not save a competition IP address, gateway, DNS server, or team hostname in the template.

Turn off BitLocker on the OS volume before running Sysprep. Suspending protection is not sufficient; wait for decryption to finish:

```powershell
manage-bde.exe -off C:
manage-bde.exe -status C:
```

Rerun the status command until `Conversion Status` is `Fully Decrypted` and `Percentage Encrypted` is `0.0%`. Sysprep can fail while the OS volume remains encrypted. Microsoft documents that [`manage-bde -off`](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/manage-bde-off) decrypts the volume and removes its key protectors when complete.

When preparation, agent tests, and decryption are complete, open an elevated Command Prompt and generalize the VM. Use the absolute Windows path rather than relying on environment-variable expansion:

```bat
C:\Windows\System32\Sysprep\Sysprep.exe /generalize /oobe /mode:vm /shutdown /unattend:C:\Windows\Panther\Unattend\koth-unattend.xml
```

Microsoft requires `/generalize` when a Windows image will be copied to another machine. `/mode:vm` is appropriate when the image will be redeployed to the same hypervisor and virtual hardware profile. See [Sysprep command-line options](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/sysprep-command-line-options?view=windows-11).

Do not start the reference VM after Sysprep shuts it down. Starting it consumes the generalized first boot and requires another Sysprep pass before conversion.

## Convert and validate the template

After the guest has shut down:

1. Remove the operating-system and VirtIO installation ISOs.
2. Confirm that the boot disk is first in the boot order.
3. Confirm that QEMU Guest Agent remains enabled in Proxmox.
4. Add a description recording the OS edition, release, architecture, network configurator, temporary administrator name, and preparation date. Do not record the password in the description.
5. Convert the stopped VM to a Proxmox template.

The equivalent final command is:

```bash
qm template <vmid>
```

Before making the template available to competition authors, create a test clone and verify all of the following:

- The clone starts on every eligible Proxmox node.
- `qm agent <clone-vmid> ping` succeeds before guest networking is configured.
- A command executed through QEMU Guest Agent runs as root or SYSTEM.
- The clone receives a new Linux machine ID or Windows SID.
- No static IP address is inherited from the reference VM.
- The expected network configurator can set an address, gateway, and DNS.
- The temporary local credential works and can be changed.
- The VM can shut down, start again, and reconnect to the guest agent.
- Linux SSH host keys are unique when SSH is installed.
- Windows may restart more than once and check for updates while completing OOBE. Allow the test clone to finish without intervention and verify that QEMU Guest Agent reconnects after each restart.
- Windows completes unattended OOBE using the local `kothadmin` account and does not request a Microsoft account, license acceptance, or other interactive input.

Delete the test clone after validation. Keep the template powered off and treat updates as a rebuild cycle: clone or temporarily convert it to a VM, update it, repeat generalization, validate another clone, and then publish the new template VMID or version.

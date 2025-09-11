# Proxmox VE King of the Hill

This project is based off of [the old proxmox king of the hill](https://github.com/UNHCSC/proxmox-koth/). I learned a lot of lessons of what to expect when dealing with the Proxmox API, and also just storing and managing all of this data in general.

So here are some of my **goals**:
1. Streamline the API and web app with frameworks
2. Rely on web app for administration
3. Allow for multiple competitions at once
4. Allow for each team to own multiple containers
5. LDAP Logins for the web app, group control (admins, viewers)

Adding ports on `firewall-cmd`:
```bash
firewall-cmd --add-port=8006/tcp --permanent
firewall-cmd --add-port=5000/tcp --permanent
firewall-cmd --reload
```
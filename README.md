# Proxmox VE King of the Hill

![Tests](https://img.shields.io/github/actions/workflow/status/UNHCSC/pve-koth/koth.yml?branch=main&label=Tests&job=run-tests)
![Build](https://img.shields.io/github/actions/workflow/status/UNHCSC/pve-koth/koth.yml?branch=main&label=Build&job=build)
![Release](https://img.shields.io/github/actions/workflow/status/UNHCSC/pve-koth/koth.yml?branch=main&label=Release&job=release)


This project modernizes the original [proxmox KOTH](https://github.com/UNHCSC/proxmox-koth/) tooling with a Go/Fiber backend, Tailwind-powered dashboard, and stream jobs for long-running operations like redeploys and teardowns.

## Requirements

- Go 1.20+ (or later) for the server binaries.
- Node.js 20+ / npm for building the dashboard assets.
- A valid `config.toml` next to the repository root to describe the database, Proxmox, and LDAP settings.
- A service user on your Proxmox cluster set up with `VM.Audit, VM.Console` permissions on Proxmox. (You can create a custom role with these permissions and assign only that role to the service user for better security.)
- Container setup and scoring are performed via the Proxmox console (raw exec), not SSH. If your setup scripts rely on SSH access, be sure to install and enable an OpenSSH server inside the container.
- Some container templates do not ship an SSH server; add `openssh-server` (or your distro equivalent) in your setup scripts if you require SSH.

## Installation

This handy-dandy script can install Proxmox VE KotH on a Fedora-based system. There are more options/ways to run the script.

The option `-u` will install the latest release version. You can pass `-r` to see all releases, and `-i <release_tag>` to install a specific version.

You can run `-p` to uninstall Proxmox VE KotH from your system.

```bash
bash -i <(curl -sSL https://raw.githubusercontent.com/UNHCSC/pve-koth/main/.github/koth_installer.sh) -u
```

## Build

- `npm install`
- `npm run build` (or `npm run devel` while developing frontend assets)
- `go build ./...` to compile the server

## Testing

- `go test ./...`

## Documentation

See the `docs/` folder for architecture overviews, user guides, and competition creation tutorials.

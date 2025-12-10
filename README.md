# Proxmox VE King of the Hill

![Tests](https://img.shields.io/github/actions/workflow/status/UNHCSC/pve-koth/koth.yml?branch=main&label=Tests&job=run-tests)
![Build](https://img.shields.io/github/actions/workflow/status/UNHCSC/pve-koth/koth.yml?branch=main&label=Build&job=build)
![Release](https://img.shields.io/github/actions/workflow/status/UNHCSC/pve-koth/koth.yml?branch=main&label=Release&job=release)


This project modernizes the original [proxmox KOTH](https://github.com/UNHCSC/proxmox-koth/) tooling with a Go/Fiber backend, Tailwind-powered dashboard, and stream jobs for long-running operations like redeploys and teardowns.

## Requirements

- Go 1.20+ (or later) for the server binaries.
- Node.js 20+ / npm for building the dashboard assets.
- A valid `config.toml` next to the repository root to describe the database, Proxmox, and LDAP settings.

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

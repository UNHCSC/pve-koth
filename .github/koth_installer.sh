#!/bin/bash

# https://raw.githubusercontent.com/UNHCSC/pve-koth/main/.github/koth_installer.sh
# This script downloads and installs the Proxmox VE KotH software to a supported Linux system.

GITHUB_REPOSITORY="UNHCSC/pve-koth"
RELEASES_URL="https://api.github.com/repos/${GITHUB_REPOSITORY}/releases"

set -e
set -o pipefail

# Function to confirm a value, returns the value confirmed (aka, yes/no, if no ask for new value)
util_confirm_value() {
    local prompt_message="$1"
    local current_value="$2"
    local user_input

    while true; do
        read -rp "${prompt_message} [${current_value}]: " user_input
        user_input="${user_input:-$current_value}"

        read -rp "You entered '${user_input}'. Is this correct? (y/n): " confirmation
        case $confirmation in
            [Yy]* ) echo "$user_input"; return ;;
            [Nn]* ) echo "Let's try again." ;;
            * ) echo "Please answer Y/y or N/n." ;;
        esac
    done
}

# Function to ensure prerequisites are met
ensure_prereqs() {
    local prereqs=(jq wget nano tar)

    for cmd in "${prereqs[@]}"; do
        if ! command -v "$cmd" &> /dev/null; then
            echo "Error: $cmd is not installed. Please install it and try again."
            exit 1
        fi
    done
}

# Function to display usage information
usage() {
    echo "Usage: $0 [options]"
    echo "Options:"
    echo "  -h, --help          Show this help message and exit"
    echo "  -r, --releases      List available releases"
    echo "  -i, --install <tag>  Install specified release tag"
    echo "  -u, --update        Update to the latest release. Can also be used to install to the latest version."
    echo "  -p, --purge         Uninstall Proxmox VE KotH from the system"
    exit 0
}

# Function to display available releases (Format: #, Tag, Date)
list_releases() {
    echo "Available releases for ${GITHUB_REPOSITORY}:"
    
    wget -q -O - "${RELEASES_URL}" | jq -r '.[] | "\(.tag_name) \(.published_at)"' | nl -w2 -s'. '

    exit 0
}

# Function to install a specified release
install_release() {
    local release_tag="$1"
    local download_url=$(wget -q -O - "${RELEASES_URL}/tags/${release_tag}" | jq -r '.assets[0].browser_download_url')

    local install_dir=$(util_confirm_value "Enter installation directory" "/opt/pve-koth")
    local koth_user="pve-koth"

    # Create installation directory if it doesn't exist
    sudo mkdir -p "$install_dir"
    sudo chown "$(whoami)":"$(whoami)" "$install_dir"

    # Download and extract the release
    wget -q -O - "$download_url" | tar -xz -C "$install_dir"

    # Create a dedicated user for KotH (if not exists)
    if ! id -u "$koth_user" &> /dev/null; then
        sudo useradd -r -s /bin/false "$koth_user"
    fi

    # Set ownership of installation directory
    sudo chown -R "$koth_user":"$koth_user" "$install_dir"

    # The binaries should be given the necessary capabilities
    for bin in "$install_dir"/*; do
        if [ -x "$bin" ] && [ ! -d "$bin" ]; then
            echo "Setting capabilities for $bin"
            sudo setcap 'cap_net_bind_service=+ep' "$bin"
        fi
    done

    # Create a systemd service file
    local service_file="/etc/systemd/system/pve-koth.service"
    sudo bash -c "cat > $service_file" <<EOL
[Unit]
Description=Proxmox VE KotH Service
After=network.target

[Service]
Type=simple
User=${koth_user}
ExecStart=${install_dir}/pve-koth
WorkingDirectory=${install_dir}
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOL

    # Reload systemd, enable and start the service
    sudo systemctl daemon-reload

    # The first run of the pve-koth binary makes the config.toml file if it doesn't exist. So we check if config.toml exists, if not we run the binary once to create it.
    if [ ! -f "${install_dir}/config.toml" ]; then
        echo "Creating initial config.toml file..."
        # This will error out, it shouldn't crash the script though. We just want to create the config.toml file. Make sure not to let it output to the user.
        sudo su - "$koth_user" -s /bin/bash -c "cd ${install_dir} && ./pve-koth" &> /dev/null || true
    fi

    # Ask to configure the config.toml file (y/n)
    read -rp "Would you like to configure the config.toml file now? (y/n) (If this is an initial setup, this is highly recommended!): " configure_env
    if [[ "$configure_env" =~ ^[Yy]$ ]]; then
        sudo nano "${install_dir}/config.toml"
    fi

    sudo systemctl enable --now pve-koth.service

    echo "Proxmox VE KotH version ${release_tag} installed successfully in ${install_dir}."

    exit 0
}

# Function to uninstall Proxmox VE KotH
uninstall_koth() {
    local install_dir=$(util_confirm_value "Enter installation directory to remove" "/opt/pve-koth")
    local koth_user="pve-koth"

    # Stop and disable the service
    sudo systemctl stop pve-koth.service || true
    sudo systemctl disable pve-koth.service || true

    # Remove systemd service file
    sudo rm -f /etc/systemd/system/pve-koth.service
    sudo systemctl daemon-reload

    # Remove installation directory
    sudo rm -rf "$install_dir"

    # Remove dedicated user
    sudo userdel "$koth_user" || true

    # Remove capabilities
    sudo setcap -r "${install_dir}/pve-koth" || true
    
    echo "Proxmox VE KotH uninstalled successfully from ${install_dir}."
    
    exit 0
}

# Main script execution starts here
ensure_prereqs

# Parse command-line arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        -h|--help) usage ;;
        -r|--releases) list_releases ;;
        -i|--install) 
            if [[ -n "$2" ]]; then
                install_release "$2"
                shift
            else
                echo "Error: --install requires a release tag argument."
                usage
            fi
            ;;
        -u|--update)
            latest_tag=$(wget -q -O - "${RELEASES_URL}" | jq -r '.[0].tag_name')
            echo "Updating to the latest release: ${latest_tag}"
            install_release "$latest_tag"
            ;;
        -p|--purge) uninstall_koth ;;
        *) echo "Unknown parameter passed: $1"; usage ;;
    esac
    shift
done

usage

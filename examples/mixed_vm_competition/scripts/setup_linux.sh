#!/bin/bash
set -euo pipefail
install -d -m 0755 /opt/pve-koth
printf 'Linux LXC is ready\n' > /opt/pve-koth/setup-complete

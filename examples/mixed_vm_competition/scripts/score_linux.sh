#!/bin/bash

setup_marker=false
bash_available=false
test "$(cat /opt/pve-koth/setup-complete 2>/dev/null)" = "Linux LXC is ready" && setup_marker=true
command -v bash >/dev/null 2>&1 && bash_available=true
printf '{"setup_marker":%s,"bash":%s}\n' "$setup_marker" "$bash_available"

#!/bin/bash

# Update and Upgrade
apt-get update && apt-get upgrade -y

# Install Packages
apt-get install -y curl wget nano

# Install Prometheus Node Exporter
wget https://github.com/prometheus/node_exporter/releases/download/v1.9.1/node_exporter-1.9.1.linux-amd64.tar.gz -O /tmp/node_exporter.tar.gz
tar -xzf /tmp/node_exporter.tar.gz -C /opt
mv /opt/node_exporter-1.9.1.linux-amd64 /opt/node_exporter

# Create systemd service for Node Exporter
cat <<EOF > /etc/systemd/system/node_exporter.service
[Unit]
Description=Prometheus Node Exporter
After=network.target

[Service]
User=root
Type=simple
WorkingDirectory=/opt/node_exporter
ExecStart=bash -c "/opt/node_exporter/node_exporter"
Restart=always

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now node_exporter

# Team's User
useradd -m -s /bin/bash koth
echo "koth:password" | chpasswd
usermod -aG sudo koth
echo "koth ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers

# Other Users
newUsers=("Sylvia Schneider" "Zack Chan" "Katy Rivas" "Amie Freeman" "Nikita Willis" "Demi-Leigh Rocha" "Nate Alexander" "Amelie Bright" "Angus Larson" "Laila Lyons" "Paul Fitzgerald" "Violet Nolan" "Jada Terry" "Armaan Huffman" "Moshe Thornton" "Lorcan Pham" "Tomas O'Quinn" "Kajus Miranda" "Millie Bridges" "Ben Allen" "Vivian Cantu" "Lyndon Massey")

for user in "${newUsers[@]}"; do
    username=$(echo "$user" | tr '[:upper:]' '[:lower:]' | tr ' ' '.')
    if [[ ! " ${USERS[*]} " =~ " ${username} " ]]; then
        USERS+=("$username")
        useradd -m -s /bin/bash "$username"
        chfn -f "$user" "$username"
        echo "$username:password" | chpasswd
        echo "$username ALL=(ALL) NOPASSWD: ALL" >>/etc/sudoers
    fi
done
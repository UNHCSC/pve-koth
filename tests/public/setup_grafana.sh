#!/bin/bash

# Install specific packages
apt-get install -y python3 python3-pip

# Install Prometheus
wget https://github.com/prometheus/prometheus/releases/download/v2.53.4/prometheus-2.53.4.linux-amd64.tar.gz -O /tmp/prometheus.tar.gz
tar -xzf /tmp/prometheus.tar.gz -C /opt
mv /opt/prometheus-2.53.4.linux-amd64 /opt/prometheus

# For each of the IPs in the comma separated list (KOTH_CONTAINER_IPS), add it to the target
TARGETS_STR="      - targets: ["
IFS=',' read -ra ADDR <<< "$KOTH_CONTAINER_IPS"
for ip in "${ADDR[@]}"; do
    TARGETS_STR+="\"$ip:9100\", "
done
TARGETS_STR=${TARGETS_STR%, }  # Remove trailing comma and space
TARGETS_STR+="]"

# Replace the targets line in prometheus.yml
sed -i "/- targets: \[/c\\$TARGETS_STR" /opt/prometheus/prometheus.yml

# Create systemd service for Prometheus
cat <<EOF > /etc/systemd/system/prometheus.service
[Unit]
Description=Prometheus Monitoring System
After=network.target

[Service]
User=root
Type=simple
WorkingDirectory=/opt/prometheus
ExecStart=/opt/prometheus/prometheus --config.file=/opt/prometheus/prometheus.yml
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# Enable Prometheus
systemctl daemon-reload
systemctl enable --now prometheus

# Grafana configurations
GRAFANA_URL="http://localhost:3000"
GRAFANA_USER="admin"
GRAFANA_PASS="admin"
DASHBOARD_ID="1860"
DATASOURCE_NAME="Prometheus"

# Install Grafana
mkdir -p /etc/apt/keyrings/
wget -q -O - https://apt.grafana.com/gpg.key | gpg --dearmor | tee /etc/apt/keyrings/grafana.gpg >/dev/null
echo "deb [signed-by=/etc/apt/keyrings/grafana.gpg] https://apt.grafana.com stable main" | tee -a /etc/apt/sources.list.d/grafana.list
apt-get update && apt-get install -y grafana jq

# Install Python requests library for the python install offload
pip install requests --break-system-packages

# Enable Grafana
systemctl daemon-reload
systemctl enable --now grafana-server


# Offload to Python because bash is not great for JSON
python3 <<EOF
import requests, time, json

grafana_url = "$GRAFANA_URL"
auth = ("$GRAFANA_USER", "$GRAFANA_PASS")
headers = {"Content-Type": "application/json"}

# Wait for Grafana to be up
while True:
    try:
        r = requests.get(f"{grafana_url}/api/health", auth=auth)
        if r.status_code == 200 and r.json().get("database") == "ok":
            print("[+] Grafana is up and running!")
            break
        else:
            print("[*] Waiting for Grafana...")
    except Exception as e:
        print(f"[*] Grafana not ready: {e}")
    time.sleep(5)

# Create Prometheus datasource
print("[*] Creating Prometheus datasource...")
data = {
    "name": "$DATASOURCE_NAME",
    "type": "prometheus",
    "access": "proxy",
    "url": "http://localhost:9090",
    "isDefault": True
}
resp = requests.post(f"{grafana_url}/api/datasources", auth=auth, headers=headers, data=json.dumps(data))
resp.raise_for_status()
uid = resp.json()["datasource"]["uid"]
print(f"[+] Prometheus datasource created (UID: {uid})")

# Download dashboard JSON from grafana.com
print("[*] Downloading dashboard JSON...")
dashboard_resp = requests.get(f"{grafana_url}/api/gnet/dashboards/$DASHBOARD_ID", auth=auth)
if dashboard_resp.status_code != 200:
    print(f"[-] Failed to download dashboard JSON: {dashboard_resp.status_code}")
    exit(1)
dashboard_resp.raise_for_status()
dashboard_json = dashboard_resp.json()


# Prepare import payload
print("[*] Importing dashboard...")
import_payload = {
    "dashboard": dashboard_json["json"],
    "overwrite": True,
    "inputs": [
        {
            "name": "DS_PROMETHEUS",
            "type": "datasource",
            "pluginId": "prometheus",
            "value": uid
        }
    ],
    "folderUid": ""
}

import_resp = requests.post(f"{grafana_url}/api/dashboards/import", auth=auth, headers=headers, data=json.dumps(import_payload))
import_resp.raise_for_status()
print("[✓] Dashboard imported successfully!")
EOF
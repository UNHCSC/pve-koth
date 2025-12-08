#!/bin/bash

CHECK_icmp=false
CHECK_exporter=false

# Container Can be Pinged? +1, -0
if ping -c 1 -W 1 "$KOTH_IP" &> /dev/null; then
    CHECK_icmp=true
fi

# Prometheus Exporter Running? +1, -1
if curl -s --max-time 10 "http://$KOTH_IP:9100/metrics" | grep -q "Processor"; then
    CHECK_exporter=true
fi

# Now give the data out as JSON
echo "{
    \"icmp\": $CHECK_icmp,
    \"exporter\": $CHECK_exporter
}"
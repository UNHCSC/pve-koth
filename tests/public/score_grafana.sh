#!/bin/bash

SCORE=0
CHECK_grafana=false
CHECK_prometheus=false

# Grafana Running? +3, -3
if curl -s --max-time 2 "http://$KOTH_IP:3000/login" | grep -q "Grafana"; then
    CHECK_grafana=true
    SCORE=$((SCORE + 3))
else
    SCORE=$((SCORE - 3))
fi

# Prometheus Running? +3, -3
if curl -s --max-time 2 "http://$KOTH_IP:9090/graph" | grep -q "Prometheus Time Series Collection and Processing Server"; then
    CHECK_prometheus=true
    SCORE=$((SCORE + 3))
else
    SCORE=$((SCORE - 3))
fi

# Now give the data out as JSON
echo "{
    \"score\": $SCORE,
    \"grafana\": $CHECK_grafana,
    \"prometheus\": $CHECK_prometheus
}"
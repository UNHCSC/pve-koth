#!/bin/bash

CHECK_icmp=false
CHECK_exporter=false
CHECK_requiredUsers=false
CHECK_usersBonus=false

# Container Can be Pinged? +1, -0
if ping -c 1 -W 1 "$KOTH_IP" &> /dev/null; then
    CHECK_icmp=true
fi

# Prometheus Exporter Running? +1, -1
if curl -s --max-time 2 "http://$KOTH_IP:9100/metrics" | grep -q "Processor"; then
    CHECK_exporter=true
fi

# Get a list of all human users on the system
USERS=()
while IFS=: read -r username _ uid _ gecos _; do
    if [[ $uid -ge 1000 && $username != "koth" ]]; then
        USERS+=("$username")
    fi
done < /etc/passwd

CHECK_requiredUsers=true
CHECK_usersBonus=true
AUTHORIZED_USERS=("Sylvia Schneider" "Zack Chan" "Katy Rivas" "Amie Freeman" "Amelie Bright" "Angus Larson" "Laila Lyons" "Paul Fitzgerald" "Violet Nolan" "Jada Terry" "Tomas O'Quinn" "Kajus Miranda" "Millie Bridges" "Vivian Cantu")
AUTHORIZED_SUDO_USERS=("Sylvia Schneider" "Katy Rivas" "Angus Larson")
for user in "${AUTHORIZED_USERS[@]}"; do
    username=$(echo "$user" | tr '[:upper:]' '[:lower:]' | tr ' ' '.')
    
    if [[ ! " ${USERS[*]} " =~ " ${username} " ]]; then
        CHECK_requiredUsers=false
    else 
        if [[ " ${AUTHORIZED_SUDO_USERS[*]} " =~ " ${user} " ]]; then
            if ! sudo -lU "$username" | grep -q "NOPASSWD: ALL"; then
                CHECK_usersBonus=false
            fi
        else
            if sudo -lU "$username" | grep -q "NOPASSWD: ALL"; then
                CHECK_usersBonus=false
            fi
        fi
    fi
done

# Now give the data out as JSON
echo "{
    \"icmp\": $CHECK_icmp,
    \"exporter\": $CHECK_exporter,
    \"requiredUsers\": $CHECK_requiredUsers,
    \"usersBonus\": $CHECK_usersBonus
}"

#!/bin/bash

CHECK_nginx=false
CHECK_content=false

# Nginx Running?
if systemctl is-active --quiet nginx; then
    CHECK_nginx=true
fi

# Website Content Correct? (flag should be the body)
if curl -s --max-time 2 "http://localhost" | grep -q "flag"; then
    CHECK_content=true
fi

# Now give the data out as JSON
echo "{
    \"nginx\": $CHECK_nginx,
    \"content\": $CHECK_content
}"
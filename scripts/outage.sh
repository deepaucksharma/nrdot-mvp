#!/bin/bash
set -e

# Script to simulate upstream outage by toggling mock-upstream service mode
# Usage: outage.sh on|off

if [ "$1" != "on" ] && [ "$1" != "off" ]; then
    echo "Usage: $0 on|off"
    exit 1
fi

ENABLED="false"
if [ "$1" == "on" ]; then
    ENABLED="true"
    echo "Simulating upstream outage..."
else
    echo "Healing upstream connection..."
fi

# Send request to mock-upstream control endpoint
curl -X POST \
     -H "Content-Type: application/json" \
     -d "{\"enabled\": $ENABLED}" \
     http://localhost:4319/control/outage

echo ""
echo "Outage mode set to: $ENABLED"

# Check the status
echo "Current upstream status:"
curl -s http://localhost:4319/control/status | jq

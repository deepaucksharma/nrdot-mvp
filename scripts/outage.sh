#!/bin/bash
set -e

# Script to simulate upstream outage by toggling mock-upstream service mode
# Usage: outage.sh on|off|status

# Default values
API_URL="${UPSTREAM_API_URL:-http://localhost:4319}"
COMMAND="$1"

# Command validation and help
if [[ "$COMMAND" != "on" && "$COMMAND" != "off" && "$COMMAND" != "status" ]]; then
  echo "Usage: $0 [on|off|status]"
  echo
  echo "Commands:"
  echo "  on      - Simulate upstream outage"
  echo "  off     - Heal upstream connection"
  echo "  status  - Show current upstream status"
  exit 1
fi

# Just show status if that's what was requested
if [[ "$COMMAND" == "status" ]]; then
  echo "Current upstream status:"
  curl -s "${API_URL}/control/status" | jq
  exit 0
fi

# Set enabled flag based on command
ENABLED="false"
if [[ "$COMMAND" == "on" ]]; then
  ENABLED="true"
  echo "Simulating upstream outage..."
else
  echo "Healing upstream connection..."
fi

# Send request to mock-upstream control endpoint
curl -X POST \
     -H "Content-Type: application/json" \
     -d "{\"enabled\": $ENABLED}" \
     "${API_URL}/control/outage"

echo
echo "Outage mode set to: $ENABLED"

# Always show the status after making a change
echo "Current upstream status:"
curl -s "${API_URL}/control/status" | jq

#!/bin/bash
set -e

# Script to control failure rate in the mock upstream service
# Usage: set-failure-rate.sh PERCENTAGE

# Default values
API_URL="${UPSTREAM_API_URL:-http://localhost:4319}"
PERCENTAGE="${1:-0}"

# Validate input
if ! [[ "$PERCENTAGE" =~ ^[0-9]+$ ]] || [ "$PERCENTAGE" -lt 0 ] || [ "$PERCENTAGE" -gt 100 ]; then
  echo "Error: Percentage must be a number between 0 and 100"
  echo "Usage: $0 PERCENTAGE"
  exit 1
fi

echo "Setting failure rate to ${PERCENTAGE}%..."

# Send request to mock-upstream control endpoint
curl -X POST \
     -H "Content-Type: application/json" \
     -d "{\"rate_percent\": $PERCENTAGE}" \
     "${API_URL}/control/failure-rate"

echo
echo "Failure rate set to: ${PERCENTAGE}%"

# Always show the status after making a change
echo "Current upstream status:"
curl -s "${API_URL}/control/status" | jq

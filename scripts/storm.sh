#!/bin/bash
set -e

# Script to generate high cardinality load on the collector
# This simulates a tag explosion event to test the cardinality limiter

# Configuration with defaults
DURATION=${1:-30}           # Duration in seconds (default 30)
COLLECTOR_URL="${COLLECTOR_URL:-http://localhost:4318}"
TAGS_PER_DATAPOINT=${TAGS_PER_DATAPOINT:-5}
DELAY=${DELAY:-0.05}        # Delay between requests in seconds

# Generate random string more efficiently
random_string() {
  local length=${1:-32}
  echo "$(cat /dev/urandom | tr -dc 'a-zA-Z0-9' | head -c $length)"
}

echo "Starting tag storm for $DURATION seconds..."
echo "  - Sending to: $COLLECTOR_URL"
echo "  - Tags per datapoint: $TAGS_PER_DATAPOINT"

# Run in background with timeout
(
    end=$((SECONDS + DURATION))
    REQUEST_COUNT=0
    
    while [ $SECONDS -lt $end ]; do
        # Generate random tag base value
        RANDOM_TAG=$(random_string 16)
        
        # Build tag attributes dynamically based on TAGS_PER_DATAPOINT
        TAG_JSON=""
        for i in $(seq 1 $TAGS_PER_DATAPOINT); do
          TAG_JSON="${TAG_JSON}{\"key\": \"random.tag.$i\", \"value\": {\"stringValue\": \"${RANDOM_TAG}-$i\"}},"
        done
        # Add common tags
        TAG_JSON="${TAG_JSON}{\"key\": \"container.image.tag\", \"value\": {\"stringValue\": \"v1.$RANDOM_TAG\"}},
                   {\"key\": \"k8s.pod.uid\", \"value\": {\"stringValue\": \"pod-$RANDOM_TAG\"}}"
        
        # Send metrics with high cardinality
        curl -s -X POST -H "Content-Type: application/json" \
             --data "{
                \"resourceMetrics\": [{
                    \"resource\": {
                        \"attributes\": [
                            {\"key\": \"service.name\", \"value\": {\"stringValue\": \"storm-generator\"}}
                        ]
                    },
                    \"scopeMetrics\": [{
                        \"metrics\": [{
                            \"name\": \"container.explosion.metric\",
                            \"gauge\": {
                                \"dataPoints\": [{
                                    \"asDouble\": 1.0,
                                    \"timeUnixNano\": \"$(date +%s)000000000\",
                                    \"attributes\": [
                                        $TAG_JSON
                                    ]
                                }]
                            }
                        }]
                    }]
                }]
             }" \
             "$COLLECTOR_URL/v1/metrics" > /dev/null
        
        REQUEST_COUNT=$((REQUEST_COUNT + 1))
        
        # Brief pause to avoid overwhelming the system
        sleep $DELAY
    done
    
    echo "Storm completed. Sent $REQUEST_COUNT requests with high cardinality tags."
) &

PID=$!
echo "Storm running in background (PID: $PID). Will stop after $DURATION seconds."
echo "To cancel early: kill $PID"

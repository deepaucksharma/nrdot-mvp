#!/bin/bash
set -e

# Script to generate high cardinality load on the collector
# This simulates a tag explosion event to test the cardinality limiter

# Duration in seconds (default 30 seconds)
DURATION=${1:-30}

echo "Starting tag storm for $DURATION seconds..."

# Run in background with timeout
(
    end=$((SECONDS + DURATION))
    
    while [ $SECONDS -lt $end ]; do
        # Generate random tag values
        RANDOM_TAG=$(cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 32 | head -n 1)
        
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
                                        {\"key\": \"container.image.tag\", \"value\": {\"stringValue\": \"v1.$RANDOM_TAG\"}},
                                        {\"key\": \"k8s.pod.uid\", \"value\": {\"stringValue\": \"pod-$RANDOM_TAG\"}},
                                        {\"key\": \"random.tag.1\", \"value\": {\"stringValue\": \"$RANDOM_TAG-1\"}},
                                        {\"key\": \"random.tag.2\", \"value\": {\"stringValue\": \"$RANDOM_TAG-2\"}},
                                        {\"key\": \"random.tag.3\", \"value\": {\"stringValue\": \"$RANDOM_TAG-3\"}}
                                    ]
                                }]
                            }
                        }]
                    }]
                }]
             }" \
             http://localhost:4318/v1/metrics > /dev/null
        
        # Brief pause to avoid overwhelming the system
        sleep 0.05
    done
) &

echo "Storm running in background. Will stop after $DURATION seconds."

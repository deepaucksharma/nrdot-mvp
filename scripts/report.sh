#!/bin/bash
set -e

# Script to report on key metrics from the NRDOT-MVP system
# Queries Prometheus API for essential metrics

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

echo "==== NRDOT-MVP System Report ===="
echo "Time: $(date)"
echo

# Function to query Prometheus
query_prom() {
    local metric=$1
    curl -s "http://localhost:9090/api/v1/query?query=$metric" | jq -r '.data.result[0].value[1]' 2>/dev/null || echo "N/A"
}

# Function to query Prometheus with labels
query_prom_label() {
    local metric=$1
    local label=$2
    curl -s "http://localhost:9090/api/v1/query?query=$metric{$label}" | jq -r '.data.result[0].value[1]' 2>/dev/null || echo "N/A"
}

# Check if services are running
echo "=== Service Status ==="
if curl -s http://localhost:4318/v1/metrics > /dev/null; then
    echo -e "Collector API: ${GREEN}UP${NC}"
else
    echo -e "Collector API: ${RED}DOWN${NC}"
fi

if curl -s http://localhost:4319/control/status > /dev/null; then
    echo -e "Mock Upstream: ${GREEN}UP${NC}"
else
    echo -e "Mock Upstream: ${RED}DOWN${NC}"
fi

if curl -s http://localhost:9090/api/v1/query?query=up > /dev/null; then
    echo -e "Prometheus: ${GREEN}UP${NC}"
else
    echo -e "Prometheus: ${RED}DOWN${NC}"
fi
echo

# Resource usage
echo "=== Resource Usage ==="
CPU=$(query_prom "rate(container_cpu_usage_seconds_total{name=~\".*collector.*\"}[1m])*100" | awk '{printf "%.2f", $1}')
RSS=$(query_prom "container_memory_rss{name=~\".*collector.*\"}/1024/1024" | awk '{printf "%.2f", $1}')

if [[ $(echo "$CPU < 2.0" | bc -l) -eq 1 ]]; then
    echo -e "CPU Usage: ${GREEN}${CPU}%${NC} (target: <2%)"
else
    echo -e "CPU Usage: ${RED}${CPU}%${NC} (target: <2%)"
fi

if [[ $(echo "$RSS < 150.0" | bc -l) -eq 1 ]]; then
    echo -e "Memory RSS: ${GREEN}${RSS} MiB${NC} (target: <150 MiB)"
else
    echo -e "Memory RSS: ${RED}${RSS} MiB${NC} (target: <150 MiB)"
fi

GC_DURATION=$(query_prom "go_gc_duration_seconds{quantile=\"0.99\"}" | awk '{printf "%.2f", $1*1000}')
echo -e "GC Duration (P99): ${GC_DURATION} ms"
echo

# Queue and DLQ metrics
echo "=== Queue & DLQ Status ==="
QUEUE_FILL=$(query_prom "apq_fill_ratio" | awk '{printf "%.2f", $1*100}')

if [[ $(echo "$QUEUE_FILL < 50.0" | bc -l) -eq 1 ]]; then
    echo -e "APQ Fill: ${GREEN}${QUEUE_FILL}%${NC}"
elif [[ $(echo "$QUEUE_FILL < 90.0" | bc -l) -eq 1 ]]; then
    echo -e "APQ Fill: ${YELLOW}${QUEUE_FILL}%${NC}"
else
    echo -e "APQ Fill: ${RED}${QUEUE_FILL}%${NC} (spilling!)"
fi

# Queue size by class
CRITICAL_SIZE=$(query_prom_label "apq_class_size" "class=\"critical\"")
HIGH_SIZE=$(query_prom_label "apq_class_size" "class=\"high\"")
NORMAL_SIZE=$(query_prom_label "apq_class_size" "class=\"normal\"")
echo "Queue Size - Critical: ${CRITICAL_SIZE}, High: ${HIGH_SIZE}, Normal: ${NORMAL_SIZE}"

# DLQ stats
DLQ_UTIL=$(query_prom "dlq_utilization_ratio" | awk '{printf "%.2f", $1*100}')
DLQ_AGE=$(query_prom "dlq_oldest_age_seconds" | awk '{printf "%.1f", $1/60/60}') # Convert to hours

if [[ $(echo "$DLQ_UTIL < 10.0" | bc -l) -eq 1 ]]; then
    echo -e "DLQ Utilization: ${GREEN}${DLQ_UTIL}%${NC}"
else
    echo -e "DLQ Utilization: ${YELLOW}${DLQ_UTIL}%${NC}"
fi

echo "DLQ Oldest Message: ${DLQ_AGE} hours"
echo

# Cardinality metrics
echo "=== Cardinality Status ==="
KEYS_USED=$(query_prom "cl_keys_used")
DROPPED_SAMPLES=$(query_prom "cl_dropped_samples_total")

if [[ $(echo "$KEYS_USED < 60000" | bc -l) -eq 1 ]]; then
    echo -e "Keys Tracked: ${GREEN}${KEYS_USED}${NC} (max: 65536)"
else
    echo -e "Keys Tracked: ${RED}${KEYS_USED}${NC} (max: 65536)"
fi

echo "Dropped Samples: ${DROPPED_SAMPLES}"
echo

# Mock upstream status
echo "=== Mock Upstream Status ==="
UPSTREAM_STATUS=$(curl -s http://localhost:4319/control/status)
OUTAGE_ENABLED=$(echo "$UPSTREAM_STATUS" | jq -r '.outage_enabled')
FAILURE_RATE=$(echo "$UPSTREAM_STATUS" | jq -r '.failure_rate_percent')

if [[ "$OUTAGE_ENABLED" == "true" ]]; then
    echo -e "Upstream Status: ${RED}OUTAGE ENABLED${NC}"
else
    echo -e "Upstream Status: ${GREEN}HEALTHY${NC}"
fi

if [[ "$FAILURE_RATE" == "0" ]]; then
    echo -e "Failure Rate: ${GREEN}${FAILURE_RATE}%${NC}"
else
    echo -e "Failure Rate: ${YELLOW}${FAILURE_RATE}%${NC}"
fi

echo
echo "Report complete."

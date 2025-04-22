#!/bin/bash
set -e

# Script to report on key metrics from the NRDOT-MVP system
# Queries Prometheus API for essential metrics

# Configuration with defaults
PROM_URL="${PROM_URL:-http://localhost:9090}"
COLLECTOR_URL="${COLLECTOR_URL:-http://localhost:4318}"
UPSTREAM_URL="${UPSTREAM_URL:-http://localhost:4319}"
FORMAT="${FORMAT:-color}"  # color, plain, json

# Colors for output (only if format=color)
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Don't use colors if format is not color
if [ "$FORMAT" != "color" ]; then
  RED=""
  GREEN=""
  YELLOW=""
  NC=""
fi

# Initialize JSON output if requested
if [ "$FORMAT" == "json" ]; then
  RESULT="{"
fi

# Print function based on format
print_metric() {
  local name=$1
  local value=$2
  local status=$3  # good, warn, bad, neutral
  local unit=$4
  
  # Format value with 2 decimal places if it's numeric and not "N/A"
  if [[ "$value" =~ ^[0-9.]+$ ]]; then
    value=$(printf "%.2f" $value)
  fi
  
  if [ "$FORMAT" == "json" ]; then
    # Add to JSON output
    RESULT="$RESULT\"$name\":{\"value\":\"$value\",\"status\":\"$status\",\"unit\":\"$unit\"},"
  elif [ "$FORMAT" == "plain" ]; then
    # Plain text format
    echo "$name: $value $unit"
  else
    # Color format
    case $status in
      good)
        echo -e "$name: ${GREEN}$value $unit${NC}"
        ;;
      warn)
        echo -e "$name: ${YELLOW}$value $unit${NC}"
        ;;
      bad)
        echo -e "$name: ${RED}$value $unit${NC}"
        ;;
      *)
        echo -e "$name: $value $unit"
        ;;
    esac
  fi
}

print_section() {
  local title=$1
  
  if [ "$FORMAT" == "json" ]; then
    # Sections don't affect JSON output
    return
  elif [ "$FORMAT" == "plain" ]; then
    echo "=== $title ==="
  else
    echo "=== $title ==="
  fi
}

print_header() {
  local title=$1
  
  if [ "$FORMAT" == "json" ]; then
    # Headers don't affect JSON output
    return
  elif [ "$FORMAT" == "plain" ]; then
    echo "$title"
    echo "Time: $(date)"
    echo
  else
    echo "$title"
    echo "Time: $(date)"
    echo
  fi
}

# Function to query Prometheus
query_prom() {
  local metric=$1
  curl -s "${PROM_URL}/api/v1/query?query=$metric" | jq -r '.data.result[0].value[1]' 2>/dev/null || echo "N/A"
}

# Function to query Prometheus with labels
query_prom_label() {
  local metric=$1
  local label=$2
  curl -s "${PROM_URL}/api/v1/query?query=$metric{$label}" | jq -r '.data.result[0].value[1]' 2>/dev/null || echo "N/A"
}

# Function to check if a service is up
check_service() {
  local name=$1
  local url=$2
  
  if curl -s "$url" > /dev/null 2>&1; then
    echo "UP"
  else
    echo "DOWN"
  fi
}

# Start report
print_header "==== NRDOT-MVP System Report ===="

# Check if services are running
print_section "Service Status"

COLLECTOR_STATUS=$(check_service "Collector" "${COLLECTOR_URL}/v1/metrics")
UPSTREAM_STATUS=$(check_service "Upstream" "${UPSTREAM_URL}/control/status")
PROM_STATUS=$(check_service "Prometheus" "${PROM_URL}/api/v1/query?query=up")

print_metric "Collector API" "$COLLECTOR_STATUS" "$([ "$COLLECTOR_STATUS" == "UP" ] && echo "good" || echo "bad")" ""
print_metric "Mock Upstream" "$UPSTREAM_STATUS" "$([ "$UPSTREAM_STATUS" == "UP" ] && echo "good" || echo "bad")" ""
print_metric "Prometheus" "$PROM_STATUS" "$([ "$PROM_STATUS" == "UP" ] && echo "good" || echo "bad")" ""
echo

# Resource usage
print_section "Resource Usage"
CPU=$(query_prom "rate(container_cpu_usage_seconds_total{name=~\".*collector.*\"}[1m])*100" | awk '{printf "%.2f", $1}')
RSS=$(query_prom "container_memory_rss{name=~\".*collector.*\"}/1024/1024" | awk '{printf "%.2f", $1}')
GC_DURATION=$(query_prom "go_gc_duration_seconds{quantile=\"0.99\"}" | awk '{printf "%.2f", $1*1000}')

CPU_STATUS="good"
if [[ $(echo "$CPU > 2.0" | bc -l) -eq 1 ]]; then
  CPU_STATUS="bad"
fi

RSS_STATUS="good"
if [[ $(echo "$RSS > 150.0" | bc -l) -eq 1 ]]; then
  RSS_STATUS="bad"
fi

print_metric "CPU Usage" "$CPU" "$CPU_STATUS" "% (target: <2%)"
print_metric "Memory RSS" "$RSS" "$RSS_STATUS" "MiB (target: <150 MiB)"
print_metric "GC Duration (P99)" "$GC_DURATION" "neutral" "ms"
echo

# Queue and DLQ metrics
print_section "Queue & DLQ Status"
QUEUE_FILL=$(query_prom "apq_fill_ratio" | awk '{printf "%.2f", $1*100}')

QUEUE_STATUS="good"
if [[ $(echo "$QUEUE_FILL > 90.0" | bc -l) -eq 1 ]]; then
  QUEUE_STATUS="bad"
elif [[ $(echo "$QUEUE_FILL > 50.0" | bc -l) -eq 1 ]]; then
  QUEUE_STATUS="warn"
fi

print_metric "APQ Fill" "$QUEUE_FILL" "$QUEUE_STATUS" "%"

# Queue size by class
CRITICAL_SIZE=$(query_prom_label "apq_class_size" "class=\"critical\"")
HIGH_SIZE=$(query_prom_label "apq_class_size" "class=\"high\"")
NORMAL_SIZE=$(query_prom_label "apq_class_size" "class=\"normal\"")

print_metric "Queue Size - Critical" "$CRITICAL_SIZE" "neutral" "items"
print_metric "Queue Size - High" "$HIGH_SIZE" "neutral" "items"
print_metric "Queue Size - Normal" "$NORMAL_SIZE" "neutral" "items"

# DLQ stats
DLQ_UTIL=$(query_prom "dlq_utilization_ratio" | awk '{printf "%.2f", $1*100}')
DLQ_AGE=$(query_prom "dlq_oldest_age_seconds" | awk '{printf "%.1f", $1/60/60}') # Convert to hours

DLQ_STATUS="good"
if [[ $(echo "$DLQ_UTIL > 10.0" | bc -l) -eq 1 ]]; then
  DLQ_STATUS="warn"
fi

print_metric "DLQ Utilization" "$DLQ_UTIL" "$DLQ_STATUS" "%"
print_metric "DLQ Oldest Message" "$DLQ_AGE" "neutral" "hours"
echo

# Cardinality metrics
print_section "Cardinality Status"
KEYS_USED=$(query_prom "cl_keys_used")
DROPPED_SAMPLES=$(query_prom "cl_dropped_samples_total")

KEYS_STATUS="good"
if [[ $(echo "$KEYS_USED > 60000" | bc -l) -eq 1 ]]; then
  KEYS_STATUS="bad"
fi

print_metric "Keys Tracked" "$KEYS_USED" "$KEYS_STATUS" "(max: 65536)"
print_metric "Dropped Samples" "$DROPPED_SAMPLES" "neutral" ""
echo

# Mock upstream status
print_section "Mock Upstream Status"
if [ "$UPSTREAM_STATUS" == "UP" ]; then
  UPSTREAM_JSON=$(curl -s "${UPSTREAM_URL}/control/status")
  OUTAGE_ENABLED=$(echo "$UPSTREAM_JSON" | jq -r '.outage_enabled')
  FAILURE_RATE=$(echo "$UPSTREAM_JSON" | jq -r '.failure_rate_percent')
  
  OUTAGE_STATUS="good"
  if [[ "$OUTAGE_ENABLED" == "true" ]]; then
    OUTAGE_STATUS="bad"
  fi
  
  RATE_STATUS="good"
  if [[ "$FAILURE_RATE" != "0" ]]; then
    RATE_STATUS="warn"
  fi
  
  print_metric "Upstream Status" "$([ "$OUTAGE_ENABLED" == "true" ] && echo "OUTAGE" || echo "HEALTHY")" "$OUTAGE_STATUS" ""
  print_metric "Failure Rate" "$FAILURE_RATE" "$RATE_STATUS" "%"
else
  print_metric "Upstream Status" "UNREACHABLE" "bad" ""
fi

# Finalize JSON output if needed
if [ "$FORMAT" == "json" ]; then
  # Remove trailing comma and close JSON object
  RESULT="${RESULT%,}}"
  echo "$RESULT"
else
  echo
  echo "Report complete."
fi

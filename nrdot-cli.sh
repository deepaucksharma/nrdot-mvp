#!/bin/bash
#
# NRDOT-MVP Command Line Tool
# A unified interface for managing the NRDOT-MVP system
#

set -e

# Configuration with defaults
COLLECTOR_URL="${COLLECTOR_URL:-http://localhost:4318}"
UPSTREAM_URL="${UPSTREAM_URL:-http://localhost:4319}"
PROM_URL="${PROM_URL:-http://localhost:9090}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Function to display help
show_help() {
  echo "NRDOT-MVP Command Line Tool"
  echo
  echo "Usage: $0 <command> [options]"
  echo
  echo "Commands:"
  echo "  build               Build all components"
  echo "  start               Start the system (shorthand for 'up')"
  echo "  up                  Start all containers"
  echo "  down                Stop all containers"
  echo "  status              Show system status"
  echo "  report              Generate detailed system report"
  echo "  storm <duration>    Generate high cardinality load"
  echo "  outage <on|off>     Control upstream outage simulation"
  echo "  failure <rate>      Set failure rate (0-100%)"
  echo "  test                Run full test sequence"
  echo "  clean               Clean all build artifacts"
  echo "  reset               Reset the system completely"
  echo
  echo "Environment variables:"
  echo "  COLLECTOR_URL       Collector URL (default: $COLLECTOR_URL)"
  echo "  UPSTREAM_URL        Mock upstream URL (default: $UPSTREAM_URL)"
  echo "  PROM_URL            Prometheus URL (default: $PROM_URL)"
  echo
  echo "Examples:"
  echo "  $0 storm 60         Run tag storm for 60 seconds"
  echo "  $0 outage on        Simulate upstream outage"
  echo "  $0 failure 50       Set 50% failure rate"
  echo
  exit 0
}

# Function to check prerequisites
check_prereqs() {
  MISSING=0
  
  if ! command -v curl &> /dev/null; then
    echo "❌ curl is required but not found"
    MISSING=1
  fi
  
  if ! command -v jq &> /dev/null; then
    echo "❌ jq is required but not found"
    MISSING=1
  fi
  
  if ! command -v docker &> /dev/null; then
    echo "❌ docker is required but not found"
    MISSING=1
  fi
  
  if ! command -v docker-compose &> /dev/null; then
    echo "❌ docker-compose is required but not found"
    MISSING=1
  fi
  
  if [ $MISSING -eq 1 ]; then
    echo "Please install missing prerequisites and try again."
    exit 1
  fi
}

# Function to check if system is running
check_system() {
  if ! curl -s "$COLLECTOR_URL/v1/metrics" > /dev/null 2>&1; then
    echo "❌ Collector not running. Start the system with: $0 start"
    return 1
  fi
  
  if ! curl -s "$UPSTREAM_URL/control/status" > /dev/null 2>&1; then
    echo "❌ Mock upstream not running. Start the system with: $0 start"
    return 1
  fi
  
  return 0
}

# If no command provided, show help
if [ $# -eq 0 ]; then
  show_help
fi

# Process commands
case "$1" in
  help)
    show_help
    ;;
    
  build)
    echo "Building all components..."
    cd "$SCRIPT_DIR"
    chmod +x ./build.sh
    ./build.sh
    ;;
    
  up|start)
    echo "Starting all containers..."
    cd "$SCRIPT_DIR"
    mkdir -p data/dlq
    chmod +x ./scripts/*.sh
    docker-compose up -d
    echo "System started. Check status with: $0 status"
    ;;
    
  down|stop)
    echo "Stopping all containers..."
    cd "$SCRIPT_DIR"
    docker-compose down
    echo "System stopped."
    ;;
    
  status)
    if check_system; then
      echo "✅ System is running"
      echo
      echo "Container status:"
      docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep -E 'collector|mock-upstream|prometheus|grafana'
      echo
      echo "Mock upstream status:"
      curl -s "$UPSTREAM_URL/control/status" | jq
    fi
    ;;
    
  report)
    if check_system; then
      cd "$SCRIPT_DIR"
      chmod +x ./scripts/report.sh
      COLLECTOR_URL="$COLLECTOR_URL" UPSTREAM_URL="$UPSTREAM_URL" PROM_URL="$PROM_URL" ./scripts/report.sh
    fi
    ;;
    
  storm)
    if check_system; then
      DURATION=${2:-30}
      echo "Starting tag storm for $DURATION seconds..."
      cd "$SCRIPT_DIR"
      chmod +x ./scripts/storm.sh
      COLLECTOR_URL="$COLLECTOR_URL" ./scripts/storm.sh "$DURATION"
    fi
    ;;
    
  outage)
    if check_system; then
      MODE=${2:-status}
      cd "$SCRIPT_DIR"
      chmod +x ./scripts/outage.sh
      UPSTREAM_API_URL="$UPSTREAM_URL" ./scripts/outage.sh "$MODE"
    fi
    ;;
    
  failure)
    if check_system; then
      RATE=${2:-0}
      cd "$SCRIPT_DIR"
      chmod +x ./scripts/set-failure-rate.sh
      UPSTREAM_API_URL="$UPSTREAM_URL" ./scripts/set-failure-rate.sh "$RATE"
    fi
    ;;
    
  test)
    echo "Running full test sequence..."
    cd "$SCRIPT_DIR"
    
    echo "1. Ensuring system is running..."
    $0 up
    sleep 30
    
    echo "2. Running status check..."
    $0 report
    
    echo "3. Starting tag storm..."
    $0 storm 60
    sleep 70
    
    echo "4. Checking status after storm..."
    $0 report
    
    echo "5. Setting 50% failure rate..."
    $0 failure 50
    sleep 30
    
    echo "6. Checking status with failure rate..."
    $0 report
    
    echo "7. Simulating outage..."
    $0 outage on
    sleep 120
    
    echo "8. Checking status during outage..."
    $0 report
    
    echo "9. Healing the system..."
    $0 outage off
    sleep 60
    
    echo "10. Final status check..."
    $0 report
    
    echo "Test sequence completed!"
    ;;
    
  clean)
    echo "Cleaning build artifacts..."
    cd "$SCRIPT_DIR"
    rm -f bin/* plugins/*.so
    echo "Clean complete."
    ;;
    
  reset)
    echo "Resetting the system..."
    cd "$SCRIPT_DIR"
    docker-compose down -v
    rm -rf data/dlq/*
    echo "Starting fresh system..."
    docker-compose up -d
    echo "System reset complete."
    ;;
    
  *)
    echo "Unknown command: $1"
    echo
    show_help
    ;;
esac

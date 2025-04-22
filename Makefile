.PHONY: build clean up down test storm outage heal report e2e status failure-rate json-report check

# Configurable variables
DURATION ?= 30
FAILURE_RATE ?= 50
COLLECTOR_URL ?= http://localhost:4318
UPSTREAM_URL ?= http://localhost:4319
PROM_URL ?= http://localhost:9090

# Build all components
build:
	chmod +x ./build.sh
	./build.sh

# Clean up binaries and artifacts
clean:
	rm -f bin/* plugins/*.so
	rm -rf data/dlq/*

# Start the entire stack with Docker Compose
up:
	mkdir -p data/dlq
	chmod +x ./scripts/*.sh
	docker-compose up -d

# Start in foreground mode for debugging
up-fg:
	mkdir -p data/dlq
	chmod +x ./scripts/*.sh
	docker-compose up

# Stop the entire stack
down:
	docker-compose down

# Stop and remove all data
reset: down
	docker-compose down -v
	rm -rf data/dlq/*
	docker-compose up -d

# Run integration tests
test:
	go test -v ./...

# Generate high cardinality load
storm:
	COLLECTOR_URL=$(COLLECTOR_URL) ./scripts/storm.sh $(DURATION)

# Simulate outage in upstream receiver
outage:
	UPSTREAM_API_URL=$(UPSTREAM_URL) ./scripts/outage.sh on

# Recover from outage
heal:
	UPSTREAM_API_URL=$(UPSTREAM_URL) ./scripts/outage.sh off

# Check upstream status
status:
	UPSTREAM_API_URL=$(UPSTREAM_URL) ./scripts/outage.sh status

# Set failure rate
failure-rate:
	UPSTREAM_API_URL=$(UPSTREAM_URL) ./scripts/set-failure-rate.sh $(FAILURE_RATE)

# Display status report
report:
	COLLECTOR_URL=$(COLLECTOR_URL) UPSTREAM_URL=$(UPSTREAM_URL) PROM_URL=$(PROM_URL) FORMAT=color ./scripts/report.sh

# Get JSON report for programmatic usage
json-report:
	COLLECTOR_URL=$(COLLECTOR_URL) UPSTREAM_URL=$(UPSTREAM_URL) PROM_URL=$(PROM_URL) FORMAT=json ./scripts/report.sh

# Basic system check
check:
	@echo "Checking system components..."
	@echo
	@echo "1. Checking Docker status:"
	@docker info 2>/dev/null || echo "Docker not running or not available"
	@echo
	@echo "2. Checking for required tools:"
	@command -v curl >/dev/null 2>&1 && echo "✓ curl found" || echo "✗ curl not found"
	@command -v jq >/dev/null 2>&1 && echo "✓ jq found" || echo "✗ jq not found"
	@command -v bc >/dev/null 2>&1 && echo "✓ bc found" || echo "✗ bc not found"
	@echo
	@echo "3. Checking container status:"
	@docker ps --format "{{.Names}}: {{.Status}}" 2>/dev/null | grep -E 'collector|mock-upstream|prometheus|grafana' || echo "No NRDOT containers found"

# End-to-end test sequence
e2e:
	@echo "Running E2E test sequence..."
	@echo "1. Starting services..."
	$(MAKE) up
	@sleep 30
	@echo "2. Running status check..."
	$(MAKE) report
	@echo "3. Starting tag storm..."
	$(MAKE) storm DURATION=60
	@sleep 70
	@echo "4. Checking status after storm..."
	$(MAKE) report
	@echo "5. Setting 50% failure rate..."
	$(MAKE) failure-rate FAILURE_RATE=50
	@sleep 30
	@echo "6. Checking status with failure rate..."
	$(MAKE) report
	@echo "7. Simulating outage..."
	$(MAKE) outage
	@sleep 120
	@echo "8. Checking status during outage..."
	$(MAKE) report
	@echo "9. Healing the system..."
	$(MAKE) heal
	@sleep 60
	@echo "10. Final status check..."
	$(MAKE) report
	@echo "E2E test completed!"

# Show help
help:
	@echo "NRDOT-MVP Makefile commands:"
	@echo
	@echo "Build and deployment:"
	@echo "  make build             - Build all components"
	@echo "  make clean             - Clean up binaries and artifacts"
	@echo "  make up                - Start the entire stack with Docker Compose"
	@echo "  make up-fg             - Start in foreground for debugging"
	@echo "  make down              - Stop the entire stack"
	@echo "  make reset             - Stop, remove data, and restart"
	@echo "  make check             - Basic system check"
	@echo
	@echo "Testing and debugging:"
	@echo "  make test              - Run integration tests"
	@echo "  make storm [DURATION=N]          - Generate high cardinality load"
	@echo "  make outage                      - Simulate outage in upstream"
	@echo "  make heal                        - Recover from outage"
	@echo "  make status                      - Check upstream status"
	@echo "  make failure-rate [FAILURE_RATE=N] - Set failure rate percentage"
	@echo "  make report                      - Display status report"
	@echo "  make json-report                 - Get JSON status report"
	@echo "  make e2e                         - Run end-to-end test sequence"
	@echo
	@echo "Example with custom parameters:"
	@echo "  make storm DURATION=120 COLLECTOR_URL=http://example.com:4318"

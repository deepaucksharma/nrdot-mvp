.PHONY: build clean up down test storm outage heal report e2e

# Build all components
build:
	./build.sh

# Clean up binaries and artifacts
clean:
	rm -f bin/* plugins/*.so
	rm -rf data/dlq/*

# Start the entire stack with Docker Compose
up:
	mkdir -p data/dlq
	docker-compose up -d

# Stop the entire stack
down:
	docker-compose down

# Run integration tests
test:
	go test -v ./...

# Generate high cardinality load
storm:
	./scripts/storm.sh

# Simulate outage in upstream receiver
outage:
	./scripts/outage.sh on

# Recover from outage
heal:
	./scripts/outage.sh off

# Display status report
report:
	./scripts/report.sh

# End-to-end test sequence
e2e:
	@echo "Running E2E test sequence..."
	@echo "1. Starting services..."
	$(MAKE) up
	@sleep 30
	@echo "2. Running status check..."
	$(MAKE) report
	@echo "3. Starting tag storm..."
	$(MAKE) storm
	@sleep 30
	@echo "4. Checking status after storm..."
	$(MAKE) report
	@echo "5. Simulating outage..."
	$(MAKE) outage
	@sleep 300
	@echo "6. Checking status during outage..."
	$(MAKE) report
	@echo "7. Healing the system..."
	$(MAKE) heal
	@sleep 30
	@echo "8. Final status check..."
	$(MAKE) report
	@echo "E2E test completed!"

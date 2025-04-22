# NRDOT-MVP

NRDOT + MVP - Proof-of-Concept slice of NRDOT + v9.0, demonstrating experimental plugins for OpenTelemetry Collector.

## Project Overview

This project demonstrates three core experimental plugins for OpenTelemetry Collector:

1. **CardinalityLimiter processor** - Entropy-based cardinality control for metrics
2. **Adaptive Priority Queue (APQ)** - WRR-based priority queuing with 3 classes
3. **Enhanced DLQ** - File-based storage with integrity verification

The goal is to demonstrate tight resource usage (<2% CPU, <150 MiB RSS) while providing advanced data reliability features.

## System Requirements

- **Operating System**: Linux (Go plugins only work on Linux)
- **Kernel**: 4.4+ recommended
- **Memory**: 256 MiB minimum, 512 MiB recommended
- **Disk**: 20 GB free space for DLQ storage
- **Prerequisites**: 
  - Go 1.21 or higher
  - Docker and Docker Compose
  - Make
  - jq
  - curl
  - bc
  - awk

## Quick Start

```bash
# Clone the repository
git clone https://github.com/deepaucksharma/nrdot-mvp.git
cd nrdot-mvp

# Build the project (this is required before starting the containers)
make build

# Start the containerized stack
make up

# Check the system status
make report

# Generate high-cardinality load
make storm

# Simulate an outage
make outage

# Recover from outage
make heal

# Stop the stack
make down
```

## Test Scenarios

This MVP demonstrates these key runtime behaviors:

1. **Dynamic Cardinality Control** - Handle tag explosions while keeping memory usage bounded
2. **Priority-Based Backpressure** - Critical data flows even when the system is overloaded
3. **Durable Storage & Recovery** - Persist data through outages and replay when connectivity is restored

## Architecture

```
┌─────────────┐    ┌──────────────────────────────────────┐    ┌───────────────┐
│  OTLP Input │───►│               Collector              │───►│ Mock Upstream │
└─────────────┘    │                                      │    └───────────────┘
                   │ ┌────────────┐  ┌────┐  ┌──────────┐ │
                   │ │Cardinality │─►│Batch│─►│   APQ    │ │
                   │ │  Limiter   │  └────┘  │  Queue   │ │
                   │ └────────────┘          └────┬─────┘ │
                   │                              │       │
                   │                         ┌────▼─────┐ │
                   │                         │  DLQ     │ │
                   │                         │ Storage  │ │
                   │                         └──────────┘ │
                   └──────────────────────────────────────┘
```

## Known Limitations

1. **Linux Only** - Go plugins are supported only on Linux platforms
2. **Priority Recovery** - Spilled segments lose priority flagging and requeue as normal priority
3. **Deterministic Hashing** - The CardinalityLimiter uses a simple hashing approach that is not fully deterministic
4. **Simple Eviction** - Currently uses a simple eviction mechanism instead of LRU or heat-weighted
5. **Synchronous Verification** - DLQ SHA-256 checks block the replay thread

## Project Components

- `cmd/collector/` - Custom OpenTelemetry collector
- `cmd/mock-upstream/` - Mock backend service with configurable failure modes
- `plugins/cl/` - CardinalityLimiter processor implementation
- `plugins/apq/` - Adaptive Priority Queue implementation
- `plugins/dlq/` - Enhanced DLQ file storage implementation
- `scripts/` - Testing and simulation scripts
- `otel-config/` - OpenTelemetry collector configuration
- `dashboards/` - Grafana dashboards

## Development

### Building the Plugins

The CardinalityLimiter, APQ, and DLQ plugins are built as Go plugins using the `-buildmode=plugin` flag. You can build them individually:

```bash
# Build the CardinalityLimiter plugin
go build -buildmode=plugin -o plugins/cl.so ./plugins/cl

# Build the APQ plugin
go build -buildmode=plugin -o plugins/apq.so ./plugins/apq

# Build the DLQ plugin
go build -buildmode=plugin -o plugins/dlq.so ./plugins/dlq
```

Or use the provided build script:

```bash
./build.sh
```

### Docker Compose

The project includes a Docker Compose file that sets up:

- OpenTelemetry Collector with custom plugins
- Prometheus for metrics collection
- Grafana for visualization
- Mock upstream service to simulate backend behavior

You can customize the configuration in `docker-compose.yaml`.

## Troubleshooting

If you encounter issues:

1. Check the collector logs: `docker logs nrdot-collector`
2. Verify the mock upstream is running: `curl http://localhost:4319/control/status`
3. Ensure all plugins are properly built and mounted
4. Check Prometheus targets: `http://localhost:9090/targets`

## License

Copyright (c) 2025

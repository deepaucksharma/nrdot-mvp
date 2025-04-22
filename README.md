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

## Quick Start

```bash
# Build the project
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
3. **Hash Eviction** - Simple LRU eviction, not heat-weighted
4. **Synchronous Verification** - DLQ SHA-256 checks block the replay thread

## Project Components

- `cmd/collector/` - Custom OpenTelemetry collector
- `cmd/mock-upstream/` - Mock backend service with configurable failure modes
- `plugins/cl/` - CardinalityLimiter processor implementation
- `plugins/apq/` - Adaptive Priority Queue implementation
- `plugins/dlq/` - Enhanced DLQ file storage implementation
- `scripts/` - Testing and simulation scripts
- `otel-config/` - OpenTelemetry collector configuration
- `dashboards/` - Grafana dashboards

## License

Copyright (c) 2025

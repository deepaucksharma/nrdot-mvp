#!/bin/bash
set -e

echo "Building NRDOT + MVP..."

# Create bin directory if it doesn't exist
mkdir -p bin

# Build plugins
echo "Building CardinalityLimiter plugin..."
go build -buildmode=plugin -o plugins/cl.so ./plugins/cl
go mod tidy -C ./plugins/cl

echo "Building APQ plugin..."
go build -buildmode=plugin -o plugins/apq.so ./plugins/apq
go mod tidy -C ./plugins/apq

echo "Building DLQ plugin..."
go build -buildmode=plugin -o plugins/dlq.so ./plugins/dlq
go mod tidy -C ./plugins/dlq

echo "Building custom OpenTelemetry collector with experimental tags..."
go build -tags "experimental apq" -o bin/otelcol-custom ./cmd/collector

echo "Building mock-upstream service..."
go build -o bin/mock-upstream ./cmd/mock-upstream

echo "Build complete! Binaries available in ./bin directory"
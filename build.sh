#!/bin/bash
set -e

echo "Building NRDOT + MVP..."

# Build plugins
echo "Building CardinalityLimiter plugin..."
go build -buildmode=plugin -o plugins/cl.so ./plugins/cl

echo "Building APQ plugin..."
go build -buildmode=plugin -o plugins/apq.so ./plugins/apq

echo "Building custom OpenTelemetry collector with experimental tags..."
go build -tags "experimental apq" -o bin/otelcol-custom ./cmd/collector

echo "Building mock-upstream service..."
go build -o bin/mock-upstream ./cmd/mock-upstream

echo "Build complete! Binaries available in ./bin directory"

#!/bin/bash
set -e

echo "Building NRDOT + MVP..."

# Create bin directory if it doesn't exist
mkdir -p bin

# Build all plugins in a loop
PLUGINS=("cl" "apq" "dlq")
for plugin in "${PLUGINS[@]}"; do
  echo "Building ${plugin} plugin..."
  go build -buildmode=plugin -o plugins/${plugin}.so ./plugins/${plugin}
  go mod tidy -C ./plugins/${plugin}
done

# Build main applications
echo "Building custom OpenTelemetry collector..."
go build -tags "experimental apq" -o bin/otelcol-custom ./cmd/collector

echo "Building mock-upstream service..."
go build -o bin/mock-upstream ./cmd/mock-upstream

echo "Build complete! Binaries available in ./bin directory"

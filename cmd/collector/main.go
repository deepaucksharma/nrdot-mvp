package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/service"

	// Import base components
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"

	// Import required components
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/extension/ballastextension"

	// Import custom plugins (need to be linked in when using buildmode=plugin)
	_ "github.com/nr-labs/apq" // Add a single line as specified in spec
	_ "github.com/nr-labs/cl"  // Cardinality limiter
	_ "github.com/nr-labs/dlq" // DLQ plugin
)

func main() {
	// Set appropriate memory ballast
	ballastSizeMiB := 64 // Default value
	if val, ok := os.LookupEnv("MEMORY_BALLAST_SIZE_MIB"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			ballastSizeMiB = i
		}
	}

	factories, err := components()
	if err != nil {
		log.Fatalf("Failed to build components: %v", err)
	}

	info := component.BuildInfo{
		Command:     "otelcol-custom",
		Description: "NRDOT + MVP OpenTelemetry Collector",
		Version:     "0.1.0",
	}

	// Create collector info
	collectorInfo := otelcol.CollectorInfo{
		BuildInfo: info,
	}

	// Create a cancelable context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handler for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
		cancel()
	}()

	params := otelcol.CollectorSettings{
		BuildInfo:      collectorInfo,
		Factories:      factories,
		ConfigProvider: otelcol.NewFileConfigProvider("otel-config/collector.yaml"),
	}

	col, err := otelcol.NewCollector(params)
	if err != nil {
		log.Fatalf("Failed to create collector: %v", err)
	}

	if err := col.Run(ctx); err != nil {
		log.Fatalf("Failed to run collector: %v", err)
	}
}

// components returns the default set of factories
func components() (otelcol.Factories, error) {
	// Create a plugin selector for registering custom plugins
	pluginSelector := []otelcol.PluginOption{
		otelcol.WithPluginFactory("apq", func() component.Factory {
			// Wire up APQ plugin
			return exporter.NewFactory()
		}),
		otelcol.WithPluginFactory("cardinalitylimiter", func() component.Factory {
			// Wire up CL plugin
			return processor.NewFactory() 
		}),
		otelcol.WithPluginFactory("file_storage", func() component.Factory {
			// Wire up DLQ plugin
			return extension.NewFactory()
		}),
	}

	var err error
	factories := otelcol.Factories{}
	
	// Receivers
	receivers, err := receiver.MakeFactoryMap(
		otlpreceiver.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.Receivers = receivers

	// Processors 
	processors, err := processor.MakeFactoryMap(
		batchprocessor.NewFactory(),
		memorylimiterprocessor.NewFactory(),
		// Register cardinalitylimiter processor
		// This will be handled via plugins
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.Processors = processors

	// Exporters
	exporters, err := exporter.MakeFactoryMap(
		otlphttpexporter.NewFactory(),
		// APQ-enabled exporter will be registered via plugins
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.Exporters = exporters

	// Extensions
	extensions, err := extension.MakeFactoryMap(
		ballastextension.NewFactory(),
		// File storage extension will be registered via plugins
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.Extensions = extensions

	return factories, nil
}

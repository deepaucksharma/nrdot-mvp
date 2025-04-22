package main

import (
	"log"
	"os"

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

	params := otelcol.CollectorSettings{
		BuildInfo:      info,
		Factories:      factories,
		ConfigProvider: service.NewDefaultConfigProvider(),
	}

	col, err := otelcol.NewCollector(params)
	if err != nil {
		log.Fatalf("Failed to create collector: %v", err)
	}

	if err := col.Run(context.Background()); err != nil {
		log.Fatalf("Failed to run collector: %v", err)
	}
}

// components returns the default set of factories
func components() (otelcol.Factories, error) {
	// TODO: Wire up custom plugins registration here with otelcol.WithPlugins()
	// or manually append factories from plugins

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
		// TODO: Register cardinalitylimiter/custom processor
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.Processors = processors

	// Exporters
	exporters, err := exporter.MakeFactoryMap(
		otlphttpexporter.NewFactory(),
		// TODO: Register custom APQ-enabled exporter
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.Exporters = exporters

	// Extensions
	extensions, err := extension.MakeFactoryMap(
		ballastextension.NewFactory(),
		// TODO: Register file_storage extension for DLQ
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.Extensions = extensions

	return factories, nil
}

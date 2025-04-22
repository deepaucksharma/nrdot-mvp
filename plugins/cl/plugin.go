package main

import (
	"context"
	"hash/fnv"
	"math"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Config defines configuration for CardinalityLimiter processor
type Config struct {
	MaxKeys        int      `mapstructure:"max_keys"`
	HighScore      float64  `mapstructure:"high_score"`
	CriticalScore  float64  `mapstructure:"critical_score"`
	AggregateLabels []string `mapstructure:"aggregate_labels"`
}

type cardinalityLimiterProcessor struct {
	logger         *zap.Logger
	config         *Config
	keyMap         map[uint64]int
	keyMapMutex    sync.RWMutex

	// Metrics
	droppedSamples *prometheus.CounterVec
	keysUsed       prometheus.Gauge
}

// metrics
var (
	droppedSamplesMetric = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cl_dropped_samples_total",
			Help: "Total number of samples dropped by the cardinality limiter",
		},
		[]string{"metric"},
	)
	keysUsedMetric = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "cl_keys_used",
			Help: "Current number of unique keys tracked by the cardinality limiter",
		},
	)
)

// newCardinalityLimiterProcessor creates a processor for limiting cardinality
func newCardinalityLimiterProcessor(logger *zap.Logger, config *Config) *cardinalityLimiterProcessor {
	if config.MaxKeys <= 0 {
		config.MaxKeys = 65536 // Default to 64k if not specified
	}
	if config.HighScore <= 0 {
		config.HighScore = 0.75
	}
	if config.CriticalScore <= 0 {
		config.CriticalScore = 0.90
	}

	return &cardinalityLimiterProcessor{
		logger:         logger,
		config:         config,
		keyMap:         make(map[uint64]int, config.MaxKeys),
		droppedSamples: droppedSamplesMetric,
		keysUsed:       keysUsedMetric,
	}
}

// processMetrics implements the ProcessMetricsFunc type
func (p *cardinalityLimiterProcessor) processMetrics(ctx context.Context, md pmetric.Metrics) (pmetric.Metrics, error) {
	rm := md.ResourceMetrics()
	for i := 0; i < rm.Len(); i++ {
		ilm := rm.At(i).ScopeMetrics()
		for j := 0; j < ilm.Len(); j++ {
			metrics := ilm.At(j).Metrics()
			for k := 0; k < metrics.Len(); k++ {
				metric := metrics.At(k)
				metricName := metric.Name()
				
				// Process each metric type
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					p.processDataPoints(metricName, metric.Gauge().DataPoints())
				case pmetric.MetricTypeSum:
					p.processDataPoints(metricName, metric.Sum().DataPoints())
				case pmetric.MetricTypeHistogram:
					p.processHistogramDataPoints(metricName, metric.Histogram().DataPoints())
				case pmetric.MetricTypeSummary:
					p.processSummaryDataPoints(metricName, metric.Summary().DataPoints())
				}
			}
		}
	}
	
	// Update keys used metric
	p.keyMapMutex.RLock()
	p.keysUsed.Set(float64(len(p.keyMap)))
	p.keyMapMutex.RUnlock()
	
	return md, nil
}

// processDataPoints handles number datapoints (gauge and sum)
func (p *cardinalityLimiterProcessor) processDataPoints(metricName string, dps pmetric.NumberDataPointSlice) {
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)
		score := p.calculateEntropyScore(dp.Attributes())
		
		if score >= p.config.CriticalScore {
			// Critical score - drop the sample
			p.droppedSamples.WithLabelValues(metricName).Inc()
			// Mark for removal (we'll handle this in a future implementation)
			// For now, we're just counting drops
		} else if score >= p.config.HighScore {
			// High score - aggregate by removing specified labels
			for _, labelToRemove := range p.config.AggregateLabels {
				dp.Attributes().Remove(labelToRemove)
			}
		}
		
		// Track key hash in map
		hash := p.hashAttributes(dp.Attributes())
		p.keyMapMutex.Lock()
		p.keyMap[hash]++
		
		// Check if we need to evict
		if len(p.keyMap) > p.config.MaxKeys {
			// For now, simple approach: remove a random key
			// TODO: Implement LRU or heat-weighted eviction
			for k := range p.keyMap {
				delete(p.keyMap, k)
				break
			}
		}
		p.keyMapMutex.Unlock()
	}
}

// processHistogramDataPoints handles histogram datapoints
func (p *cardinalityLimiterProcessor) processHistogramDataPoints(metricName string, dps pmetric.HistogramDataPointSlice) {
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)
		score := p.calculateEntropyScore(dp.Attributes())
		
		if score >= p.config.CriticalScore {
			// Critical score - drop the sample
			p.droppedSamples.WithLabelValues(metricName).Inc()
			// Mark for removal (in future implementation)
		} else if score >= p.config.HighScore {
			// High score - aggregate by removing specified labels
			for _, labelToRemove := range p.config.AggregateLabels {
				dp.Attributes().Remove(labelToRemove)
			}
		}
		
		// Track key hash in map (same logic as above)
		hash := p.hashAttributes(dp.Attributes())
		p.keyMapMutex.Lock()
		p.keyMap[hash]++
		if len(p.keyMap) > p.config.MaxKeys {
			for k := range p.keyMap {
				delete(p.keyMap, k)
				break
			}
		}
		p.keyMapMutex.Unlock()
	}
}

// processSummaryDataPoints handles summary datapoints
func (p *cardinalityLimiterProcessor) processSummaryDataPoints(metricName string, dps pmetric.SummaryDataPointSlice) {
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)
		score := p.calculateEntropyScore(dp.Attributes())
		
		if score >= p.config.CriticalScore {
			// Critical score - drop the sample
			p.droppedSamples.WithLabelValues(metricName).Inc()
			// Mark for removal (in future implementation)
		} else if score >= p.config.HighScore {
			// High score - aggregate by removing specified labels
			for _, labelToRemove := range p.config.AggregateLabels {
				dp.Attributes().Remove(labelToRemove)
			}
		}
		
		// Track key hash in map (same logic as above)
		hash := p.hashAttributes(dp.Attributes())
		p.keyMapMutex.Lock()
		p.keyMap[hash]++
		if len(p.keyMap) > p.config.MaxKeys {
			for k := range p.keyMap {
				delete(p.keyMap, k)
				break
			}
		}
		p.keyMapMutex.Unlock()
	}
}

// calculateEntropyScore computes the entropy-based score for a set of attributes
func (p *cardinalityLimiterProcessor) calculateEntropyScore(attrs pmetric.Map) float64 {
	// For this MVP, we'll use a simplistic approach:
	// Count the number of attributes and their total length as a proxy for entropy
	attrCount := attrs.Len()
	if attrCount == 0 {
		return 0.0
	}

	totalChars := 0
	attrs.Range(func(k string, v pmetric.Value) bool {
		totalChars += len(k) + len(v.AsString())
		return true
	})
	
	// Normalize to a 0-1 score 
	// This is a simple heuristic - a more sophisticated entropy calculation
	// would be implemented in a production version
	score := math.Min(1.0, float64(totalChars)/(100.0+float64(attrCount*5)))
	return score
}

// hashAttributes creates a FNV-1a 64-bit hash of the attributes
func (p *cardinalityLimiterProcessor) hashAttributes(attrs pmetric.Map) uint64 {
	h := fnv.New64a()
	
	// Sort keys for deterministic hashing
	keys := make([]string, 0, attrs.Len())
	attrs.Range(func(k string, v pmetric.Value) bool {
		keys = append(keys, k)
		return true
	})
	
	// Sort keys (we'll implement this in a real version)
	// sort.Strings(keys)
	
	// Hash each key-value pair
	for _, k := range keys {
		v, _ := attrs.Get(k)
		h.Write([]byte(k))
		h.Write([]byte(v.AsString()))
	}
	
	return h.Sum64()
}

// newFactory creates a factory for the cardinality limiter processor
func NewFactory() processor.Factory {
	return processor.NewFactory(
		"cardinalitylimiter",
		createDefaultConfig,
		processor.WithMetrics(createMetricsProcessor),
	)
}

// createDefaultConfig creates the default configuration for the processor
func createDefaultConfig() component.Config {
	return &Config{
		MaxKeys:        65536,
		HighScore:      0.75,
		CriticalScore:  0.90,
		AggregateLabels: []string{"container.image.tag", "k8s.pod.uid"},
	}
}

// createMetricsProcessor creates a processor for metrics based on the config
func createMetricsProcessor(
	ctx context.Context,
	set processor.CreateSettings,
	cfg component.Config,
	nextConsumer component.Metrics,
) (processor.Metrics, error) {
	pCfg := cfg.(*Config)
	metricsProcessor := newCardinalityLimiterProcessor(set.Logger, pCfg)

	return processor.NewMetricsProcessor(ctx, set, cfg, nextConsumer,
		metricsProcessor.processMetrics)
}

// This is the plugin entry point
var (
	_ component.ConfigValidator = (*Config)(nil)
)

// Validate validates the processor configuration
func (cfg *Config) Validate() error {
	return nil
}

// Export the plugin factory function
func main() {}

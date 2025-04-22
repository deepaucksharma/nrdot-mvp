package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Configuration structure for the APQ
type APQConfig struct {
	Enabled bool              `mapstructure:"enabled"`
	Classes []PriorityClass   `mapstructure:"classes"`
}

// PriorityClass defines a priority class with weight and pattern
type PriorityClass struct {
	Name    string `mapstructure:"name"`
	Weight  int    `mapstructure:"weight"`
	Pattern string `mapstructure:"pattern"`
}

// AdaptivePriorityQueue implements a priority-based queue with WRR scheduling
type AdaptivePriorityQueue struct {
	// Separate queues for each priority class
	queues     [][]interface{}
	queueMutex sync.Mutex
	
	// Configuration
	capacity      int
	weights       []int
	classPatterns []*regexp.Regexp
	classNames    []string
	
	// Scheduling state
	currentClass    int32
	remainingTokens int32
	
	// Metrics
	fillRatio  prometheus.Gauge
	classSize  *prometheus.GaugeVec
	spillTotal *prometheus.CounterVec
	
	// For spilling
	spillFunc func(item interface{}) error
	logger    *zap.Logger
}

// metrics
var (
	apqFillRatioMetric = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "apq_fill_ratio",
			Help: "Current fill ratio of the APQ (0.0-1.0)",
		},
	)
	
	apqClassSizeMetric = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "apq_class_size",
			Help: "Current number of items in each priority class",
		},
		[]string{"class"},
	)
	
	apqSpillTotalMetric = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "apq_spill_total",
			Help: "Total number of items spilled from each priority class",
		},
		[]string{"class"},
	)
)

// NewAdaptivePriorityQueue creates a new APQ with the given configuration
func NewAdaptivePriorityQueue(capacity int, classes []PriorityClass, logger *zap.Logger) (*AdaptivePriorityQueue, error) {
	if capacity <= 0 {
		return nil, errors.New("queue capacity must be positive")
	}
	
	if len(classes) == 0 {
		// Default to single class if none specified
		classes = []PriorityClass{
			{Name: "default", Weight: 1, Pattern: ".*"},
		}
	}
	
	// Compile all patterns
	patterns := make([]*regexp.Regexp, len(classes))
	weights := make([]int, len(classes))
	names := make([]string, len(classes))
	
	for i, class := range classes {
		if class.Weight <= 0 {
			return nil, fmt.Errorf("class weight must be positive: %s", class.Name)
		}
		
		re, err := regexp.Compile(class.Pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern for class %s: %v", class.Name, err)
		}
		
		patterns[i] = re
		weights[i] = class.Weight
		names[i] = class.Name
	}
	
	queues := make([][]interface{}, len(classes))
	for i := range queues {
		queues[i] = make([]interface{}, 0, capacity/len(classes))
	}
	
	return &AdaptivePriorityQueue{
		queues:        queues,
		capacity:      capacity,
		weights:       weights,
		classPatterns: patterns,
		classNames:    names,
		currentClass:  0,
		fillRatio:     apqFillRatioMetric,
		classSize:     apqClassSizeMetric,
		spillTotal:    apqSpillTotalMetric,
		logger:        logger,
	}, nil
}

// SetSpillFunc sets the callback function for handling spilled items
func (q *AdaptivePriorityQueue) SetSpillFunc(f func(interface{}) error) {
	q.spillFunc = f
}

// Enqueue adds an item to the queue in the appropriate priority class
func (q *AdaptivePriorityQueue) Enqueue(item interface{}) error {
	// Determine which class this item belongs to
	classIdx := q.classifyItem(item)
	
	q.queueMutex.Lock()
	defer q.queueMutex.Unlock()
	
	// Check if we need to spill
	totalItems := q.getTotalSize()
	freeSlots := q.capacity - totalItems
	
	// Spill condition: less than 5% free space
	if float64(freeSlots)/float64(q.capacity) < 0.05 {
		// If we have a spill function, use it
		if q.spillFunc != nil {
			err := q.spillFunc(item)
			if err != nil {
				return fmt.Errorf("failed to spill item: %v", err)
			}
			// Increment spill counter
			q.spillTotal.WithLabelValues(q.classNames[classIdx]).Inc()
			return nil
		}
		return errors.New("queue full and no spill function defined")
	}
	
	// Add to appropriate queue
	q.queues[classIdx] = append(q.queues[classIdx], item)
	
	// Update metrics
	q.updateMetrics()
	
	return nil
}

// Dequeue removes and returns an item from the queue using WRR scheduling
func (q *AdaptivePriorityQueue) Dequeue() (interface{}, error) {
	q.queueMutex.Lock()
	defer q.queueMutex.Unlock()
	
	// Check if queue is empty
	if q.getTotalSize() == 0 {
		return nil, errors.New("queue is empty")
	}
	
	// Use WRR scheduling to select next class
	selectedClass := q.selectPriorityClass()
	
	// Get an item from the selected class
	if len(q.queues[selectedClass]) == 0 {
		// This shouldn't happen with proper selectPriorityClass implementation
		return nil, errors.New("selected queue is empty")
	}
	
	// Remove and return the first item
	item := q.queues[selectedClass][0]
	q.queues[selectedClass] = q.queues[selectedClass][1:]
	
	// Update metrics
	q.updateMetrics()
	
	return item, nil
}

// DequeueBlocking waits for an item to be available and then dequeues it
func (q *AdaptivePriorityQueue) DequeueBlocking(ctx context.Context) (interface{}, error) {
	for {
		// Try to dequeue
		item, err := q.Dequeue()
		if err == nil {
			return item, nil
		}
		
		// If queue is empty, wait briefly and check again
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// Continue and try again
		}
	}
}

// Size returns the total number of items in the queue
func (q *AdaptivePriorityQueue) Size() int {
	q.queueMutex.Lock()
	defer q.queueMutex.Unlock()
	return q.getTotalSize()
}

// getTotalSize returns the sum of all queue sizes (internal, no locking)
func (q *AdaptivePriorityQueue) getTotalSize() int {
	total := 0
	for _, queue := range q.queues {
		total += len(queue)
	}
	return total
}

// classifyItem determines which priority class an item belongs to
func (q *AdaptivePriorityQueue) classifyItem(item interface{}) int {
	// TODO: Implement proper classification based on item content
	// For now, return a default class (will be enhanced with actual pattern matching)
	// This is a key gap mentioned in the roadmap
	
	return 0 // Default to highest priority for MVP
}

// selectPriorityClass implements the WRR scheduling algorithm
func (q *AdaptivePriorityQueue) selectPriorityClass() int {
	// Get current class atomically
	currentClass := atomic.LoadInt32(&q.currentClass)
	remaining := atomic.LoadInt32(&q.remainingTokens)
	
	// If we have tokens left for this class, use them
	if remaining > 0 {
		// Decrement tokens and return current class
		atomic.StoreInt32(&q.remainingTokens, remaining-1)
		return int(currentClass)
	}
	
	// Find next non-empty class
	nextClassFound := false
	initialClass := currentClass
	for !nextClassFound {
		// Move to next class with wrap-around
		currentClass = (currentClass + 1) % int32(len(q.queues))
		
		// Avoid infinite loop if all queues are empty (shouldn't happen)
		if currentClass == initialClass {
			// Just use current class and let calling function handle empty queue
			break
		}
		
		// Check if this class has items
		if len(q.queues[currentClass]) > 0 {
			nextClassFound = true
		}
	}
	
	// Update tokens based on class weight
	atomic.StoreInt32(&q.remainingTokens, int32(q.weights[currentClass])-1)
	atomic.StoreInt32(&q.currentClass, currentClass)
	
	return int(currentClass)
}

// updateMetrics updates all the APQ metrics
func (q *AdaptivePriorityQueue) updateMetrics() {
	// Update fill ratio
	totalSize := q.getTotalSize()
	q.fillRatio.Set(float64(totalSize) / float64(q.capacity))
	
	// Update class sizes
	for i, queue := range q.queues {
		q.classSize.WithLabelValues(q.classNames[i]).Set(float64(len(queue)))
	}
}

// QueueItem wraps the data being processed in the queue
type QueueItem struct {
	req     consumer.Requests
	ctx     context.Context
	respCh  chan error
	attempt int
}

// APQSendingQueueFactory is a factory for APQ-enabled sending queues
type APQSendingQueueFactory struct{}

// NewFactory creates a new factory for APQ
func NewFactory() exporter.Factory {
	// This is a plugin stub; in real implementation, we would
	// hook into the exporter.Factory chain to provide our queue
	return nil
}

// Export the plugin factory function
func main() {}

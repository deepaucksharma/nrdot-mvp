package dlq

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"

	"github.com/klauspost/compress/zstd"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	// File format constants
	magicBytes     = "NRDQv1"
	headerSize     = 32 // Magic(6) + ItemCount(8) + SHA256(32-6-8=18 remaining)
	defaultMaxSize = 128 * 1024 * 1024 // 128 MiB per segment
	
	// Replay rates
	defaultReplayRateMiBps = 4  // 4 MiB/s
	replayTokenInterval    = 10 * time.Millisecond
)

// FileStorageConfig holds the configuration for the file-backed DLQ
type FileStorageConfig struct {
	Directory           string        `mapstructure:"directory"`
	MaxSegmentMiB       int           `mapstructure:"max_segment_mib"`
	VerificationInterval time.Duration `mapstructure:"verification_interval"`
}

// FileStorageExtension implements a file-based DLQ with SHA-256 verification
type FileStorageExtension struct {
	config           *FileStorageConfig
	logger           *zap.Logger
	currentSegment   *os.File
	currentSize      int64
	currentItemCount int64
	hasher           hash.Hash
	compressor       *zstd.Encoder
	mutex            sync.Mutex
	
	// Replay functionality
	replayQueue     []string // List of segments to replay
	replayActive    bool
	replayCtx       context.Context
	replayCancel    context.CancelFunc
	
	// Metrics
	utilizationRatio prometheus.Gauge
	oldestAgeSeconds prometheus.Gauge
	corruptedTotal   prometheus.Counter
}

// metrics
var (
	dlqUtilizationRatioMetric = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dlq_utilization_ratio",
			Help: "Current utilization ratio of the DLQ (0.0-1.0)",
		},
	)
	
	dlqOldestAgeSecondsMetric = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dlq_oldest_age_seconds",
			Help: "Age of the oldest item in the DLQ in seconds",
		},
	)
	
	dlqCorruptedTotalMetric = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "dlq_corrupted_total",
			Help: "Total number of corrupted segments detected",
		},
	)
)

// NewFileStorage creates a new file-backed DLQ extension
func NewFileStorage(config *FileStorageConfig, logger *zap.Logger) (*FileStorageExtension, error) {
	if config.Directory == "" {
		return nil, errors.New("directory must be specified for file storage")
	}
	
	// Set defaults if not specified
	if config.MaxSegmentMiB <= 0 {
		config.MaxSegmentMiB = defaultMaxSize / (1024 * 1024)
	}
	
	if config.VerificationInterval <= 0 {
		config.VerificationInterval = 10 * time.Minute
	}
	
	// Create directory if it doesn't exist
	if err := os.MkdirAll(config.Directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}
	
	// Initialize storage
	fs := &FileStorageExtension{
		config:           config,
		logger:           logger,
		hasher:           sha256.New(),
		utilizationRatio: dlqUtilizationRatioMetric,
		oldestAgeSeconds: dlqOldestAgeSecondsMetric,
		corruptedTotal:   dlqCorruptedTotalMetric,
	}
	
	// Initialize zstd compressor
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd encoder: %v", err)
	}
	fs.compressor = encoder
	
	return fs, nil
}

// Start the extension
func (fs *FileStorageExtension) Start(ctx context.Context, host component.Host) error {
	// Create a new segment if none exists
	if err := fs.rotateSegmentIfNeeded(); err != nil {
		return fmt.Errorf("failed to initialize segment: %v", err)
	}
	
	// Start verification loop
	go fs.verificationLoop(ctx)
	
	// Update metrics initially
	fs.updateMetrics()
	
	return nil
}

// Stop the extension
func (fs *FileStorageExtension) Stop(ctx context.Context) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	
	// Stop any active replay
	if fs.replayActive && fs.replayCancel != nil {
		fs.replayCancel()
	}
	
	// Close current segment
	if fs.currentSegment != nil {
		if err := fs.finalizeSegment(); err != nil {
			fs.logger.Error("Failed to finalize segment during shutdown", zap.Error(err))
		}
		if err := fs.currentSegment.Close(); err != nil {
			return fmt.Errorf("failed to close segment: %v", err)
		}
		fs.currentSegment = nil
	}
	
	return nil
}

// StoreItem persists an item to the DLQ
func (fs *FileStorageExtension) StoreItem(item []byte) error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	
	// Ensure we have an active segment
	if err := fs.rotateSegmentIfNeeded(); err != nil {
		return fmt.Errorf("failed to ensure active segment: %v", err)
	}
	
	// Compress the item
	compressed := fs.compressor.EncodeAll(item, nil)
	
	// Update hash
	fs.hasher.Write(compressed)
	
	// Write size and data
	sizeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(sizeBytes, uint32(len(compressed)))
	
	if _, err := fs.currentSegment.Write(sizeBytes); err != nil {
		return fmt.Errorf("failed to write item size: %v", err)
	}
	
	if _, err := fs.currentSegment.Write(compressed); err != nil {
		return fmt.Errorf("failed to write item data: %v", err)
	}
	
	// Update counters
	fs.currentSize += int64(len(sizeBytes) + len(compressed))
	fs.currentItemCount++
	
	// Rotate if needed
	if fs.currentSize >= int64(fs.config.MaxSegmentMiB*1024*1024) {
		if err := fs.rotateSegment(); err != nil {
			return fmt.Errorf("failed to rotate segment: %v", err)
		}
	}
	
	// Update metrics
	fs.updateMetrics()
	
	return nil
}

// StartReplay begins replaying items from the DLQ
func (fs *FileStorageExtension) StartReplay(ctx context.Context, callback func([]byte) error) error {
	fs.mutex.Lock()
	
	// Check if replay is already active
	if fs.replayActive {
		fs.mutex.Unlock()
		return errors.New("replay already in progress")
	}
	
	// Get list of segments to replay
	segments, err := fs.listSegments()
	if err != nil {
		fs.mutex.Unlock()
		return fmt.Errorf("failed to list segments: %v", err)
	}
	
	// No segments to replay
	if len(segments) == 0 {
		fs.mutex.Unlock()
		return nil
	}
	
	// Set up replay state
	fs.replayQueue = segments
	fs.replayActive = true
	fs.replayCtx, fs.replayCancel = context.WithCancel(ctx)
	fs.mutex.Unlock()
	
	// Start replay goroutine
	go fs.replayLoop(callback)
	
	return nil
}

// StopReplay stops an active replay
func (fs *FileStorageExtension) StopReplay() error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	
	if !fs.replayActive {
		return errors.New("no replay in progress")
	}
	
	// Cancel the context to stop the replay
	if fs.replayCancel != nil {
		fs.replayCancel()
	}
	
	fs.replayActive = false
	fs.replayQueue = nil
	
	return nil
}

// rotateSegmentIfNeeded creates a new segment if none exists
func (fs *FileStorageExtension) rotateSegmentIfNeeded() error {
	if fs.currentSegment == nil {
		return fs.rotateSegment()
	}
	return nil
}

// rotateSegment finalizes the current segment and creates a new one
func (fs *FileStorageExtension) rotateSegment() error {
	// Finalize current segment if it exists
	if fs.currentSegment != nil {
		if err := fs.finalizeSegment(); err != nil {
			return err
		}
		if err := fs.currentSegment.Close(); err != nil {
			return fmt.Errorf("failed to close segment: %v", err)
		}
	}
	
	// Create a new segment file
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	segmentPath := filepath.Join(fs.config.Directory, fmt.Sprintf("segment_%s.dlq", timestamp))
	
	file, err := os.Create(segmentPath)
	if err != nil {
		return fmt.Errorf("failed to create segment file: %v", err)
	}
	
	// Write placeholder header (will be updated when segment is finalized)
	placeholder := make([]byte, headerSize)
	copy(placeholder, magicBytes)
	if _, err := file.Write(placeholder); err != nil {
		file.Close()
		return fmt.Errorf("failed to write header placeholder: %v", err)
	}
	
	// Reset state for new segment
	fs.currentSegment = file
	fs.currentSize = int64(headerSize)
	fs.currentItemCount = 0
	fs.hasher = sha256.New()
	
	return nil
}

// finalizeSegment updates the segment header with final hash and count
func (fs *FileStorageExtension) finalizeSegment() error {
	// Get current hash
	hash := fs.hasher.Sum(nil)
	
	// Seek to start of file
	if _, err := fs.currentSegment.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to start: %v", err)
	}
	
	// Write magic bytes
	if _, err := fs.currentSegment.Write([]byte(magicBytes)); err != nil {
		return fmt.Errorf("failed to write magic bytes: %v", err)
	}
	
	// Write item count
	countBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(countBytes, uint64(fs.currentItemCount))
	if _, err := fs.currentSegment.Write(countBytes); err != nil {
		return fmt.Errorf("failed to write item count: %v", err)
	}
	
	// Write hash (truncate if needed to fit header size)
	hashSize := headerSize - len(magicBytes) - len(countBytes)
	if _, err := fs.currentSegment.Write(hash[:hashSize]); err != nil {
		return fmt.Errorf("failed to write hash: %v", err)
	}
	
	// Sync to disk
	return fs.currentSegment.Sync()
}

// listSegments returns a list of segment files in the storage directory
func (fs *FileStorageExtension) listSegments() ([]string, error) {
	entries, err := os.ReadDir(fs.config.Directory)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %v", err)
	}
	
	segments := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".dlq" {
			segments = append(segments, filepath.Join(fs.config.Directory, entry.Name()))
		}
	}
	
	return segments, nil
}

// verificationLoop periodically verifies the integrity of segments
func (fs *FileStorageExtension) verificationLoop(ctx context.Context) {
	ticker := time.NewTicker(fs.config.VerificationInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fs.verifySegments()
		}
	}
}

// verifySegments checks all segments for corruption
func (fs *FileStorageExtension) verifySegments() {
	segments, err := fs.listSegments()
	if err != nil {
		fs.logger.Error("Failed to list segments for verification", zap.Error(err))
		return
	}
	
	for _, segmentPath := range segments {
		// Skip current active segment
		if fs.currentSegment != nil {
			currentPath, err := filepath.Abs(fs.currentSegment.Name())
			if err == nil {
				absPath, err := filepath.Abs(segmentPath)
				if err == nil && currentPath == absPath {
					continue
				}
			}
		}
		
		// Verify this segment
		if err := fs.verifySegment(segmentPath); err != nil {
			fs.logger.Error("Segment verification failed", 
				zap.String("segment", segmentPath),
				zap.Error(err))
			fs.corruptedTotal.Inc()
			
			// TODO: Move corrupted file to quarantine directory
		}
	}
}

// verifySegment checks a single segment for corruption
func (fs *FileStorageExtension) verifySegment(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open segment: %v", err)
	}
	defer file.Close()
	
	// Read header
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(file, header); err != nil {
		return fmt.Errorf("failed to read header: %v", err)
	}
	
	// Verify magic bytes
	if string(header[:len(magicBytes)]) != magicBytes {
		return errors.New("invalid magic bytes")
	}
	
	// Get item count
	itemCount := binary.BigEndian.Uint64(header[len(magicBytes):len(magicBytes)+8])
	
	// Get stored hash
	storedHash := header[len(magicBytes)+8:]
	
	// Calculate hash of data
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("failed to hash data: %v", err)
	}
	
	calculatedHash := hasher.Sum(nil)
	
	// Compare hashes (only the stored portion)
	if !bytes.Equal(calculatedHash[:len(storedHash)], storedHash) {
		return errors.New("hash verification failed")
	}
	
	return nil
}

// replayLoop processes segments for replay with rate limiting
func (fs *FileStorageExtension) replayLoop(callback func([]byte) error) {
	// Set up rate limiter using token bucket
	bytesPerToken := int64(fs.config.MaxSegmentMiB * 1024 * 1024 / 100) // ~1% of segment size per token
	if bytesPerToken < 4096 {
		bytesPerToken = 4096 // Minimum 4 KiB per token
	}
	
	availableBytes := int64(0)
	tokenTicker := time.NewTicker(replayTokenInterval)
	defer tokenTicker.Stop()
	
	// Calculate bytes per token interval
	bytesPerInterval := int64(defaultReplayRateMiBps * 1024 * 1024 * 
		replayTokenInterval.Seconds())
	
	// Live vs replay flag (for 1:1 interleaving)
	processLive := true
	toggleTicker := time.NewTicker(500 * time.Millisecond)
	defer toggleTicker.Stop()
	
	// Main replay loop
	for {
		select {
		case <-fs.replayCtx.Done():
			// Replay was cancelled
			fs.mutex.Lock()
			fs.replayActive = false
			fs.replayQueue = nil
			fs.mutex.Unlock()
			return
			
		case <-toggleTicker.C:
			// Toggle between live and replay
			processLive = !processLive
			if processLive {
				// When processing live, just sleep and continue
				continue
			}
			
		case <-tokenTicker.C:
			// Add token
			availableBytes += bytesPerInterval
			
			// Skip if we're in live mode
			if processLive {
				continue
			}
			
			// Process items as long as we have tokens and items
			for availableBytes > 0 {
				// Get next segment
				fs.mutex.Lock()
				if len(fs.replayQueue) == 0 {
					// No more segments to replay
					fs.replayActive = false
					fs.replayQueue = nil
					fs.mutex.Unlock()
					return
				}
				
				segmentPath := fs.replayQueue[0]
				fs.mutex.Unlock()
				
				// Process some items from this segment
				processedBytes, done, err := fs.processSomeItems(segmentPath, availableBytes, callback)
				if err != nil {
					fs.logger.Error("Error processing segment", 
						zap.String("segment", segmentPath),
						zap.Error(err))
					
					// Move to next segment
					fs.mutex.Lock()
					if len(fs.replayQueue) > 0 {
						fs.replayQueue = fs.replayQueue[1:]
					}
					fs.mutex.Unlock()
					break
				}
				
				// Update available bytes
				availableBytes -= processedBytes
				
				// If segment is done, move to next
				if done {
					fs.mutex.Lock()
					if len(fs.replayQueue) > 0 {
						fs.replayQueue = fs.replayQueue[1:]
					}
					fs.mutex.Unlock()
				}
				
				// If we've used all tokens, break
				if availableBytes <= 0 {
					break
				}
			}
		}
	}
}

// processSomeItems replays a batch of items from a segment
func (fs *FileStorageExtension) processSomeItems(segmentPath string, maxBytes int64, callback func([]byte) error) (int64, bool, error) {
	// Open the segment file
	file, err := os.Open(segmentPath)
	if err != nil {
		return 0, true, fmt.Errorf("failed to open segment: %v", err)
	}
	defer file.Close()
	
	// Skip header
	if _, err := file.Seek(int64(headerSize), io.SeekStart); err != nil {
		return 0, true, fmt.Errorf("failed to seek past header: %v", err)
	}
	
	// Set up zstd decoder
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return 0, true, fmt.Errorf("failed to create zstd decoder: %v", err)
	}
	defer decoder.Close()
	
	// Process items until we hit the byte limit
	var processedBytes int64
	for processedBytes < maxBytes {
		// Read item size
		sizeBytes := make([]byte, 4)
		if _, err := io.ReadFull(file, sizeBytes); err != nil {
			if err == io.EOF {
				// End of file, segment is done
				return processedBytes, true, nil
			}
			return processedBytes, false, fmt.Errorf("failed to read item size: %v", err)
		}
		
		itemSize := binary.BigEndian.Uint32(sizeBytes)
		
		// Read item data
		compressedData := make([]byte, itemSize)
		if _, err := io.ReadFull(file, compressedData); err != nil {
			return processedBytes, false, fmt.Errorf("failed to read item data: %v", err)
		}
		
		// Decompress
		data, err := decoder.DecodeAll(compressedData, nil)
		if err != nil {
			return processedBytes, false, fmt.Errorf("failed to decompress item: %v", err)
		}
		
		// Process the item
		if err := callback(data); err != nil {
			return processedBytes, false, fmt.Errorf("callback failed: %v", err)
		}
		
		// Update bytes processed
		processedBytes += int64(len(sizeBytes) + len(compressedData))
		
		// Stop if we've hit the limit
		if processedBytes >= maxBytes {
			// Save position for next call
			return processedBytes, false, nil
		}
	}
	
	return processedBytes, false, nil
}

// updateMetrics updates all DLQ metrics
func (fs *FileStorageExtension) updateMetrics() {
	// Get all segments
	segments, err := fs.listSegments()
	if err != nil {
		fs.logger.Error("Failed to list segments for metrics", zap.Error(err))
		return
	}
	
	// Calculate total size
	var totalSize int64
	var oldestTime time.Time
	
	for _, segment := range segments {
		info, err := os.Stat(segment)
		if err != nil {
			continue
		}
		
		totalSize += info.Size()
		
		// Track oldest segment time
		if oldestTime.IsZero() || info.ModTime().Before(oldestTime) {
			oldestTime = info.ModTime()
		}
	}
	
	// Update metrics
	maxCapacity := int64(fs.config.MaxSegmentMiB * 1024 * 1024 * 100) // Assume max 100 segments
	fs.utilizationRatio.Set(float64(totalSize) / float64(maxCapacity))
	
	if !oldestTime.IsZero() {
		fs.oldestAgeSeconds.Set(float64(time.Since(oldestTime).Seconds()))
	} else {
		fs.oldestAgeSeconds.Set(0)
	}
}

// NewFactory creates a factory for File Storage extension
func NewFactory() extension.Factory {
	// This would be implemented to register the extension
	return nil
}

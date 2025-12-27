package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Manager manages all camera queues and global memory limits
type Manager struct {
	queues       map[string]*Queue
	globalConfig GlobalQueueConfig
	mu           sync.RWMutex

	// Memory monitoring
	currentTotalSize int64
	logger           Logger
}

// NewManager creates a new queue manager
func NewManager(config GlobalQueueConfig, logger Logger) (*Manager, error) {
	if logger == nil {
		logger = &defaultLogger{}
	}

	// Ensure base path exists
	if err := os.MkdirAll(config.BasePath, 0755); err != nil {
		return nil, fmt.Errorf("create base path: %w", err)
	}

	return &Manager{
		queues:       make(map[string]*Queue),
		globalConfig: config,
		logger:       logger,
	}, nil
}

// CreateQueue creates a new queue for a camera
func (m *Manager) CreateQueue(cameraID string, config QueueConfig) (*Queue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.queues[cameraID]; exists {
		return nil, fmt.Errorf("queue already exists for camera: %s", cameraID)
	}

	directory := filepath.Join(m.globalConfig.BasePath, cameraID)
	queue, err := NewQueue(cameraID, directory, config, m.logger)
	if err != nil {
		return nil, fmt.Errorf("create queue: %w", err)
	}

	m.queues[cameraID] = queue
	m.logger.Info("Queue created",
		"camera", cameraID,
		"directory", directory)

	return queue, nil
}

// GetQueue returns a queue by camera ID
func (m *Manager) GetQueue(cameraID string) (*Queue, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	q, ok := m.queues[cameraID]
	return q, ok
}

// RemoveQueue removes a queue for a camera
func (m *Manager) RemoveQueue(cameraID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	q, exists := m.queues[cameraID]
	if !exists {
		return fmt.Errorf("queue not found: %s", cameraID)
	}

	delete(m.queues, cameraID)

	// Optionally remove the directory
	state := q.GetState()
	if err := os.RemoveAll(state.Directory); err != nil {
		m.logger.Warn("Failed to remove queue directory",
			"camera", cameraID,
			"directory", state.Directory,
			"error", err)
	}

	return nil
}

// GetAllQueues returns all queues
func (m *Manager) GetAllQueues() []*Queue {
	m.mu.RLock()
	defer m.mu.RUnlock()

	queues := make([]*Queue, 0, len(m.queues))
	for _, q := range m.queues {
		queues = append(queues, q)
	}
	return queues
}

// GetGlobalStats returns global statistics
func (m *Manager) GetGlobalStats() GlobalQueueStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var totalImages int
	var totalSize int64
	cameraStats := make([]QueueStats, 0, len(m.queues))

	for _, q := range m.queues {
		stats := q.GetStats()
		cameraStats = append(cameraStats, stats)
		totalImages += stats.ImageCount
		totalSize += int64(stats.TotalSizeMB * 1024 * 1024)
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return GlobalQueueStats{
		TotalImages:   totalImages,
		TotalSizeMB:   float64(totalSize) / (1024 * 1024),
		CameraStats:   cameraStats,
		MemoryUsageMB: float64(mem.HeapAlloc) / (1024 * 1024),
		MemoryLimitMB: m.globalConfig.MaxHeapMB,
	}
}

// StartMemoryMonitor starts the background memory monitoring goroutine
func (m *Manager) StartMemoryMonitor(ctx context.Context) {
	interval := time.Duration(m.globalConfig.MemoryCheckSeconds) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	m.logger.Info("Memory monitor started",
		"interval_seconds", m.globalConfig.MemoryCheckSeconds,
		"max_total_size_mb", m.globalConfig.MaxTotalSizeMB,
		"max_heap_mb", m.globalConfig.MaxHeapMB)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Memory monitor stopped")
			return
		case <-ticker.C:
			m.checkMemoryPressure()
		}
	}
}

// checkMemoryPressure checks global memory usage and triggers emergency thinning if needed
func (m *Manager) checkMemoryPressure() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Calculate total queue size across all cameras
	var totalSize int64
	for _, q := range m.queues {
		state := q.GetState()
		totalSize += state.TotalSizeBytes
	}

	m.currentTotalSize = totalSize
	maxBytes := int64(m.globalConfig.MaxTotalSizeMB) * 1024 * 1024

	// Check queue size limit
	if totalSize > maxBytes {
		m.logger.Warn("Global queue size exceeded, triggering emergency thin",
			"total_mb", float64(totalSize)/(1024*1024),
			"max_mb", m.globalConfig.MaxTotalSizeMB)

		// Emergency thin all queues
		for _, q := range m.queues {
			q.EmergencyThin(m.globalConfig.EmergencyThinRatio)
		}
	}

	// Check system memory (heap usage)
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	maxHeapBytes := uint64(m.globalConfig.MaxHeapMB) * 1024 * 1024
	if mem.HeapAlloc > maxHeapBytes {
		m.logger.Warn("System memory pressure detected",
			"heap_mb", float64(mem.HeapAlloc)/(1024*1024),
			"max_heap_mb", m.globalConfig.MaxHeapMB)

		// Aggressive emergency thin
		for _, q := range m.queues {
			q.EmergencyThin(0.3) // Keep only 30%
		}

		// Force garbage collection
		runtime.GC()
	}
}

// ExpireAllOldImages runs expiration on all queues
func (m *Manager) ExpireAllOldImages() int {
	m.mu.RLock()
	queues := make([]*Queue, 0, len(m.queues))
	for _, q := range m.queues {
		queues = append(queues, q)
	}
	m.mu.RUnlock()

	total := 0
	for _, q := range queues {
		total += q.ExpireOldImages()
	}
	return total
}

// StartExpirationWorker starts a background worker that periodically expires old images
func (m *Manager) StartExpirationWorker(ctx context.Context, interval time.Duration) {
	if interval < time.Minute {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	m.logger.Info("Expiration worker started",
		"interval", interval.String())

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Expiration worker stopped")
			return
		case <-ticker.C:
			expired := m.ExpireAllOldImages()
			if expired > 0 {
				m.logger.Info("Expiration worker completed",
					"expired", expired)
			}
		}
	}
}

// GetTotalQueueSize returns the total size of all queues in bytes
func (m *Manager) GetTotalQueueSize() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total int64
	for _, q := range m.queues {
		state := q.GetState()
		total += state.TotalSizeBytes
	}
	return total
}

// GetTotalImageCount returns the total number of images across all queues
func (m *Manager) GetTotalImageCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total int
	for _, q := range m.queues {
		total += q.GetImageCount()
	}
	return total
}

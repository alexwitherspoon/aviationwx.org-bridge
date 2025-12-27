package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/alexwitherspoon/aviationwx-bridge/internal/camera"
	"github.com/alexwitherspoon/aviationwx-bridge/internal/image"
	timehealth "github.com/alexwitherspoon/aviationwx-bridge/internal/time"
	"github.com/alexwitherspoon/aviationwx-bridge/internal/upload"
)

// CameraConfig represents camera configuration needed by scheduler
type CameraConfig struct {
	ID             string
	RemotePath     string
	Enabled        bool
	ImageProcessor *image.Processor // Optional image processor for resize/quality
}

// Scheduler manages the capture and upload cycle for all cameras
type Scheduler struct {
	cameras       []camera.Camera
	cameraConfigs map[string]CameraConfig // Map camera ID to config
	uploader      upload.Client
	timeHealth    *timehealth.TimeHealth // Time health manager for EXIF stamping
	config        Config
	ctx           context.Context
	cancel        context.CancelFunc
	cameraState   map[string]*CameraState
	degradedMode  *DegradedMode
	mu            sync.RWMutex // Protects cameraState
}

// Config represents scheduler configuration
type Config struct {
	IntervalSeconds int // Base interval between capture attempts (default: 60)
	GlobalTimeout   int // Global timeout for operations (seconds)
}

// CameraState tracks the state of a single camera
type CameraState struct {
	CameraID       string
	LastSuccess    time.Time
	LastError      error
	LastErrorTime  time.Time
	NextAttempt    time.Time
	BackoffSeconds int
	FailureCount   int
	SuccessCount   int
	IsBackingOff   bool
}

// Status represents the current status of the scheduler
type Status struct {
	Running       bool
	CameraCount   int
	CameraStates  []CameraState
	DegradedMode  bool
	LastCycleTime time.Time
}

// NewScheduler creates a new scheduler instance
func NewScheduler(cameras []camera.Camera, cameraConfigs map[string]CameraConfig, uploader upload.Client, timeHealth *timehealth.TimeHealth, config Config) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Apply defaults
	if config.IntervalSeconds == 0 {
		config.IntervalSeconds = 60 // Default 60 seconds
	}
	if config.GlobalTimeout == 0 {
		config.GlobalTimeout = 120 // Default 2 minutes
	}
	
	return &Scheduler{
		cameras:       cameras,
		cameraConfigs: cameraConfigs,
		uploader:      uploader,
		timeHealth:    timeHealth,
		config:        config,
		ctx:           ctx,
		cancel:        cancel,
		cameraState:   make(map[string]*CameraState),
		degradedMode:  NewDegradedMode(DefaultDegradedConfig()),
	}
}

// Start begins the scheduler loop
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize camera states
	for _, cam := range s.cameras {
		s.cameraState[cam.ID()] = &CameraState{
			CameraID:       cam.ID(),
			NextAttempt:    time.Now(), // Start immediately
			BackoffSeconds: 0,
		}
	}

	// Start scheduler loop in goroutine
	go s.run()

	return nil
}

// Stop stops the scheduler gracefully
func (s *Scheduler) Stop() {
	s.cancel()
}

// GetStatus returns the current scheduler status
func (s *Scheduler) GetStatus() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	states := make([]CameraState, 0, len(s.cameraState))
	for _, state := range s.cameraState {
		states = append(states, *state)
	}

	return Status{
		Running:      s.ctx.Err() == nil,
		CameraCount:  len(s.cameras),
		CameraStates: states,
		DegradedMode: s.degradedMode.IsActive(),
	}
}

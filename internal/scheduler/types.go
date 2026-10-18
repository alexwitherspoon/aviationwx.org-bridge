package scheduler

import (
	"time"

	"github.com/alexwitherspoon/aviationwx-bridge/internal/image"
)

// CameraConfig represents camera configuration needed by scheduler
type CameraConfig struct {
	ID             string
	RemotePath     string
	Enabled        bool
	ImageProcessor *image.Processor // Optional image processor for resize/quality
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

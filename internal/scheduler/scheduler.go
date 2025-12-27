package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/alexwitherspoon/aviationwx-bridge/internal/camera"
	timehealth "github.com/alexwitherspoon/aviationwx-bridge/internal/time"
)

// run is the main scheduler loop
func (s *Scheduler) run() {
	baseInterval := time.Duration(s.config.IntervalSeconds) * time.Second
	ticker := time.NewTicker(baseInterval)
	defer ticker.Stop()

	// Initial immediate run
	s.processCameras()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.processCameras()

			// Adjust ticker interval based on degraded mode
			multiplier := s.degradedMode.GetIntervalMultiplier()
			newInterval := time.Duration(float64(s.config.IntervalSeconds)*multiplier) * time.Second
			if newInterval != baseInterval {
				ticker.Reset(newInterval)
			}
		}
	}
}

// processCameras processes all cameras that are ready
func (s *Scheduler) processCameras() {
	s.mu.RLock()
	cameras := make([]struct {
		cam   camera.Camera
		state *CameraState
	}, 0)

	for _, cam := range s.cameras {
		state := s.cameraState[cam.ID()]
		if state == nil {
			continue
		}

		// Check if camera should attempt now
		if !ShouldAttempt(state) {
			continue
		}

		cameras = append(cameras, struct {
			cam   camera.Camera
			state *CameraState
		}{cam: cam, state: state})
	}
	s.mu.RUnlock()

	// Check degraded mode concurrency limit
	concurrencyLimit := s.degradedMode.GetConcurrencyLimit()

	var wg sync.WaitGroup
	var semaphore chan struct{}

	// Create semaphore only if we have a concurrency limit
	if concurrencyLimit > 0 {
		semaphore = make(chan struct{}, concurrencyLimit)
	}

	for _, item := range cameras {
		wg.Add(1)
		go func(c camera.Camera, st *CameraState) {
			defer wg.Done()

			// Acquire semaphore if in degraded mode
			if semaphore != nil {
				semaphore <- struct{}{}
				defer func() { <-semaphore }()
			}

			s.processCamera(c, st)
		}(item.cam, item.state)
	}

	// Wait for all cameras to complete
	wg.Wait()
}

// processCamera handles capture and upload for a single camera
func (s *Scheduler) processCamera(cam camera.Camera, state *CameraState) {
	// Check if camera is enabled
	camConfig, ok := s.cameraConfigs[cam.ID()]
	if !ok || !camConfig.Enabled {
		return
	}

	// Create context with timeout for this operation
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(s.config.GlobalTimeout)*time.Second)
	defer cancel()

	// CRITICAL: Always capture fresh - never use cached images
	// This ensures we never upload stale data
	imageData, err := cam.Capture(ctx)
	if err != nil {
		// Capture failed
		s.mu.Lock()
		state.LastError = err
		state.LastErrorTime = time.Now()
		UpdateBackoff(state, DefaultBackoffConfig())
		s.mu.Unlock()

		// Record failure for degraded mode
		s.degradedMode.RecordFailure()
		return
	}

	// Capture succeeded - reset backoff
	s.mu.Lock()
	ResetBackoff(state)
	state.LastSuccess = time.Now()
	s.mu.Unlock()
	
	// Stamp EXIF timestamp if time is healthy
	// This happens before upload to ensure timestamp is accurate
	captureTime := time.Now()
	if s.timeHealth != nil {
		stampedData, err := timehealth.StampEXIF(imageData, s.timeHealth, captureTime)
		if err == nil {
			// Use stamped data (may be same as original if EXIF not implemented yet)
			imageData = stampedData
		}
		// If EXIF stamping fails, continue with original data (fail gracefully)
	}
	
	// Upload the fresh image
	// Use remote path from config, default to camera ID if not set
	remotePath := camConfig.RemotePath
	if remotePath == "" {
		remotePath = fmt.Sprintf("%s/latest.jpg", cam.ID())
	} else {
		// Ensure path ends with /latest.jpg
		if remotePath[len(remotePath)-1] != '/' {
			remotePath += "/"
		}
		remotePath += "latest.jpg"
	}
	
	uploadErr := s.uploader.Upload(remotePath, imageData)
	if uploadErr != nil {
		// Upload failed - but we already captured fresh image
		// Don't retry with same image (freshness requirement)
		s.mu.Lock()
		state.LastError = uploadErr
		state.LastErrorTime = time.Now()
		UpdateBackoff(state, DefaultBackoffConfig())
		s.mu.Unlock()

		// Record failure for degraded mode
		s.degradedMode.RecordFailure()
		return
	}

	// Success - both capture and upload succeeded
	s.mu.Lock()
	state.LastSuccess = time.Now()
	state.LastError = nil
	state.LastErrorTime = time.Time{}
	s.mu.Unlock()

	// Record success for degraded mode
	s.degradedMode.RecordSuccess()
}

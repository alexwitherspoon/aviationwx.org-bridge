package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alexwitherspoon/aviationwx-bridge/internal/camera"
	timehealth "github.com/alexwitherspoon/aviationwx-bridge/internal/time"
)

// mockCamera is a mock camera for testing
type mockCamera struct {
	id      string
	camType string
	data    []byte
	err     error
}

func (m *mockCamera) Capture(ctx context.Context) ([]byte, error) {
	return m.data, m.err
}

func (m *mockCamera) ID() string {
	return m.id
}

func (m *mockCamera) Type() string {
	return m.camType
}

// mockUploader is a mock upload client for testing
type mockUploader struct {
	err error
}

func (m *mockUploader) Upload(remotePath string, data []byte) error {
	return m.err
}

func (m *mockUploader) TestConnection() error {
	return m.err
}

func TestNewScheduler(t *testing.T) {
	cameras := []camera.Camera{
		&mockCamera{id: "cam1", camType: "http"},
	}
	configs := map[string]CameraConfig{
		"cam1": {ID: "cam1", RemotePath: "test/cam1", Enabled: true},
	}
	uploader := &mockUploader{}
	timeHealth := &timehealth.TimeHealth{} // Mock time health (will need proper initialization in real usage)
	
	s := NewScheduler(cameras, configs, uploader, timeHealth, Config{})

	if s == nil {
		t.Fatal("NewScheduler() returned nil")
	}
	if s.config.IntervalSeconds != 60 {
		t.Errorf("IntervalSeconds = %d, want 60", s.config.IntervalSeconds)
	}
	if s.config.GlobalTimeout != 120 {
		t.Errorf("GlobalTimeout = %d, want 120", s.config.GlobalTimeout)
	}
}

func TestScheduler_Start(t *testing.T) {
	cameras := []camera.Camera{
		&mockCamera{id: "cam1", camType: "http"},
	}
	configs := map[string]CameraConfig{
		"cam1": {ID: "cam1", RemotePath: "test/cam1", Enabled: true},
	}
	uploader := &mockUploader{}

	s := NewScheduler(cameras, configs, uploader, nil, Config{IntervalSeconds: 1})

	err := s.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give it a moment to initialize
	time.Sleep(50 * time.Millisecond)

	// Check that camera state was initialized
	status := s.GetStatus()
	if status.CameraCount != 1 {
		t.Errorf("CameraCount = %d, want 1", status.CameraCount)
	}
	if len(status.CameraStates) != 1 {
		t.Errorf("CameraStates length = %d, want 1", len(status.CameraStates))
	}

	// Stop scheduler
	s.Stop()

	// Wait for shutdown
	time.Sleep(100 * time.Millisecond)

	// Check that it stopped
	status = s.GetStatus()
	if status.Running {
		t.Error("Scheduler should be stopped")
	}
}

func TestScheduler_Stop(t *testing.T) {
	cameras := []camera.Camera{
		&mockCamera{id: "cam1", camType: "http"},
	}
	configs := map[string]CameraConfig{
		"cam1": {ID: "cam1", RemotePath: "test/cam1", Enabled: true},
	}
	uploader := &mockUploader{}

	s := NewScheduler(cameras, configs, uploader, nil, Config{IntervalSeconds: 1})
	s.Start()

	// Stop scheduler
	s.Stop()

	// Wait for shutdown
	time.Sleep(100 * time.Millisecond)

	// Verify context is cancelled
	select {
	case <-s.ctx.Done():
		// Good, context is cancelled
	default:
		t.Error("Context should be cancelled after Stop()")
	}
}

func TestScheduler_GetStatus(t *testing.T) {
	cameras := []camera.Camera{
		&mockCamera{id: "cam1", camType: "http"},
		&mockCamera{id: "cam2", camType: "onvif"},
	}
	configs := map[string]CameraConfig{
		"cam1": {ID: "cam1", RemotePath: "test/cam1", Enabled: true},
		"cam2": {ID: "cam2", RemotePath: "test/cam2", Enabled: true},
	}
	uploader := &mockUploader{}

	s := NewScheduler(cameras, configs, uploader, nil, Config{})
	s.Start()

	status := s.GetStatus()

	if status.CameraCount != 2 {
		t.Errorf("CameraCount = %d, want 2", status.CameraCount)
	}
	if len(status.CameraStates) != 2 {
		t.Errorf("CameraStates length = %d, want 2", len(status.CameraStates))
	}
	if !status.Running {
		t.Error("Scheduler should be running")
	}

	s.Stop()
}

func TestScheduler_ProcessCamera_Success(t *testing.T) {
	cameras := []camera.Camera{
		&mockCamera{
			id:      "cam1",
			camType: "http",
			data:    []byte("test image data"),
			err:     nil,
		},
	}
	configs := map[string]CameraConfig{
		"cam1": {ID: "cam1", RemotePath: "test/cam1", Enabled: true},
	}
	uploader := &mockUploader{err: nil}

	s := NewScheduler(cameras, configs, uploader, nil, Config{GlobalTimeout: 5})
	s.Start()

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	status := s.GetStatus()
	if len(status.CameraStates) != 1 {
		t.Fatalf("CameraStates length = %d, want 1", len(status.CameraStates))
	}

	state := status.CameraStates[0]
	if state.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", state.FailureCount)
	}
	if state.IsBackingOff {
		t.Error("Should not be backing off after success")
	}
	if state.LastSuccess.IsZero() {
		t.Error("LastSuccess should be set")
	}

	s.Stop()
}

func TestScheduler_ProcessCamera_CaptureFailure(t *testing.T) {
	captureErr := errors.New("capture failed")
	cameras := []camera.Camera{
		&mockCamera{
			id:      "cam1",
			camType: "http",
			data:    nil,
			err:     captureErr,
		},
	}
	configs := map[string]CameraConfig{
		"cam1": {ID: "cam1", RemotePath: "test/cam1", Enabled: true},
	}
	uploader := &mockUploader{}

	s := NewScheduler(cameras, configs, uploader, nil, Config{GlobalTimeout: 5})
	s.Start()

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	status := s.GetStatus()
	state := status.CameraStates[0]

	if state.FailureCount == 0 {
		t.Error("FailureCount should be > 0 after capture failure")
	}
	if !state.IsBackingOff {
		t.Error("Should be backing off after failure")
	}
	if state.LastError == nil {
		t.Error("LastError should be set")
	}

	s.Stop()
}

func TestScheduler_ProcessCamera_UploadFailure(t *testing.T) {
	uploadErr := errors.New("upload failed")
	cameras := []camera.Camera{
		&mockCamera{
			id:      "cam1",
			camType: "http",
			data:    []byte("test image data"),
			err:     nil,
		},
	}
	configs := map[string]CameraConfig{
		"cam1": {ID: "cam1", RemotePath: "test/cam1", Enabled: true},
	}
	uploader := &mockUploader{err: uploadErr}

	s := NewScheduler(cameras, configs, uploader, nil, Config{GlobalTimeout: 5})
	s.Start()

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	status := s.GetStatus()
	state := status.CameraStates[0]

	if state.FailureCount == 0 {
		t.Error("FailureCount should be > 0 after upload failure")
	}
	if !state.IsBackingOff {
		t.Error("Should be backing off after failure")
	}
	if state.LastError == nil {
		t.Error("LastError should be set")
	}

	s.Stop()
}

func TestScheduler_ProcessCamera_Disabled(t *testing.T) {
	cameras := []camera.Camera{
		&mockCamera{id: "cam1", camType: "http"},
	}
	configs := map[string]CameraConfig{
		"cam1": {ID: "cam1", RemotePath: "test/cam1", Enabled: false},
	}
	uploader := &mockUploader{}

	s := NewScheduler(cameras, configs, uploader, nil, Config{GlobalTimeout: 5})
	s.Start()

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	status := s.GetStatus()
	state := status.CameraStates[0]

	// Disabled camera should not have been processed
	if !state.LastSuccess.IsZero() {
		t.Error("Disabled camera should not have LastSuccess set")
	}

	s.Stop()
}

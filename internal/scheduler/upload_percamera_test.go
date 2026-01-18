package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexwitherspoon/aviationwx-bridge/internal/queue"
)

// TestUploadWorker_PerCameraUploaders tests that each camera uses its own uploader
func TestUploadWorker_PerCameraUploaders(t *testing.T) {
	// Create temp directory for queues
	tmpDir, err := os.MkdirTemp("", "test-upload-worker-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create upload worker
	workerCfg := UploadWorkerConfig{
		MinUploadInterval: 10 * time.Millisecond,
		AuthBackoff:       100 * time.Millisecond,
		RetryDelay:        10 * time.Millisecond,
		Logger:            nil,
	}
	worker := NewUploadWorker(workerCfg)

	// Create mock uploaders for two cameras
	uploader1 := &mockUploader{err: nil}
	uploader2 := &mockUploader{err: fmt.Errorf("uploader 2 error")}

	// Create queues for two cameras
	queueConfig := queue.DefaultQueueConfig()
	queueMgr, err := queue.NewManager(queue.GlobalQueueConfig{
		BasePath:           tmpDir,
		MaxTotalSizeMB:     10,
		MaxHeapMB:          50,
		MemoryCheckSeconds: 60,
		EmergencyThinRatio: 0.5,
	}, nil)
	if err != nil {
		t.Fatalf("Failed to create queue manager: %v", err)
	}

	q1, err := queueMgr.CreateQueue("cam1", queueConfig)
	if err != nil {
		t.Fatalf("Failed to create queue for cam1: %v", err)
	}

	q2, err := queueMgr.CreateQueue("cam2", queueConfig)
	if err != nil {
		t.Fatalf("Failed to create queue for cam2: %v", err)
	}

	// Add queues with different uploaders
	config1 := CameraConfig{ID: "cam1", RemotePath: "test/cam1", Enabled: true}
	config2 := CameraConfig{ID: "cam2", RemotePath: "test/cam2", Enabled: true}

	worker.AddQueue("cam1", q1, config1, uploader1)
	worker.AddQueue("cam2", q2, config2, uploader2)

	// Verify both cameras are registered
	if len(worker.uploaders) != 2 {
		t.Errorf("Expected 2 uploaders, got %d", len(worker.uploaders))
	}
	if len(worker.queueOrder) != 2 {
		t.Errorf("Expected 2 cameras in queue order, got %d", len(worker.queueOrder))
	}

	// Verify each camera has its own uploader
	if worker.uploaders["cam1"] != uploader1 {
		t.Error("cam1 should have uploader1")
	}
	if worker.uploaders["cam2"] != uploader2 {
		t.Error("cam2 should have uploader2")
	}
}

// TestUploadWorker_AddQueue tests adding a queue to the upload worker
func TestUploadWorker_AddQueue(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-add-queue-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	worker := NewUploadWorker(UploadWorkerConfig{})
	uploader := &mockUploader{}

	queueMgr, err := queue.NewManager(queue.GlobalQueueConfig{
		BasePath:           tmpDir,
		MaxTotalSizeMB:     10,
		MaxHeapMB:          50,
		MemoryCheckSeconds: 60,
		EmergencyThinRatio: 0.5,
	}, nil)
	if err != nil {
		t.Fatalf("Failed to create queue manager: %v", err)
	}

	q, err := queueMgr.CreateQueue("test-cam", queue.DefaultQueueConfig())
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	config := CameraConfig{ID: "test-cam", RemotePath: "test/path", Enabled: true}
	worker.AddQueue("test-cam", q, config, uploader)

	// Verify queue was added
	if len(worker.queues) != 1 {
		t.Errorf("Expected 1 queue, got %d", len(worker.queues))
	}
	if worker.queues["test-cam"] != q {
		t.Error("Queue not stored correctly")
	}
	if worker.uploaders["test-cam"] != uploader {
		t.Error("Uploader not stored correctly")
	}
	if len(worker.queueOrder) != 1 || worker.queueOrder[0] != "test-cam" {
		t.Error("Queue order not set correctly")
	}
	if worker.cameraFailures["test-cam"] == nil {
		t.Error("Camera failure state not initialized")
	}
}

// TestUploadWorker_RemoveQueue tests removing a queue from the upload worker
func TestUploadWorker_RemoveQueue(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-remove-queue-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	worker := NewUploadWorker(UploadWorkerConfig{})
	uploader := &mockUploader{}

	queueMgr, err := queue.NewManager(queue.GlobalQueueConfig{
		BasePath:           tmpDir,
		MaxTotalSizeMB:     10,
		MaxHeapMB:          50,
		MemoryCheckSeconds: 60,
		EmergencyThinRatio: 0.5,
	}, nil)
	if err != nil {
		t.Fatalf("Failed to create queue manager: %v", err)
	}

	// Add two queues
	q1, _ := queueMgr.CreateQueue("cam1", queue.DefaultQueueConfig())
	q2, _ := queueMgr.CreateQueue("cam2", queue.DefaultQueueConfig())

	config1 := CameraConfig{ID: "cam1", RemotePath: "test/cam1", Enabled: true}
	config2 := CameraConfig{ID: "cam2", RemotePath: "test/cam2", Enabled: true}

	worker.AddQueue("cam1", q1, config1, uploader)
	worker.AddQueue("cam2", q2, config2, uploader)

	// Remove one queue
	worker.RemoveQueue("cam1")

	// Verify cam1 was removed but cam2 remains
	if len(worker.queues) != 1 {
		t.Errorf("Expected 1 queue after removal, got %d", len(worker.queues))
	}
	if _, exists := worker.queues["cam1"]; exists {
		t.Error("cam1 should have been removed from queues")
	}
	if _, exists := worker.uploaders["cam1"]; exists {
		t.Error("cam1 should have been removed from uploaders")
	}
	if _, exists := worker.cameraFailures["cam1"]; exists {
		t.Error("cam1 should have been removed from cameraFailures")
	}
	if len(worker.queueOrder) != 1 || worker.queueOrder[0] != "cam2" {
		t.Errorf("Queue order incorrect after removal: %v", worker.queueOrder)
	}
	if worker.queues["cam2"] != q2 {
		t.Error("cam2 should still exist")
	}
}

// TestUploadWorker_RemoveQueue_NonExistent tests removing a non-existent queue
func TestUploadWorker_RemoveQueue_NonExistent(t *testing.T) {
	worker := NewUploadWorker(UploadWorkerConfig{})

	// Should not panic when removing non-existent queue
	worker.RemoveQueue("non-existent")

	// Verify worker is still in valid state
	if worker.queues == nil {
		t.Error("queues map should not be nil")
	}
}

// TestUploadWorker_BuildRemotePath tests remote path building
func TestUploadWorker_BuildRemotePath(t *testing.T) {
	worker := NewUploadWorker(UploadWorkerConfig{})

	tests := []struct {
		name       string
		basePath   string
		cameraID   string
		timestamp  time.Time
		wantPrefix string
		wantSuffix string
	}{
		{
			name:       "with base path",
			basePath:   "uploads/camera1",
			cameraID:   "cam1",
			timestamp:  time.Unix(1234567890, 0),
			wantPrefix: "uploads/camera1/",
			wantSuffix: ".jpg",
		},
		{
			name:       "empty base path uses camera ID",
			basePath:   "",
			cameraID:   "cam1",
			timestamp:  time.Unix(1234567890, 0),
			wantPrefix: "cam1/",
			wantSuffix: ".jpg",
		},
		{
			name:       "base path with trailing slash",
			basePath:   "uploads/",
			cameraID:   "cam1",
			timestamp:  time.Unix(1234567890, 0),
			wantPrefix: "uploads/",
			wantSuffix: ".jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := worker.buildRemotePath(tt.basePath, tt.cameraID, tt.timestamp)

			if len(result) < len(tt.wantPrefix)+len(tt.wantSuffix) {
				t.Errorf("Result too short: %s", result)
			}

			if result[:len(tt.wantPrefix)] != tt.wantPrefix {
				t.Errorf("Expected prefix %s, got %s", tt.wantPrefix, result[:len(tt.wantPrefix)])
			}

			if result[len(result)-len(tt.wantSuffix):] != tt.wantSuffix {
				t.Errorf("Expected suffix %s, got %s", tt.wantSuffix, result[len(result)-len(tt.wantSuffix):])
			}
		})
	}
}

// TestUploadWorker_IsAuthError tests authentication error detection
func TestUploadWorker_IsAuthError(t *testing.T) {
	worker := NewUploadWorker(UploadWorkerConfig{})

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"auth error", fmt.Errorf("authentication failed"), true},
		{"401 error", fmt.Errorf("401 unauthorized"), true},
		{"403 error", fmt.Errorf("403 forbidden"), true},
		{"login error", fmt.Errorf("login required"), true},
		{"credential error", fmt.Errorf("invalid credentials"), true},
		{"permission error", fmt.Errorf("permission denied"), true},
		{"access denied", fmt.Errorf("access denied"), true},
		{"regular error", fmt.Errorf("network timeout"), false},
		{"connection error", fmt.Errorf("connection refused"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := worker.isAuthError(tt.err)
			if got != tt.want {
				t.Errorf("isAuthError() = %v, want %v for error: %v", got, tt.want, tt.err)
			}
		})
	}
}

// TestUploadWorker_GetStats tests statistics retrieval
func TestUploadWorker_GetStats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-stats-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	worker := NewUploadWorker(UploadWorkerConfig{})

	// Set some stats
	worker.uploadsTotal = 10
	worker.uploadsSuccess = 7
	worker.uploadsFailed = 3
	worker.uploadsRetried = 2
	worker.authFailures = 1

	stats := worker.GetStats()

	if stats.UploadsTotal != 10 {
		t.Errorf("UploadsTotal = %d, want 10", stats.UploadsTotal)
	}
	if stats.UploadsSuccess != 7 {
		t.Errorf("UploadsSuccess = %d, want 7", stats.UploadsSuccess)
	}
	if stats.UploadsFailed != 3 {
		t.Errorf("UploadsFailed = %d, want 3", stats.UploadsFailed)
	}
	if stats.UploadsRetried != 2 {
		t.Errorf("UploadsRetried = %d, want 2", stats.UploadsRetried)
	}
	if stats.AuthFailures != 1 {
		t.Errorf("AuthFailures = %d, want 1", stats.AuthFailures)
	}
}

// TestUploadWorker_ConfigDefaults tests configuration defaults
func TestUploadWorker_ConfigDefaults(t *testing.T) {
	worker := NewUploadWorker(UploadWorkerConfig{})

	if worker.minUploadInterval != time.Second {
		t.Errorf("Default minUploadInterval = %v, want %v", worker.minUploadInterval, time.Second)
	}
	if worker.authBackoff != 60*time.Second {
		t.Errorf("Default authBackoff = %v, want %v", worker.authBackoff, 60*time.Second)
	}
	if worker.retryDelay != 5*time.Second {
		t.Errorf("Default retryDelay = %v, want %v", worker.retryDelay, 5*time.Second)
	}
	if worker.logger == nil {
		t.Error("Default logger should not be nil")
	}
}

// TestReadImageFile tests the readImageFile helper function
func TestReadImageFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-read-image-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testData := []byte("test image data")
	testFile := filepath.Join(tmpDir, "test.jpg")

	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	data, err := readImageFile(testFile)
	if err != nil {
		t.Errorf("readImageFile() error = %v", err)
	}
	if string(data) != string(testData) {
		t.Errorf("readImageFile() data mismatch, got %q, want %q", string(data), string(testData))
	}

	// Test non-existent file
	_, err = readImageFile(filepath.Join(tmpDir, "nonexistent.jpg"))
	if err == nil {
		t.Error("readImageFile() should error on non-existent file")
	}
}

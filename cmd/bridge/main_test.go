package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexwitherspoon/aviationwx-bridge/internal/config"
	"github.com/alexwitherspoon/aviationwx-bridge/internal/logger"
)

func TestBridge_testCamera_Success(t *testing.T) {
	fakeJPEG := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(fakeJPEG)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	svc, err := config.NewService(tmpDir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	bridge := &Bridge{
		configService:      svc,
		log:                logger.Default(),
		lastCaptures:       make(map[string]*CachedImage),
		cameraWorkerStatus: make(map[string]*CameraWorkerStatus),
	}

	cam := config.Camera{
		ID:          "test-cam",
		Type:        "http",
		SnapshotURL: server.URL + "/snapshot.jpg",
		Auth:        nil,
		ONVIF:       nil,
		RTSP:        nil,
	}

	image, err := bridge.testCamera(cam)
	if err != nil {
		t.Fatalf("testCamera: %v", err)
	}
	if !bytes.Equal(image, fakeJPEG) {
		t.Errorf("got image %d bytes, want %d bytes; content mismatch", len(image), len(fakeJPEG))
	}
}

func TestBridge_testCamera_CaptureError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	svc, err := config.NewService(tmpDir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	bridge := &Bridge{
		configService:      svc,
		log:                logger.Default(),
		lastCaptures:       make(map[string]*CachedImage),
		cameraWorkerStatus: make(map[string]*CameraWorkerStatus),
	}

	cam := config.Camera{
		ID:          "test-cam",
		Type:        "http",
		SnapshotURL: server.URL + "/snapshot.jpg",
	}

	_, err = bridge.testCamera(cam)
	if err == nil {
		t.Fatal("testCamera expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "capture image") {
		t.Errorf("error should mention capture: %v", err)
	}
}

func TestBridge_testCamera_CreateError(t *testing.T) {
	tmpDir := t.TempDir()
	svc, err := config.NewService(tmpDir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	bridge := &Bridge{
		configService:      svc,
		log:                logger.Default(),
		lastCaptures:       make(map[string]*CachedImage),
		cameraWorkerStatus: make(map[string]*CameraWorkerStatus),
	}

	cam := config.Camera{
		ID:          "test-cam",
		Type:        "http",
		SnapshotURL: "", // Missing required field
	}

	_, err = bridge.testCamera(cam)
	if err == nil {
		t.Fatal("testCamera expected error for invalid config")
	}
	if !strings.Contains(err.Error(), "create camera") {
		t.Errorf("error should mention create camera: %v", err)
	}
}

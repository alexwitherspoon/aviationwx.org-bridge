package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/alexwitherspoon/aviationwx-bridge/internal/camera"
	"github.com/alexwitherspoon/aviationwx-bridge/internal/config"
	"github.com/alexwitherspoon/aviationwx-bridge/internal/image"
	"github.com/alexwitherspoon/aviationwx-bridge/internal/logger"
	"github.com/alexwitherspoon/aviationwx-bridge/internal/scheduler"
	"github.com/alexwitherspoon/aviationwx-bridge/internal/update"
	"github.com/alexwitherspoon/aviationwx-bridge/internal/upload"
	"github.com/alexwitherspoon/aviationwx-bridge/internal/web"
	"github.com/alexwitherspoon/aviationwx-bridge/pkg/health"
)

// Build info set at compile time via ldflags
var (
	Version   = "dev"
	GitCommit = "unknown"
)

// Bridge is the main application struct
type Bridge struct {
	config        *config.Config
	configPath    string
	orchestrator  *scheduler.Orchestrator
	webServer     *web.Server
	updateChecker *update.Checker
	systemMonitor *health.SystemMonitor
	log           *logger.Logger

	// Preview cache
	lastCaptures map[string]*CachedImage
	captureMu    sync.RWMutex

	// Worker status tracking
	cameraWorkerStatus map[string]*CameraWorkerStatus
	workerStatusMu     sync.RWMutex
}

// CameraWorkerStatus tracks the runtime status of a camera worker
type CameraWorkerStatus struct {
	CameraID     string
	Running      bool
	LastError    string
	LastAttempt  time.Time
	ErrorCount   int
}

// CachedImage holds a captured image with metadata
type CachedImage struct {
	Data       []byte
	CapturedAt time.Time
}

func main() {
	// Initialize logger from environment
	logger.Init()
	log := logger.Default()

	log.Info("AviationWX Bridge starting",
		"version", Version,
		"commit", GitCommit)

	// Load configuration
	configPath := os.Getenv("AVIATIONWX_CONFIG")
	if configPath == "" {
		configPath = "/data/config.json"
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Warn("Could not load config, using defaults",
			"path", configPath,
			"error", err)
		cfg = &config.Config{
			Version:  2,
			Timezone: "UTC",
			Cameras:  []config.Camera{},
			WebConsole: &config.WebConsole{
				Enabled:  true,
				Port:     1229,
				Password: "aviationwx",
			},
		}
		// Try to save default config for next time
		if err := saveConfig(configPath, cfg); err != nil {
			log.Warn("Could not save default config", "error", err)
		}
	}

	// Create update checker
	updateChecker := update.NewChecker(Version, GitCommit)
	updateChecker.Start()
	log.Info("Update checker started")

	// Get queue path for system monitor
	queuePath := os.Getenv("AVIATIONWX_QUEUE_PATH")
	if queuePath == "" {
		queuePath = "/dev/shm/aviationwx"
	}

	// Create bridge
	bridge := &Bridge{
		config:             cfg,
		configPath:         configPath,
		updateChecker:      updateChecker,
		systemMonitor:      health.NewSystemMonitor(queuePath),
		log:                log,
		lastCaptures:       make(map[string]*CachedImage),
		cameraWorkerStatus: make(map[string]*CameraWorkerStatus),
	}

	// Initialize orchestrator (non-fatal if it fails)
	if err := bridge.initOrchestrator(); err != nil {
		log.Warn("Could not initialize orchestrator - cameras disabled", "error", err)
		// Continue - web UI will still work for configuration
	}

	// Create web server with callbacks (ALWAYS succeeds - web UI must be accessible)
	bridge.webServer = web.NewServer(web.ServerConfig{
		Config:           cfg,
		ConfigPath:       configPath,
		OnConfigChange:   bridge.handleConfigChange,
		GetStatus:        bridge.getStatus,
		TestCamera:       bridge.testCamera,
		TestUpload:       bridge.testUpload,
		GetCameraImage:   bridge.getCameraImage,
		GetWorkerStatus:  bridge.getWorkerStatus,
	})

	// Start orchestrator if we have cameras (non-fatal)
	if bridge.orchestrator != nil && len(cfg.Cameras) > 0 {
		if err := bridge.orchestrator.Start(); err != nil {
			log.Warn("Failed to start orchestrator - cameras disabled", "error", err)
		} else {
			log.Info("Orchestrator started", "cameras", len(cfg.Cameras))
		}
	} else if bridge.orchestrator == nil {
		log.Warn("Orchestrator not initialized - cameras disabled, web UI accessible")
	}

	// Start web server in goroutine
	go func() {
		port := cfg.GetWebPort()
		log.Info("Web console available",
			"url", fmt.Sprintf("http://localhost:%d", port),
			"password", cfg.GetWebPassword())
		if err := bridge.webServer.Start(); err != nil {
			log.Error("Web server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info("Shutting down...")

	// Stop update checker
	if bridge.updateChecker != nil {
		bridge.updateChecker.Stop()
	}

	// Stop orchestrator
	if bridge.orchestrator != nil {
		bridge.orchestrator.Stop()
	}

	// Stop web server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := bridge.webServer.Stop(ctx); err != nil {
		log.Error("Error stopping server", "error", err)
	}

	log.Info("Goodbye!")
}

// initOrchestrator initializes the orchestrator and adds cameras
func (b *Bridge) initOrchestrator() error {
	// Get queue path from environment or use default
	queuePath := os.Getenv("AVIATIONWX_QUEUE_PATH")
	if queuePath == "" {
		queuePath = "/dev/shm/aviationwx"
	}

	// Create orchestrator
	orchConfig := scheduler.DefaultOrchestratorConfig()
	orchConfig.QueueBasePath = queuePath
	orchConfig.Timezone = b.config.Timezone
	orchConfig.Logger = newBridgeLogger(b.log)
	// Note: exiftool is used for EXIF operations (auto-detected from PATH)

	orch, err := scheduler.NewOrchestrator(orchConfig)
	if err != nil {
		return fmt.Errorf("create orchestrator: %w", err)
	}
	b.orchestrator = orch

	// Add cameras from config (skip invalid cameras, don't fail)
	successCount := 0
	for _, camConfig := range b.config.Cameras {
		if !camConfig.Enabled {
			b.log.Info("Camera disabled, skipping", "camera", camConfig.ID)
			continue
		}

		if err := b.addCamera(camConfig); err != nil {
			b.log.Error("Failed to add camera, skipping", 
				"camera", camConfig.ID, 
				"error", err,
				"help", "Check camera configuration and credentials")
			// Continue with other cameras - don't fail entire orchestrator
		} else {
			successCount++
		}
	}

	b.log.Info("Camera initialization complete", 
		"total", len(b.config.Cameras),
		"enabled", successCount)

	return nil
}

// addCamera creates and adds a camera to the orchestrator
func (b *Bridge) addCamera(camConfig config.Camera) error {
	// Track worker status
	b.workerStatusMu.Lock()
	status := &CameraWorkerStatus{
		CameraID:    camConfig.ID,
		Running:     false,
		LastAttempt: time.Now(),
	}
	b.cameraWorkerStatus[camConfig.ID] = status
	b.workerStatusMu.Unlock()

	// Create camera instance
	cam, err := b.createCamera(camConfig)
	if err != nil {
		status.LastError = fmt.Sprintf("Create camera failed: %v", err)
		status.ErrorCount++
		return fmt.Errorf("create camera: %w", err)
	}

	// Create image processor if configured
	var imgProcessor *image.Processor
	if camConfig.Image != nil && camConfig.Image.NeedsProcessing() {
		imgProcessor = image.NewProcessor(camConfig.Image)
	}

	// Create scheduler camera config
	schedConfig := scheduler.CameraConfig{
		RemotePath:     camConfig.ID, // Use camera ID as remote path
		ImageProcessor: imgProcessor,
	}

	// Get capture interval
	interval := camConfig.CaptureIntervalSeconds
	if interval == 0 {
		interval = 60 // Default 60 seconds
	}

	// Create uploader for this camera
	var uploader upload.Client
	if camConfig.Upload != nil {
		var err error
		uploader, err = b.createUploader(camConfig.Upload)
		if err != nil {
			status.LastError = fmt.Sprintf("Create uploader failed: %v", err)
			status.ErrorCount++
			return fmt.Errorf("create uploader: %w", err)
		}
	} else {
		status.LastError = "Upload configuration missing"
		status.ErrorCount++
		return fmt.Errorf("upload configuration required for camera %s", camConfig.ID)
	}

	// Add to orchestrator with camera-specific uploader
	if err := b.orchestrator.AddCamera(cam, schedConfig, interval, uploader, b.updatePreviewCache); err != nil {
		status.LastError = fmt.Sprintf("Add to orchestrator failed: %v", err)
		status.ErrorCount++
		return fmt.Errorf("add to orchestrator: %w", err)
	}

	// Success!
	status.Running = true
	status.LastError = ""
	b.log.Info("Camera worker started successfully",
		"camera", camConfig.ID,
		"type", camConfig.Type,
		"interval", interval)

	return nil
}

// getWorkerStatus returns the runtime status of a camera worker
func (b *Bridge) getWorkerStatus(cameraID string) map[string]interface{} {
	b.workerStatusMu.RLock()
	defer b.workerStatusMu.RUnlock()

	status, exists := b.cameraWorkerStatus[cameraID]
	if !exists {
		return map[string]interface{}{
			"worker_running": false,
			"worker_error":   "Not started",
		}
	}

	result := map[string]interface{}{
		"worker_running": status.Running,
	}

	if status.LastError != "" {
		result["worker_error"] = status.LastError
		result["worker_error_count"] = status.ErrorCount
	}

	if !status.LastAttempt.IsZero() {
		result["worker_last_attempt"] = status.LastAttempt.Format(time.RFC3339)
	}

	return result
}

// createCamera creates a camera instance from config
func (b *Bridge) createCamera(camConfig config.Camera) (camera.Camera, error) {
	cameraConf := camera.Config{
		ID:          camConfig.ID,
		Type:        camConfig.Type,
		SnapshotURL: camConfig.SnapshotURL,
	}

	if camConfig.Auth != nil {
		cameraConf.Auth = &camera.AuthConfig{
			Type:     camConfig.Auth.Type,
			Username: camConfig.Auth.Username,
			Password: camConfig.Auth.Password,
			Token:    camConfig.Auth.Token,
		}
	}

	if camConfig.RTSP != nil {
		cameraConf.RTSP = &camera.RTSPConfig{
			URL:       camConfig.RTSP.URL,
			Username:  camConfig.RTSP.Username,
			Password:  camConfig.RTSP.Password,
			Substream: camConfig.RTSP.Substream,
		}
	}

	return camera.NewCamera(cameraConf)
}

// createUploader creates an upload client from config
func (b *Bridge) createUploader(uploadConfig *config.Upload) (upload.Client, error) {
	return upload.NewFTPSClient(upload.Config{
		Host:                  uploadConfig.Host,
		Port:                  uploadConfig.Port,
		Username:              uploadConfig.Username,
		Password:              uploadConfig.Password,
		TLS:                   uploadConfig.TLS,
		TLSVerify:             true,
		TimeoutConnectSeconds: 10,
		TimeoutUploadSeconds:  30,
	})
}

// handleConfigChange is called when config is updated via web UI
func (b *Bridge) handleConfigChange(newCfg *config.Config) error {
	// Build maps of old and new cameras BEFORE updating b.config
	oldCameras := make(map[string]config.Camera)
	for _, cam := range b.config.Cameras {
		oldCameras[cam.ID] = cam
	}

	newCameras := make(map[string]config.Camera)
	for _, cam := range newCfg.Cameras {
		newCameras[cam.ID] = cam
	}

	// Save config to file (ALWAYS save - web UI shows all cameras)
	if err := saveConfig(b.configPath, newCfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	// Update internal config reference (ALWAYS update - web UI needs to show current state)
	b.config = newCfg

	// Update web server config (it needs the updated reference too)
	b.webServer.UpdateConfig(newCfg)

	// Find cameras to delete
	var camerasToDelete []string
	for id := range oldCameras {
		if _, exists := newCameras[id]; !exists {
			camerasToDelete = append(camerasToDelete, id)
		}
	}

	// Find cameras to add or update
	var camerasToAdd []config.Camera
	var camerasToUpdate []config.Camera

	for id, newCam := range newCameras {
		oldCam, existed := oldCameras[id]
		if !existed {
			// Brand new camera
			if newCam.Enabled {
				camerasToAdd = append(camerasToAdd, newCam)
			}
		} else {
			// Camera exists - check if it changed or was disabled/enabled
			if oldCam.Enabled && !newCam.Enabled {
				// Camera disabled - treat as deletion from orchestrator
				camerasToDelete = append(camerasToDelete, id)
			} else if !oldCam.Enabled && newCam.Enabled {
				// Camera was disabled, now enabled - treat as addition
				camerasToAdd = append(camerasToAdd, newCam)
			} else if newCam.Enabled && cameraConfigChanged(oldCam, newCam) {
				// Camera configuration changed - needs update
				camerasToUpdate = append(camerasToUpdate, newCam)
			}
		}
	}

	// Clean up preview cache for deleted cameras
	b.captureMu.Lock()
	for cameraID := range b.lastCaptures {
		if _, exists := newCameras[cameraID]; !exists {
			delete(b.lastCaptures, cameraID)
			b.log.Debug("Preview cache cleaned up", "camera", cameraID)
		}
	}
	b.captureMu.Unlock()

	// Skip orchestrator operations if orchestrator is not initialized
	if b.orchestrator == nil {
		b.log.Warn("Orchestrator not initialized - camera changes saved but not applied")
		return nil
	}

	// Delete cameras from orchestrator (worker level - non-fatal)
	for _, cameraID := range camerasToDelete {
		if err := b.orchestrator.RemoveCamera(cameraID); err != nil {
			b.log.Error("Failed to remove camera worker",
				"camera", cameraID, "error", err)
			// Continue - not fatal, camera is removed from config
		} else {
			b.log.Info("Camera worker removed", "camera", cameraID)
		}
	}

	// Add new cameras to orchestrator (worker level - non-fatal)
	successfulAdds := 0
	for _, camConfig := range camerasToAdd {
		if err := b.addCamera(camConfig); err != nil {
			b.log.Error("Failed to start camera worker",
				"camera", camConfig.ID,
				"error", err,
				"help", "Camera saved to config but worker not started - check configuration")
			// Continue - camera is in config, user can see and fix it
		} else {
			b.log.Info("Camera worker started", "camera", camConfig.ID)
			successfulAdds++
		}
	}

	// Start any newly added workers (non-fatal)
	if successfulAdds > 0 {
		if err := b.orchestrator.Start(); err != nil {
			b.log.Warn("Failed to start new workers", "error", err)
		}
	}

	// Update existing cameras (worker level - non-fatal)
	successfulUpdates := 0
	for _, camConfig := range camerasToUpdate {
		b.log.Info("Updating camera worker", "camera", camConfig.ID)

		// Remove old camera worker
		if err := b.orchestrator.RemoveCamera(camConfig.ID); err != nil {
			b.log.Error("Failed to remove camera worker for update",
				"camera", camConfig.ID, "error", err)
			continue
		}

		// Add camera with new configuration
		if err := b.addCamera(camConfig); err != nil {
			b.log.Error("Failed to restart camera worker",
				"camera", camConfig.ID,
				"error", err,
				"help", "Camera config updated but worker not started - check configuration")
			continue
		}

		b.log.Info("Camera worker updated", "camera", camConfig.ID)
		successfulUpdates++
	}

	// Start updated workers (non-fatal)
	if successfulUpdates > 0 {
		if err := b.orchestrator.Start(); err != nil {
			b.log.Warn("Failed to start updated workers", "error", err)
		}
	}

	b.log.Info("Config saved and applied",
		"cameras_added", len(camerasToAdd),
		"cameras_deleted", len(camerasToDelete),
		"cameras_updated", len(camerasToUpdate),
		"workers_started", successfulAdds+successfulUpdates,
		"workers_failed", len(camerasToAdd)+len(camerasToUpdate)-successfulAdds-successfulUpdates)

	return nil
}

// cameraConfigChanged checks if camera configuration changed in a way that requires restart
func cameraConfigChanged(old, new config.Camera) bool {
	// Check type change
	if old.Type != new.Type {
		return true
	}

	// Check auth changes
	if (old.Auth == nil) != (new.Auth == nil) {
		return true
	}
	if old.Auth != nil && new.Auth != nil {
		if old.Auth.Username != new.Auth.Username || old.Auth.Password != new.Auth.Password {
			return true
		}
	}

	// Check capture settings
	if old.SnapshotURL != new.SnapshotURL {
		return true
	}
	if old.CaptureIntervalSeconds != new.CaptureIntervalSeconds {
		return true
	}

	// Check RTSP settings
	if (old.RTSP == nil) != (new.RTSP == nil) {
		return true
	}
	if old.RTSP != nil && new.RTSP != nil {
		if old.RTSP.URL != new.RTSP.URL {
			return true
		}
	}

	// Check ONVIF settings
	if (old.ONVIF == nil) != (new.ONVIF == nil) {
		return true
	}
	if old.ONVIF != nil && new.ONVIF != nil {
		if old.ONVIF.Endpoint != new.ONVIF.Endpoint ||
			old.ONVIF.Username != new.ONVIF.Username ||
			old.ONVIF.Password != new.ONVIF.Password ||
			old.ONVIF.ProfileToken != new.ONVIF.ProfileToken {
			return true
		}
	}

	// Check upload settings
	if (old.Upload == nil) != (new.Upload == nil) {
		return true
	}
	if old.Upload != nil && new.Upload != nil {
		if old.Upload.Host != new.Upload.Host ||
			old.Upload.Port != new.Upload.Port ||
			old.Upload.Username != new.Upload.Username ||
			old.Upload.Password != new.Upload.Password ||
			old.Upload.TLS != new.Upload.TLS {
			return true
		}
	}

	// Check image processing settings
	if (old.Image == nil) != (new.Image == nil) {
		return true
	}
	if old.Image != nil && new.Image != nil {
		if old.Image.MaxWidth != new.Image.MaxWidth ||
			old.Image.MaxHeight != new.Image.MaxHeight ||
			old.Image.Quality != new.Image.Quality {
			return true
		}
	}

	// No significant changes detected
	return false
}

// getStatus returns the current system status
func (b *Bridge) getStatus() interface{} {
	result := map[string]interface{}{
		"version":    Version,
		"git_commit": GitCommit,
	}

	// Add update status
	if b.updateChecker != nil {
		updateStatus := b.updateChecker.Status()
		result["update"] = map[string]interface{}{
			"available":      updateStatus.UpdateAvailable,
			"latest_version": updateStatus.LatestVersion,
			"latest_url":     updateStatus.LatestURL,
		}
	}

	if b.orchestrator != nil {
		status := b.orchestrator.GetStatus()
		result["running"] = status.Running
		result["uptime_seconds"] = int(status.Uptime.Seconds())
		result["uploads_today"] = status.UploadStats.UploadsSuccess
		result["queue_size"] = status.UploadStats.QueuedImages
		result["ntp_healthy"] = status.TimeInfo.TimeHealthy

		// Camera stats
		cameras := make([]map[string]interface{}, len(status.CameraStats))
		for i, cam := range status.CameraStats {
			cameras[i] = map[string]interface{}{
				"id":              cam.CameraID,
				"captures_total":  cam.CaptureStats.CapturesTotal,
				"captures_failed": cam.CaptureStats.CapturesFailed,
				"queue_count":     cam.QueueStats.ImageCount,
				"queue_size_mb":   cam.QueueStats.TotalSizeMB,
				"is_backing_off":  cam.IsBackingOff,
			}
			if !cam.LastSuccess.IsZero() {
				cameras[i]["last_success"] = cam.LastSuccess.Format(time.RFC3339)
			}
			if cam.LastError != nil {
				cameras[i]["last_error"] = cam.LastError.Error()
			}
		}
		result["cameras"] = cameras

		// Upload stats
		result["upload_stats"] = map[string]interface{}{
			"total":         status.UploadStats.UploadsTotal,
			"success":       status.UploadStats.UploadsSuccess,
			"failed":        status.UploadStats.UploadsFailed,
			"auth_failures": status.UploadStats.AuthFailures,
		}

		// Queue storage stats (for UI visibility)
		result["queue_storage"] = map[string]interface{}{
			"total_images":       status.GlobalQueueStats.TotalImages,
			"total_size_mb":      status.GlobalQueueStats.TotalSizeMB,
			"memory_usage_mb":    status.GlobalQueueStats.MemoryUsageMB,
			"memory_limit_mb":    status.GlobalQueueStats.MemoryLimitMB,
			"capacity_percent":   calculateCapacityPercent(status.GlobalQueueStats.TotalSizeMB, status.GlobalQueueStats.MemoryLimitMB),
			"filesystem_free_mb": status.GlobalQueueStats.FilesystemFreeMB,
			"filesystem_used_mb": status.GlobalQueueStats.FilesystemUsedMB,
		}
	} else {
		result["running"] = false
		result["uploads_today"] = 0
		result["queue_size"] = 0
		result["ntp_healthy"] = true
		result["queue_storage"] = map[string]interface{}{
			"total_images":       0,
			"total_size_mb":      0.0,
			"memory_usage_mb":    0.0,
			"memory_limit_mb":    100,
			"capacity_percent":   0.0,
			"filesystem_free_mb": 0.0,
			"filesystem_used_mb": 0.0,
		}
	}

	// Add system resource stats
	if b.systemMonitor != nil {
		sysStats := b.systemMonitor.GetStats()
		result["system"] = map[string]interface{}{
			"cpu_percent":    sysStats.CPUPercent,
			"cpu_level":      sysStats.CPULevel,
			"num_goroutines": sysStats.NumGoroutines,
			"num_cpu":        sysStats.NumCPU,
			"mem_used_mb":    sysStats.MemUsedMB,
			"mem_total_mb":   sysStats.MemTotalMB,
			"mem_percent":    sysStats.MemPercent,
			"mem_level":      sysStats.MemLevel,
			"heap_alloc_mb":  sysStats.HeapAllocMB,
			"disk_used_mb":   sysStats.DiskUsedMB,
			"disk_free_mb":   sysStats.DiskFreeMB,
			"disk_total_mb":  sysStats.DiskTotalMB,
			"disk_percent":   sysStats.DiskPercent,
			"disk_level":     sysStats.DiskLevel,
			"overall_level":  sysStats.OverallLevel,
			"uptime":         sysStats.Uptime,
		}
	}

	return result
}

// testCamera tests camera capture
func (b *Bridge) testCamera(camConfig config.Camera) ([]byte, error) {
	cam, err := b.createCamera(camConfig)
	if err != nil {
		return nil, fmt.Errorf("create camera: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	imageData, err := cam.Capture(ctx)
	if err != nil {
		return nil, fmt.Errorf("capture: %w", err)
	}

	// Apply image processing if configured
	if camConfig.Image != nil && camConfig.Image.NeedsProcessing() {
		processor := image.NewProcessor(camConfig.Image)
		imageData, err = processor.Process(imageData)
		if err != nil {
			return nil, fmt.Errorf("process image: %w", err)
		}
	}

	return imageData, nil
}

// testUpload tests upload connection
func (b *Bridge) testUpload(uploadConfig config.Upload) error {
	uploader, err := b.createUploader(&uploadConfig)
	if err != nil {
		return fmt.Errorf("create uploader: %w", err)
	}

	return uploader.TestConnection()
}

// updatePreviewCache updates the cached preview image for a camera
func (b *Bridge) updatePreviewCache(cameraID string, imageData []byte, captureTime time.Time) {
	b.captureMu.Lock()
	defer b.captureMu.Unlock()

	b.lastCaptures[cameraID] = &CachedImage{
		Data:       imageData,
		CapturedAt: captureTime,
	}

	b.log.Debug("Preview cache updated", "camera", cameraID, "size", len(imageData))
}

// getCameraImage returns the latest captured image for a camera
func (b *Bridge) getCameraImage(cameraID string) ([]byte, error) {
	// Check cache first
	b.captureMu.RLock()
	cached, ok := b.lastCaptures[cameraID]
	b.captureMu.RUnlock()

	if ok && cached != nil {
		// Return cached image
		b.log.Debug("Returning cached preview", "camera", cameraID, "age", time.Since(cached.CapturedAt))
		return cached.Data, nil
	}

	// No cache available - return empty instead of capturing
	// Fresh captures will populate cache within 60 seconds
	return nil, fmt.Errorf("no preview available yet")
}

// bridgeLogger wraps logger.Logger to implement scheduler.Logger
type bridgeLogger struct {
	log *logger.Logger
}

func newBridgeLogger(log *logger.Logger) *bridgeLogger {
	return &bridgeLogger{log: log}
}

func (l *bridgeLogger) Debug(msg string, keysAndValues ...interface{}) {
	l.log.Debug(msg, keysAndValues...)
}

func (l *bridgeLogger) Info(msg string, keysAndValues ...interface{}) {
	l.log.Info(msg, keysAndValues...)
}

func (l *bridgeLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.log.Warn(msg, keysAndValues...)
}

func (l *bridgeLogger) Error(msg string, keysAndValues ...interface{}) {
	l.log.Error(msg, keysAndValues...)
}

func loadConfig(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func saveConfig(path string, cfg *config.Config) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	// Create backup of existing config before overwriting
	if _, err := os.Stat(path); err == nil {
		backupPath := path + ".backup"
		if err := os.Rename(path, backupPath); err != nil {
			// Non-fatal - log but continue
			fmt.Printf("Warning: could not create config backup: %v\n", err)
		}
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first, then atomic rename
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename temp config: %w", err)
	}

	return nil
}

// calculateCapacityPercent calculates percentage of queue capacity used
func calculateCapacityPercent(usedMB float64, limitMB int) float64 {
	if limitMB <= 0 {
		return 0
	}
	percent := (usedMB / float64(limitMB)) * 100
	if percent > 100 {
		percent = 100
	}
	return percent
}

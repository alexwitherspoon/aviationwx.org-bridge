package camera

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// RTSPCamera implements Camera interface for RTSP stream cameras.
// Uses ffmpeg to capture a single frame from the RTSP stream.
type RTSPCamera struct {
	config Config
}

// NewRTSPCamera creates a new RTSP camera instance.
// Returns an error if the RTSP URL is missing.
func NewRTSPCamera(config Config) (*RTSPCamera, error) {
	if config.RTSP == nil {
		return nil, fmt.Errorf("rtsp config is required")
	}
	if config.RTSP.URL == "" {
		return nil, fmt.Errorf("rtsp.url is required")
	}

	return &RTSPCamera{
		config: config,
	}, nil
}

// Capture fetches a fresh snapshot from the RTSP stream using ffmpeg.
// Captures a single frame and exits immediately (no long-running decoder state).
// Always returns fresh data - never cached or stale images.
func (c *RTSPCamera) Capture(ctx context.Context) ([]byte, error) {
	timeout := time.Duration(c.config.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 20 * time.Second // Default RTSP timeout
	}

	// Create context with timeout
	captureCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build final RTSP URL with all modifications
	rtspURL := c.config.RTSP.URL

	// If substream is preferred and URL supports it, modify URL
	// (This is camera-specific - some cameras have substream URLs)
	if c.config.RTSP.Substream {
		// Try common substream patterns (camera-specific)
		// Most cameras use stream1 for main, stream2 for substream
		// This is a best-effort attempt - actual implementation may vary
		rtspURL = c.modifyURLForSubstream(rtspURL)
	}

	// Add authentication if provided and not already in URL
	if c.config.RTSP.Username != "" && c.config.RTSP.Password != "" {
		if !containsCredentials(rtspURL) {
			rtspURL = fmt.Sprintf("rtsp://%s:%s@%s",
				c.config.RTSP.Username,
				c.config.RTSP.Password,
				extractHostPath(rtspURL))
		}
	}

	// Build ffmpeg command
	// -rtsp_transport tcp: Use TCP for more reliable connection
	// -i: Input RTSP URL
	// -vframes 1: Capture only one frame
	// -f image2: Output format as image
	// -vcodec mjpeg: Use MJPEG codec for JPEG output
	// -: Output to stdout
	args := []string{
		"-rtsp_transport", "tcp",
		"-i", rtspURL,
		"-vframes", "1",
		"-f", "image2",
		"-vcodec", "mjpeg",
		"-",
	}

	// Create ffmpeg command
	cmd := exec.CommandContext(captureCtx, "ffmpeg", args...)

	// Capture stderr separately for error messages
	// ffmpeg writes image data to stdout and errors/warnings to stderr
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	// Capture stdout (image data)
	output, err := cmd.Output()
	if err != nil {
		// Check if error is due to timeout
		if captureCtx.Err() == context.DeadlineExceeded {
			return nil, &TimeoutError{
				CameraID: c.config.ID,
				Timeout:  timeout,
			}
		}

		// Include stderr in error message for better debugging
		stderrMsg := stderrBuf.String()
		errMsg := "ffmpeg capture failed"
		if stderrMsg != "" {
			errMsg += ": " + stderrMsg
		}

		// Check for authentication errors
		if isAuthError(err) || strings.Contains(stderrMsg, "401") {
			return nil, &AuthError{
				CameraID: c.config.ID,
				Message:  "RTSP authentication failed",
			}
		}

		return nil, &CaptureError{
			CameraID: c.config.ID,
			Message:  errMsg,
			Err:      err,
		}
	}

	if len(output) == 0 {
		return nil, &CaptureError{
			CameraID: c.config.ID,
			Message:  "ffmpeg returned empty output",
		}
	}

	return output, nil
}

// ID returns the camera identifier
func (c *RTSPCamera) ID() string {
	return c.config.ID
}

// Type returns the camera type
func (c *RTSPCamera) Type() string {
	return "rtsp"
}

// Helper functions

// modifyURLForSubstream attempts to modify RTSP URL for substream
// This is camera-specific and may need adjustment per camera model
func (c *RTSPCamera) modifyURLForSubstream(url string) string {
	// Common patterns:
	// stream1 -> stream2
	// main -> sub
	// 0 -> 1
	// This is a best-effort - actual implementation varies by camera
	if strings.Contains(url, "/stream1") {
		return strings.Replace(url, "/stream1", "/stream2", 1)
	}
	if strings.Contains(url, "/main") {
		return strings.Replace(url, "/main", "/sub", 1)
	}
	if strings.Contains(url, "/0") && !strings.Contains(url, "/10") {
		return strings.Replace(url, "/0", "/1", 1)
	}
	// If no pattern matches, return original URL
	return url
}

func containsCredentials(url string) bool {
	// Check if URL contains @ symbol (indicates credentials)
	return strings.Contains(url, "@")
}

func extractHostPath(rtspURL string) string {
	// Parse URL properly to handle ports, query params, etc.
	u, err := url.Parse(rtspURL)
	if err != nil {
		// Fallback to string manipulation if parsing fails
		if idx := strings.Index(rtspURL, "://"); idx >= 0 {
			rtspURL = rtspURL[idx+3:]
		}
		if idx := strings.Index(rtspURL, "@"); idx >= 0 {
			rtspURL = rtspURL[idx+1:]
		}
		return rtspURL
	}

	// Reconstruct URL without scheme and credentials
	hostPath := u.Host
	if u.Path != "" {
		hostPath += u.Path
	}
	if u.RawQuery != "" {
		hostPath += "?" + u.RawQuery
	}
	return hostPath
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "authentication") ||
		strings.Contains(errStr, "access denied")
}

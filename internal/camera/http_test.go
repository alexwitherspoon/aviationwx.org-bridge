package camera

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPCamera(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				ID:          "test-camera",
				SnapshotURL: "http://example.com/snapshot.jpg",
			},
			wantErr: false,
		},
		{
			name: "missing snapshot URL",
			config: Config{
				ID: "test-camera",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cam, err := NewHTTPCamera(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHTTPCamera() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && cam == nil {
				t.Error("NewHTTPCamera() returned nil camera")
			}
		})
	}
}

func TestHTTPCamera_Capture_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify cache-busting headers
		if r.Header.Get("Cache-Control") != "no-cache, no-store, must-revalidate" {
			t.Error("missing Cache-Control header")
		}
		if r.Header.Get("Pragma") != "no-cache" {
			t.Error("missing Pragma header")
		}

		// Verify cache-busting query parameter
		if !strings.Contains(r.URL.RawQuery, "t=") {
			t.Error("missing timestamp query parameter")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake-image-data"))
	}))
	defer server.Close()

	config := Config{
		ID:             "test-camera",
		SnapshotURL:    server.URL + "/snapshot.jpg",
		TimeoutSeconds: 5,
	}

	cam, err := NewHTTPCamera(config)
	if err != nil {
		t.Fatalf("NewHTTPCamera() error = %v", err)
	}

	ctx := context.Background()
	data, err := cam.Capture(ctx)

	if err != nil {
		t.Errorf("Capture() error = %v", err)
	}
	if len(data) == 0 {
		t.Error("Capture() returned empty data")
	}
	if string(data) != "fake-image-data" {
		t.Errorf("Capture() = %s, want 'fake-image-data'", string(data))
	}
}

func TestHTTPCamera_Capture_Timeout(t *testing.T) {
	// Create slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Longer than timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		ID:             "test-camera",
		SnapshotURL:    server.URL + "/snapshot.jpg",
		TimeoutSeconds: 1, // 1 second timeout
	}

	cam, err := NewHTTPCamera(config)
	if err != nil {
		t.Fatalf("NewHTTPCamera() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err = cam.Capture(ctx)

	if err == nil {
		t.Error("Capture() expected timeout error")
	}

	var timeoutErr *TimeoutError
	if !isTimeoutErrorType(err, timeoutErr) {
		t.Errorf("Capture() error = %v, want TimeoutError", err)
	}
}

func TestHTTPCamera_Capture_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	config := Config{
		ID:             "test-camera",
		SnapshotURL:    server.URL + "/snapshot.jpg",
		TimeoutSeconds: 5,
	}

	cam, err := NewHTTPCamera(config)
	if err != nil {
		t.Fatalf("NewHTTPCamera() error = %v", err)
	}

	ctx := context.Background()
	_, err = cam.Capture(ctx)

	if err == nil {
		t.Error("Capture() expected error for non-200 status")
	}

	var captureErr *CaptureError
	if !isCaptureErrorType(err, captureErr) {
		t.Errorf("Capture() error = %v, want CaptureError", err)
	}
}

func TestHTTPCamera_Capture_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// No body written
	}))
	defer server.Close()

	config := Config{
		ID:             "test-camera",
		SnapshotURL:    server.URL + "/snapshot.jpg",
		TimeoutSeconds: 5,
	}

	cam, err := NewHTTPCamera(config)
	if err != nil {
		t.Fatalf("NewHTTPCamera() error = %v", err)
	}

	ctx := context.Background()
	_, err = cam.Capture(ctx)

	if err == nil {
		t.Error("Capture() expected error for empty body")
	}

	var captureErr *CaptureError
	if !isCaptureErrorType(err, captureErr) {
		t.Errorf("Capture() error = %v, want CaptureError", err)
	}
}

func TestHTTPCamera_Capture_BasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if username != "admin" || password != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("authenticated-image"))
	}))
	defer server.Close()

	config := Config{
		ID:             "test-camera",
		SnapshotURL:    server.URL + "/snapshot.jpg",
		TimeoutSeconds: 5,
		Auth: &AuthConfig{
			Type:     "basic",
			Username: "admin",
			Password: "secret",
		},
	}

	cam, err := NewHTTPCamera(config)
	if err != nil {
		t.Fatalf("NewHTTPCamera() error = %v", err)
	}

	ctx := context.Background()
	data, err := cam.Capture(ctx)

	if err != nil {
		t.Errorf("Capture() error = %v", err)
	}
	if string(data) != "authenticated-image" {
		t.Errorf("Capture() = %s, want 'authenticated-image'", string(data))
	}
}

func TestHTTPCamera_Capture_BearerAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != "test-token-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("bearer-authenticated-image"))
	}))
	defer server.Close()

	config := Config{
		ID:             "test-camera",
		SnapshotURL:    server.URL + "/snapshot.jpg",
		TimeoutSeconds: 5,
		Auth: &AuthConfig{
			Type:  "bearer",
			Token: "test-token-123",
		},
	}

	cam, err := NewHTTPCamera(config)
	if err != nil {
		t.Fatalf("NewHTTPCamera() error = %v", err)
	}

	ctx := context.Background()
	data, err := cam.Capture(ctx)

	if err != nil {
		t.Errorf("Capture() error = %v", err)
	}
	if string(data) != "bearer-authenticated-image" {
		t.Errorf("Capture() = %s, want 'bearer-authenticated-image'", string(data))
	}
}

func TestHTTPCamera_Capture_AuthError(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "basic auth missing username",
			config: Config{
				ID:          "test-camera",
				SnapshotURL: "http://example.com/snapshot.jpg",
				Auth: &AuthConfig{
					Type:     "basic",
					Password: "secret",
				},
			},
		},
		{
			name: "basic auth missing password",
			config: Config{
				ID:          "test-camera",
				SnapshotURL: "http://example.com/snapshot.jpg",
				Auth: &AuthConfig{
					Type:     "basic",
					Username: "admin",
				},
			},
		},
		{
			name: "bearer auth missing token",
			config: Config{
				ID:          "test-camera",
				SnapshotURL: "http://example.com/snapshot.jpg",
				Auth: &AuthConfig{
					Type: "bearer",
				},
			},
		},
		{
			name: "unsupported auth type",
			config: Config{
				ID:          "test-camera",
				SnapshotURL: "http://example.com/snapshot.jpg",
				Auth: &AuthConfig{
					Type: "unsupported",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cam, err := NewHTTPCamera(tt.config)
			if err != nil {
				t.Fatalf("NewHTTPCamera() error = %v", err)
			}

			ctx := context.Background()
			_, err = cam.Capture(ctx)

			if err == nil {
				t.Error("Capture() expected auth error")
			}

			var authErr *AuthError
			if !isAuthErrorType(err, authErr) {
				t.Errorf("Capture() error = %v, want AuthError", err)
			}
		})
	}
}

func TestHTTPCamera_ID(t *testing.T) {
	config := Config{
		ID:          "test-camera-123",
		SnapshotURL: "http://example.com/snapshot.jpg",
	}

	cam, err := NewHTTPCamera(config)
	if err != nil {
		t.Fatalf("NewHTTPCamera() error = %v", err)
	}

	if cam.ID() != "test-camera-123" {
		t.Errorf("ID() = %s, want 'test-camera-123'", cam.ID())
	}
}

func TestHTTPCamera_Type(t *testing.T) {
	config := Config{
		ID:          "test-camera",
		SnapshotURL: "http://example.com/snapshot.jpg",
	}

	cam, err := NewHTTPCamera(config)
	if err != nil {
		t.Fatalf("NewHTTPCamera() error = %v", err)
	}

	if cam.Type() != "http" {
		t.Errorf("Type() = %s, want 'http'", cam.Type())
	}
}

func TestHTTPCamera_Capture_CacheBusting(t *testing.T) {
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("image"))
	}))
	defer server.Close()

	config := Config{
		ID:             "test-camera",
		SnapshotURL:    server.URL + "/snapshot.jpg",
		TimeoutSeconds: 5,
	}

	cam, err := NewHTTPCamera(config)
	if err != nil {
		t.Fatalf("NewHTTPCamera() error = %v", err)
	}

	ctx := context.Background()
	_, err = cam.Capture(ctx)
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	// Verify timestamp query parameter was added
	if !strings.Contains(capturedURL, "t=") {
		t.Errorf("URL missing timestamp parameter: %s", capturedURL)
	}

	// Test with existing query parameters
	config.SnapshotURL = server.URL + "/snapshot.jpg?existing=param"
	cam, err = NewHTTPCamera(config)
	if err != nil {
		t.Fatalf("NewHTTPCamera() error = %v", err)
	}

	_, err = cam.Capture(ctx)
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	// Verify timestamp was added with & separator
	if !strings.Contains(capturedURL, "&t=") && !strings.Contains(capturedURL, "?t=") {
		t.Errorf("URL missing timestamp parameter: %s", capturedURL)
	}
}

// Helper functions for type assertions

func isTimeoutErrorType(err error, target *TimeoutError) bool {
	_, ok := err.(*TimeoutError)
	return ok
}

func isCaptureErrorType(err error, target *CaptureError) bool {
	_, ok := err.(*CaptureError)
	return ok
}

func isAuthErrorType(err error, target *AuthError) bool {
	_, ok := err.(*AuthError)
	return ok
}

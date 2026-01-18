package time

import (
	"strings"
	"testing"
	"time"
)

// TestEXIF_UTCEnforcement is a SAFETY-CRITICAL test
// Ensures ALL timestamps are written as UTC with proper markers
func TestEXIF_UTCEnforcement(t *testing.T) {
	testCases := []struct {
		name     string
		timezone string
	}{
		{"UTC", "UTC"},
		{"US_Pacific", "America/Los_Angeles"},
		{"US_Eastern", "America/New_York"},
		{"Europe_London", "Europe/London"},
		{"Asia_Tokyo", "Asia/Tokyo"},
		{"Australia_Sydney", "Australia/Sydney"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Load timezone
			loc, err := time.LoadLocation(tc.timezone)
			if err != nil {
				t.Fatal(err)
			}

			// Create mock time health (healthy)
			timeHealth := &TimeHealth{
				healthy: true,
			}

			// Create authority with specific timezone
			authority, err := NewAuthority(timeHealth, AuthorityConfig{
				Timezone:                 tc.timezone,
				CameraToleranceSeconds:   5,
				CameraWarnDriftSeconds:   30,
				CameraRejectDriftSeconds: 300,
			})
			if err != nil {
				t.Fatal(err)
			}

			// Determine observation time
			captureTime := time.Now().In(loc) // Capture in local time
			result := authority.DetermineObservationTime(captureTime.UTC(), nil)

			// CRITICAL: Verify result time is in UTC
			if result.Time.Location() != time.UTC {
				t.Errorf("CRITICAL SAFETY FAILURE: Observation time is not UTC: %v (timezone: %v)",
					result.Time, result.Time.Location())
			}

			// CRITICAL: Verify observation result has correct format
			stampResult := StampBridgeEXIFWithTool(generateTestJPEG(), result)

			// Verify marker format (even if stamping failed)
			expectedPrefix := "AviationWX-Bridge:UTC:v1:"
			if !strings.HasPrefix(stampResult.Marker, expectedPrefix) {
				t.Errorf("CRITICAL: Invalid marker format. Expected prefix %q, got %q",
					expectedPrefix, stampResult.Marker)
			}

			// Verify marker contains UTC
			if !strings.Contains(stampResult.Marker, "UTC") {
				t.Errorf("CRITICAL: Marker missing UTC indicator: %s", stampResult.Marker)
			}

			// If stamping succeeded, verify the actual EXIF
			if stampResult.Stamped {
				// Verify time is recent (within 5 seconds)
				age := time.Since(stampResult.ObservationUTC)
				if age < -1*time.Second || age > 5*time.Second {
					t.Errorf("CRITICAL: Stamped time is not recent: %v (age: %v)",
						stampResult.ObservationUTC, age)
				}

				t.Logf("✓ Timezone %s: EXIF stamped as UTC (marker: %s)", tc.timezone, stampResult.Marker)
			} else {
				t.Logf("⚠ Timezone %s: EXIF stamping skipped (exiftool not available)", tc.timezone)
			}
		})
	}
}

// TestEXIF_MarkerValidation tests the bridge marker format
// This is SAFETY-CRITICAL for server-side timestamp parsing
func TestEXIF_MarkerValidation(t *testing.T) {
	testCases := []struct {
		name   string
		result ObservationResult
		expect string
	}{
		{
			name: "HighConfidence_NTP",
			result: ObservationResult{
				Time:       time.Now().UTC(),
				Source:     SourceBridgeClock,
				Confidence: ConfidenceHigh,
			},
			expect: "AviationWX-Bridge:UTC:v1:bridge_clock:high",
		},
		{
			name: "LowConfidence_NTPUnhealthy",
			result: ObservationResult{
				Time:       time.Now().UTC(),
				Source:     SourceBridgeClock,
				Confidence: ConfidenceLow,
				Warning: &TimeWarning{
					Code:    "ntp_unhealthy",
					Message: "NTP not synchronized",
				},
			},
			expect: "AviationWX-Bridge:UTC:v1:bridge_clock:low:warn:ntp_unhealthy",
		},
		{
			name: "CameraTime_HighConfidence",
			result: ObservationResult{
				Time:       time.Now().UTC(),
				Source:     SourceCameraEXIF,
				Confidence: ConfidenceHigh,
			},
			expect: "AviationWX-Bridge:UTC:v1:camera_exif:high",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stampResult := StampBridgeEXIFWithTool(generateTestJPEG(), tc.result)

			// CRITICAL: Verify marker format
			if stampResult.Marker != tc.expect {
				t.Errorf("CRITICAL: Marker mismatch.\nExpected: %s\nGot:      %s",
					tc.expect, stampResult.Marker)
			}

			// CRITICAL: Verify marker always contains UTC
			if !strings.Contains(stampResult.Marker, "UTC") {
				t.Errorf("CRITICAL: Marker missing UTC: %s", stampResult.Marker)
			}

			// CRITICAL: Verify marker always contains version
			if !strings.Contains(stampResult.Marker, "v1") {
				t.Errorf("CRITICAL: Marker missing version: %s", stampResult.Marker)
			}

			t.Logf("✓ Marker validated: %s", stampResult.Marker)
		})
	}
}

// TestEXIF_NTPFailureHandling tests behavior when NTP is unhealthy
// This is SAFETY-CRITICAL to warn about uncertain timestamps
func TestEXIF_NTPFailureHandling(t *testing.T) {
	// Create unhealthy time health
	timeHealth := &TimeHealth{
		healthy: false,
		offset:  30 * time.Second, // Significant drift
	}

	authority, err := NewAuthority(timeHealth, DefaultAuthorityConfig())
	if err != nil {
		t.Fatal(err)
	}

	// Determine observation time with unhealthy NTP
	captureTime := time.Now().UTC()
	result := authority.DetermineObservationTime(captureTime, nil)

	// CRITICAL: Should have low confidence
	if result.Confidence != ConfidenceLow {
		t.Errorf("CRITICAL: NTP unhealthy but confidence is %s (expected low)",
			result.Confidence)
	}

	// CRITICAL: Should have warning
	if result.Warning == nil {
		t.Error("CRITICAL: NTP unhealthy but no warning present")
	} else {
		if result.Warning.Code != "ntp_unhealthy" {
			t.Errorf("Expected warning code 'ntp_unhealthy', got %q",
				result.Warning.Code)
		}
		t.Logf("✓ Warning present: %s", result.Warning.Message)
	}

	// Stamp EXIF and verify warning is included
	stampResult := StampBridgeEXIFWithTool(generateTestJPEG(), result)

	if !strings.Contains(stampResult.Marker, "warn:ntp_unhealthy") {
		t.Errorf("CRITICAL: Marker missing NTP warning: %s", stampResult.Marker)
	}

	t.Logf("✓ NTP failure properly marked: %s", stampResult.Marker)
}

// TestEXIF_TimeInFuture tests rejection of future timestamps
// This is SAFETY-CRITICAL to detect clock errors
func TestEXIF_TimeInFuture(t *testing.T) {
	timeHealth := &TimeHealth{
		healthy: true,
	}

	authority, err := NewAuthority(timeHealth, DefaultAuthorityConfig())
	if err != nil {
		t.Fatal(err)
	}

	// Create camera time that's 10 minutes in the future
	futureTime := time.Now().UTC().Add(10 * time.Minute)
	cameraTime := futureTime

	// Bridge captured "now"
	bridgeTime := time.Now().UTC()

	result := authority.DetermineObservationTime(bridgeTime, &cameraTime)

	// CRITICAL: Should reject future camera time
	if result.Source == SourceCameraEXIF {
		t.Error("CRITICAL: Accepted camera time from the future")
	}

	// CRITICAL: Should use bridge time instead
	if result.Source != SourceBridgeClock {
		t.Errorf("Expected SourceBridgeClock, got %s", result.Source)
	}

	// CRITICAL: Should have warning about clock drift
	if result.Warning == nil {
		t.Error("CRITICAL: Camera clock error but no warning")
	}

	t.Logf("✓ Future time rejected: %s", result.Warning.Message)
}

// generateTestJPEG creates a minimal valid JPEG for testing
func generateTestJPEG() []byte {
	return []byte{
		0xFF, 0xD8, // SOI
		0xFF, 0xE0, // APP0
		0x00, 0x10, // Length
		0x4A, 0x46, 0x49, 0x46, 0x00, // "JFIF\0"
		0x01, 0x01, // Version
		0x00,                   // No units
		0x00, 0x01, 0x00, 0x01, // Density
		0x00, 0x00, // No thumbnail
		0xFF, 0xD9, // EOI
	}
}

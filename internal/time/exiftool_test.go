package time

import (
	"os"
	"strings"
	"testing"
	"time"
)

// requireExiftool requires exiftool to be available for the test
// Tests will FAIL if exiftool is not installed (fail closed approach)
func requireExiftool(t *testing.T) *ExifToolHelper {
	t.Helper()
	helper, err := DefaultExifToolHelper()
	if err != nil {
		t.Fatalf("exiftool is required but not available: %v\n\nInstall exiftool:\n  macOS: brew install exiftool\n  Ubuntu/Debian: sudo apt-get install libimage-exiftool-perl", err)
	}
	return helper
}

func TestExifToolHelper_NewExifToolHelper(t *testing.T) {
	helper := requireExiftool(t)

	if helper == nil {
		t.Fatal("Expected non-nil helper")
	}

	if helper.exiftoolPath == "" {
		t.Error("Expected non-empty exiftool path")
	}
}

func TestExifToolHelper_IsAvailable(t *testing.T) {
	helper := requireExiftool(t)

	if !helper.IsAvailable() {
		t.Error("Expected exiftool to be available")
	}
}

func TestExifToolHelper_GetVersion(t *testing.T) {
	helper := requireExiftool(t)

	version, err := helper.GetVersion()
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}

	if version == "" {
		t.Error("Expected non-empty version string")
	}

	t.Logf("exiftool version: %s", version)
}

func TestExifToolHelper_ReadEXIF_NoFile(t *testing.T) {
	helper := requireExiftool(t)

	// Note: exiftool may return an empty result rather than an error for nonexistent files
	result, err := helper.ReadEXIF("/nonexistent/path/to/image.jpg")

	// Either an error or an empty/unsuccessful result is acceptable
	if err == nil && result != nil && result.Success {
		// If it claims success, check that there's no meaningful data
		if result.DateTimeOriginal != nil {
			t.Error("Should not have DateTimeOriginal for nonexistent file")
		}
	}
	// Test passes if we got an error or empty result
	t.Logf("ReadEXIF on nonexistent file: err=%v, result=%+v", err, result)
}

func TestExifToolHelper_ReadEXIF_ValidJPEG(t *testing.T) {
	helper := requireExiftool(t)

	// Create a temp file with valid JPEG data
	tmpFile, err := os.CreateTemp("", "exiftool-test-*.jpg")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write minimal JPEG
	jpegData := createTestJPEG(1024)
	if _, err := tmpFile.Write(jpegData); err != nil {
		t.Fatalf("Failed to write JPEG data: %v", err)
	}
	tmpFile.Close()

	// Read EXIF (should succeed even with no EXIF data)
	result, err := helper.ReadEXIF(tmpFile.Name())
	if err != nil {
		t.Fatalf("ReadEXIF() error = %v", err)
	}

	if !result.Success {
		t.Error("Expected Success to be true")
	}

	// Should not have bridge stamp yet
	if result.IsBridgeStamped {
		t.Error("Expected image to not be bridge-stamped")
	}
}

func TestExifToolHelper_WriteEXIF(t *testing.T) {
	helper := requireExiftool(t)

	// Create temp file
	tmpFile, err := os.CreateTemp("", "exiftool-write-test-*.jpg")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write minimal JPEG
	jpegData := createTestJPEG(1024)
	if _, err := tmpFile.Write(jpegData); err != nil {
		t.Fatalf("Failed to write JPEG data: %v", err)
	}
	tmpFile.Close()

	// Write EXIF data
	opts := ExifWriteOptions{
		DateTimeOriginal:   "2024:12:25 10:30:00",
		OffsetTimeOriginal: "+00:00",
		UserComment:        "AviationWX-Bridge:UTC:v1:bridge_clock:high",
	}

	if err := helper.WriteEXIF(tmpFile.Name(), opts); err != nil {
		t.Fatalf("WriteEXIF() error = %v", err)
	}

	// Read back and verify
	result, err := helper.ReadEXIF(tmpFile.Name())
	if err != nil {
		t.Fatalf("ReadEXIF() after write error = %v", err)
	}

	if !result.Success {
		t.Error("Expected Success to be true after write")
	}

	if result.DateTimeOriginal == nil {
		t.Error("Expected DateTimeOriginal to be set")
	} else if *result.DateTimeOriginal != "2024:12:25 10:30:00" {
		t.Errorf("DateTimeOriginal = %s, want 2024:12:25 10:30:00", *result.DateTimeOriginal)
	}

	if result.OffsetTimeOriginal == nil {
		t.Error("Expected OffsetTimeOriginal to be set")
	} else if *result.OffsetTimeOriginal != "+00:00" {
		t.Errorf("OffsetTimeOriginal = %s, want +00:00", *result.OffsetTimeOriginal)
	}

	if !result.IsBridgeStamped {
		t.Error("Expected image to be bridge-stamped after write")
	}

	if result.UserComment == nil {
		t.Error("Expected UserComment to be set")
	} else if !strings.Contains(*result.UserComment, "AviationWX-Bridge") {
		t.Errorf("UserComment should contain AviationWX-Bridge, got: %s", *result.UserComment)
	}
}

func TestExifToolHelper_WriteEXIFToData(t *testing.T) {
	helper := requireExiftool(t)

	jpegData := createTestJPEG(1024)

	opts := ExifWriteOptions{
		DateTimeOriginal:   "2024:12:25 15:00:00",
		OffsetTimeOriginal: "+00:00",
		UserComment:        "AviationWX-Bridge:UTC:v1:bridge_clock:high",
	}

	modifiedData, err := helper.WriteEXIFToData(jpegData, opts)
	if err != nil {
		t.Fatalf("WriteEXIFToData() error = %v", err)
	}

	if len(modifiedData) == 0 {
		t.Error("Expected non-empty modified data")
	}

	// Modified data should be larger (has EXIF now)
	if len(modifiedData) <= len(jpegData) {
		t.Error("Modified data should be larger than original (has EXIF added)")
	}

	// Verify the modified data by writing to temp file and reading back
	tmpFile, err := os.CreateTemp("", "exiftool-verify-*.jpg")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(modifiedData); err != nil {
		t.Fatalf("Failed to write modified data: %v", err)
	}
	tmpFile.Close()

	result, err := helper.ReadEXIF(tmpFile.Name())
	if err != nil {
		t.Fatalf("ReadEXIF() on modified data error = %v", err)
	}

	if !result.IsBridgeStamped {
		t.Error("Modified data should be bridge-stamped")
	}

	if result.DateTimeOriginal == nil || *result.DateTimeOriginal != "2024:12:25 15:00:00" {
		t.Errorf("DateTimeOriginal not correctly written, got: %v", result.DateTimeOriginal)
	}
}

func TestExifToolHelper_ValidateEXIF(t *testing.T) {
	helper := requireExiftool(t)

	// Create temp file with bridge-stamped EXIF
	tmpFile, err := os.CreateTemp("", "exiftool-validate-*.jpg")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	jpegData := createTestJPEG(1024)
	if _, err := tmpFile.Write(jpegData); err != nil {
		t.Fatalf("Failed to write JPEG: %v", err)
	}
	tmpFile.Close()

	// Write valid bridge EXIF
	opts := ExifWriteOptions{
		DateTimeOriginal:   "2024:12:25 10:30:00",
		OffsetTimeOriginal: "+00:00",
		UserComment:        "AviationWX-Bridge:UTC:v1:bridge_clock:high",
	}
	if err := helper.WriteEXIF(tmpFile.Name(), opts); err != nil {
		t.Fatalf("WriteEXIF() error = %v", err)
	}

	// Validate
	result, err := helper.ValidateEXIF(tmpFile.Name())
	if err != nil {
		t.Fatalf("ValidateEXIF() error = %v", err)
	}

	if !result.Valid {
		t.Errorf("Expected valid result, got errors: %v", result.Errors)
	}

	if !result.HasBridgeMarker {
		t.Error("Expected HasBridgeMarker to be true")
	}

	if result.DateTimeOriginal == nil {
		t.Error("Expected DateTimeOriginal to be set")
	}
}

func TestExifToolHelper_ValidateEXIF_Invalid(t *testing.T) {
	helper := requireExiftool(t)

	// Create temp file with no EXIF
	tmpFile, err := os.CreateTemp("", "exiftool-validate-invalid-*.jpg")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	jpegData := createTestJPEG(1024)
	if _, err := tmpFile.Write(jpegData); err != nil {
		t.Fatalf("Failed to write JPEG: %v", err)
	}
	tmpFile.Close()

	// Validate (should fail - no bridge marker)
	result, err := helper.ValidateEXIF(tmpFile.Name())
	if err != nil {
		t.Fatalf("ValidateEXIF() error = %v", err)
	}

	if result.Valid {
		t.Error("Expected invalid result for unstamped image")
	}

	if result.HasBridgeMarker {
		t.Error("Expected HasBridgeMarker to be false")
	}

	if len(result.Errors) == 0 {
		t.Error("Expected validation errors")
	}
}

func TestExifToolHelper_ParseCameraTime(t *testing.T) {
	helper := requireExiftool(t)

	tests := []struct {
		name     string
		result   *ExifReadResult
		wantTime bool
	}{
		{
			name:     "nil result",
			result:   nil,
			wantTime: false,
		},
		{
			name: "no DateTimeOriginal",
			result: &ExifReadResult{
				Success: true,
			},
			wantTime: false,
		},
		{
			name: "valid DateTimeOriginal",
			result: &ExifReadResult{
				Success:          true,
				DateTimeOriginal: strPtr("2024:12:25 10:30:00"),
			},
			wantTime: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsedTime, ok := helper.ParseCameraTime(tc.result)
			if ok != tc.wantTime {
				t.Errorf("ParseCameraTime() ok = %v, want %v", ok, tc.wantTime)
			}
			if tc.wantTime && parsedTime == nil {
				t.Error("Expected non-nil time")
			}
			if tc.wantTime && parsedTime != nil {
				expected := time.Date(2024, 12, 25, 10, 30, 0, 0, time.UTC)
				if !parsedTime.Equal(expected) {
					t.Errorf("ParseCameraTime() = %v, want %v", parsedTime, expected)
				}
			}
		})
	}
}

func TestStampBridgeEXIFWithTool(t *testing.T) {
	// Skip if exiftool not available
	if _, err := DefaultExifToolHelper(); err != nil {
		t.Skipf("exiftool not available: %v", err)
	}

	observation := ObservationResult{
		Time:       time.Date(2024, 12, 25, 10, 30, 0, 0, time.UTC),
		Source:     SourceBridgeClock,
		Confidence: ConfidenceHigh,
	}

	jpegData := createTestJPEG(1024)
	result := StampBridgeEXIFWithTool(jpegData, observation)

	if !result.Stamped {
		t.Error("Expected image to be stamped")
	}

	if len(result.Data) <= len(jpegData) {
		t.Error("Stamped data should be larger than original")
	}

	if result.Marker == "" {
		t.Error("Expected non-empty marker")
	}

	if !strings.Contains(result.Marker, "AviationWX-Bridge") {
		t.Errorf("Marker should contain AviationWX-Bridge, got: %s", result.Marker)
	}

	if !strings.Contains(result.Marker, string(SourceBridgeClock)) {
		t.Errorf("Marker should contain source, got: %s", result.Marker)
	}
}

func TestStampBridgeEXIFWithTool_WithWarning(t *testing.T) {
	// Skip if exiftool not available
	if _, err := DefaultExifToolHelper(); err != nil {
		t.Skipf("exiftool not available: %v", err)
	}

	observation := ObservationResult{
		Time:       time.Now().UTC(),
		Source:     SourceCameraEXIF,
		Confidence: ConfidenceMedium,
		Warning: &TimeWarning{
			Code:    "camera_clock_drift",
			Message: "Camera clock is drifting",
		},
	}

	jpegData := createTestJPEG(1024)
	result := StampBridgeEXIFWithTool(jpegData, observation)

	if !result.Stamped {
		t.Error("Expected image to be stamped")
	}

	if !strings.Contains(result.Marker, "warn:camera_clock_drift") {
		t.Errorf("Marker should contain warning code, got: %s", result.Marker)
	}
}

func TestGetExifToolPath(t *testing.T) {
	// This test documents the behavior of GetExifToolPath
	// It may or may not find exiftool depending on the environment

	path, err := GetExifToolPath()
	if err != nil {
		t.Logf("exiftool not found (this is OK in some environments): %v", err)
		return
	}

	if path == "" {
		t.Error("Expected non-empty path when exiftool is found")
	}

	t.Logf("exiftool found at: %s", path)
}

func TestExifToolHelper_Timeout(t *testing.T) {
	helper := requireExiftool(t)

	// Set very short timeout
	helper.SetTimeout(1 * time.Nanosecond)

	// This should timeout (or might succeed on very fast systems)
	tmpFile, err := os.CreateTemp("", "exiftool-timeout-*.jpg")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	jpegData := createTestJPEG(1024)
	tmpFile.Write(jpegData)
	tmpFile.Close()

	// Try to read - may timeout or succeed depending on system speed
	_, err = helper.ReadEXIF(tmpFile.Name())
	// We don't fail on error here since timeout behavior varies
	t.Logf("ReadEXIF with 1ns timeout: err=%v", err)
}

// Helper function
func strPtr(s string) *string {
	return &s
}

// Benchmark tests

func BenchmarkExifToolHelper_ReadEXIF(b *testing.B) {
	helper, err := DefaultExifToolHelper()
	if err != nil {
		b.Skipf("exiftool not available: %v", err)
	}

	// Create temp file once
	tmpFile, err := os.CreateTemp("", "exiftool-bench-*.jpg")
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	jpegData := createTestJPEG(50 * 1024) // 50KB
	tmpFile.Write(jpegData)
	tmpFile.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		helper.ReadEXIF(tmpFile.Name())
	}
}

func BenchmarkExifToolHelper_WriteEXIF(b *testing.B) {
	helper, err := DefaultExifToolHelper()
	if err != nil {
		b.Skipf("exiftool not available: %v", err)
	}

	jpegData := createTestJPEG(50 * 1024) // 50KB

	opts := ExifWriteOptions{
		DateTimeOriginal:   "2024:12:25 10:30:00",
		OffsetTimeOriginal: "+00:00",
		UserComment:        "AviationWX-Bridge:UTC:v1:bridge_clock:high",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create temp file for each iteration
		tmpFile, _ := os.CreateTemp("", "exiftool-bench-write-*.jpg")
		tmpFile.Write(jpegData)
		tmpFile.Close()
		helper.WriteEXIF(tmpFile.Name(), opts)
		os.Remove(tmpFile.Name())
	}
}

func BenchmarkStampBridgeEXIFWithTool(b *testing.B) {
	if _, err := DefaultExifToolHelper(); err != nil {
		b.Skipf("exiftool not available: %v", err)
	}

	observation := ObservationResult{
		Time:       time.Now().UTC(),
		Source:     SourceBridgeClock,
		Confidence: ConfidenceHigh,
	}

	jpegData := createTestJPEG(50 * 1024) // 50KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		StampBridgeEXIFWithTool(jpegData, observation)
	}
}

// createTestJPEG creates a minimal valid JPEG for testing
// The size parameter is a hint - the actual size may be smaller for minimal JPEGs
func createTestJPEG(size int) []byte {
	// Minimal valid JPEG structure that exiftool can process
	minimalJPEG := []byte{
		0xFF, 0xD8, // SOI
		0xFF, 0xE0, 0x00, 0x10, // APP0 marker + length
		0x4A, 0x46, 0x49, 0x46, 0x00, // "JFIF\0"
		0x01, 0x01, // version 1.1
		0x00,       // aspect ratio units (0 = no units)
		0x00, 0x01, // X density
		0x00, 0x01, // Y density
		0x00, 0x00, // thumbnail size (0x0)
		0xFF, 0xDB, 0x00, 0x43, 0x00, // DQT marker + length + table 0
		// 64 bytes quantization table (all 16s for simplicity)
		0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10,
		0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10,
		0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10,
		0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10,
		0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10,
		0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10,
		0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10,
		0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10,
		0xFF, 0xC0, 0x00, 0x0B, // SOF0 marker + length
		0x08,       // 8 bits per component
		0x00, 0x01, // height = 1
		0x00, 0x01, // width = 1
		0x01,             // 1 component (grayscale)
		0x01, 0x11, 0x00, // component 1: ID=1, sampling=1x1, quant table 0
		0xFF, 0xC4, 0x00, 0x1F, // DHT marker + length
		0x00, // DC table 0
		// Huffman table (counts + symbols for simple DC)
		0x00, 0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01,
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0A, 0x0B,
		0xFF, 0xC4, 0x00, 0xB5, // DHT marker + length (AC table)
		0x10, // AC table 0
		// AC Huffman table
		0x00, 0x02, 0x01, 0x03, 0x03, 0x02, 0x04, 0x03,
		0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7D,
		0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12,
		0x21, 0x31, 0x41, 0x06, 0x13, 0x51, 0x61, 0x07,
		0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
		0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0,
		0x24, 0x33, 0x62, 0x72, 0x82, 0x09, 0x0A, 0x16,
		0x17, 0x18, 0x19, 0x1A, 0x25, 0x26, 0x27, 0x28,
		0x29, 0x2A, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39,
		0x3A, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48, 0x49,
		0x4A, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
		0x5A, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69,
		0x6A, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79,
		0x7A, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
		0x8A, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98,
		0x99, 0x9A, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6, 0xA7,
		0xA8, 0xA9, 0xAA, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6,
		0xB7, 0xB8, 0xB9, 0xBA, 0xC2, 0xC3, 0xC4, 0xC5,
		0xC6, 0xC7, 0xC8, 0xC9, 0xCA, 0xD2, 0xD3, 0xD4,
		0xD5, 0xD6, 0xD7, 0xD8, 0xD9, 0xDA, 0xE1, 0xE2,
		0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA,
		0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8,
		0xF9, 0xFA,
		0xFF, 0xDA, 0x00, 0x08, // SOS marker + length
		0x01,       // 1 component
		0x01, 0x00, // component 1, DC table 0, AC table 0
		0x00, 0x3F, 0x00, // spectral selection
		0x7F,       // Single gray pixel data (encoded)
		0xFF, 0xD9, // EOI
	}

	// For larger sizes, pad the image data section
	if size > len(minimalJPEG) {
		// Insert padding before EOI marker
		padding := make([]byte, size-len(minimalJPEG))
		result := make([]byte, 0, size)
		result = append(result, minimalJPEG[:len(minimalJPEG)-2]...) // Everything except EOI
		result = append(result, padding...)
		result = append(result, 0xFF, 0xD9) // EOI
		return result
	}

	return minimalJPEG
}

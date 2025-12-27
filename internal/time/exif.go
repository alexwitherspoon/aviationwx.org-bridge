package time

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// BridgeEXIFMarker is the marker string that identifies bridge-processed images
// Server looks for this in UserComment to know the timestamp is UTC
const BridgeEXIFMarker = "AviationWX-Bridge"

// EXIFStampResult contains the result of EXIF stamping
type EXIFStampResult struct {
	Data           []byte    // Image data (possibly with EXIF added)
	Stamped        bool      // Whether EXIF was successfully stamped
	Marker         string    // The marker string that was added
	ObservationUTC time.Time // The observation time that was stamped
}

// StampEXIF adds EXIF timestamp to image data if time is healthy.
// Returns the image data with EXIF timestamp if healthy, original data otherwise.
// If EXIF stamping fails, returns original data (fail gracefully).
func StampEXIF(imageData []byte, timeHealth *TimeHealth, captureTime time.Time) ([]byte, error) {
	// Only stamp EXIF if time is healthy
	if timeHealth != nil && !timeHealth.IsHealthy() {
		// Time unhealthy - return original data without EXIF
		// Server will estimate capture time using file mtime
		return imageData, nil
	}

	// Ensure capture time is in UTC
	if captureTime.IsZero() {
		captureTime = time.Now().UTC()
	} else {
		captureTime = captureTime.UTC()
	}

	// Format timestamp as EXIF DateTime format: "2006:01:02 15:04:05"
	exifDateTime := captureTime.Format("2006:01:02 15:04:05")

	// Try to add EXIF data
	result, err := addEXIFTimestamp(imageData, exifDateTime)
	if err != nil {
		// Fail gracefully - return original data if EXIF stamping fails
		// This ensures uploads continue even if EXIF fails
		return imageData, nil
	}

	return result, nil
}

// StampBridgeEXIF stamps image with observation time and bridge marker
// This is the primary method for stamping images from the bridge
func StampBridgeEXIF(imageData []byte, observation ObservationResult) EXIFStampResult {
	result := EXIFStampResult{
		Data:           imageData,
		Stamped:        false,
		ObservationUTC: observation.Time,
	}

	// Check if it's a JPEG
	if !isJPEG(imageData) {
		return result
	}

	// Build the marker string
	// Format: AviationWX-Bridge:UTC:v1:{source}:{confidence}[:warn:{code}]
	markerParts := []string{
		BridgeEXIFMarker,
		"UTC",
		"v1",
		string(observation.Source),
		string(observation.Confidence),
	}

	if observation.Warning != nil {
		markerParts = append(markerParts, "warn:"+observation.Warning.Code)
	}

	marker := strings.Join(markerParts, ":")
	result.Marker = marker

	// Format timestamp
	utcTimeStr := observation.Time.Format("2006:01:02 15:04:05")

	// Build EXIF data structure
	exifData := buildBridgeEXIF(utcTimeStr, marker, observation.Time)

	// Try to inject EXIF into JPEG
	stamped, err := injectEXIF(imageData, exifData)
	if err != nil {
		// Fail gracefully - return original data
		return result
	}

	result.Data = stamped
	result.Stamped = true
	return result
}

// ReadEXIFTimestamp attempts to read the DateTimeOriginal from EXIF
// Returns the parsed time and whether it was found
func ReadEXIFTimestamp(imageData []byte) (*time.Time, bool) {
	if !isJPEG(imageData) {
		return nil, false
	}

	// Find EXIF segment
	exifData := findEXIFSegment(imageData)
	if exifData == nil {
		return nil, false
	}

	// Parse DateTimeOriginal from EXIF
	dateTimeStr := findEXIFTag(exifData, 0x9003) // DateTimeOriginal tag
	if dateTimeStr == "" {
		dateTimeStr = findEXIFTag(exifData, 0x0132) // DateTime tag (fallback)
	}

	if dateTimeStr == "" {
		return nil, false
	}

	// Parse the EXIF datetime format: "2006:01:02 15:04:05"
	t, err := time.Parse("2006:01:02 15:04:05", dateTimeStr)
	if err != nil {
		return nil, false
	}

	return &t, true
}

// IsBridgeStamped checks if an image was stamped by the bridge
func IsBridgeStamped(imageData []byte) bool {
	if !isJPEG(imageData) {
		return false
	}

	// Look for bridge marker in image data
	// This is a simple check - in production we'd parse UserComment properly
	return bytes.Contains(imageData, []byte(BridgeEXIFMarker))
}

// HasEXIF checks if image data contains EXIF data
func HasEXIF(imageData []byte) bool {
	if !isJPEG(imageData) {
		return false
	}

	// Look for EXIF marker in first 64KB
	searchLen := len(imageData)
	if searchLen > 65536 {
		searchLen = 65536
	}

	return bytes.Contains(imageData[:searchLen], []byte("Exif"))
}

// IsJPEGComplete checks if a JPEG file is complete (has end marker)
func IsJPEGComplete(imageData []byte) bool {
	if len(imageData) < 4 {
		return false
	}

	// Check for JPEG SOI marker at start
	if imageData[0] != 0xFF || imageData[1] != 0xD8 {
		return false
	}

	// Check for JPEG EOI marker at end
	if imageData[len(imageData)-2] != 0xFF || imageData[len(imageData)-1] != 0xD9 {
		return false
	}

	return true
}

// Internal helper functions

func isJPEG(data []byte) bool {
	return len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8
}

// addEXIFTimestamp adds EXIF timestamp to JPEG image data.
// This is a simplified implementation that creates a basic EXIF segment.
func addEXIFTimestamp(imageData []byte, dateTime string) ([]byte, error) {
	if !isJPEG(imageData) {
		return imageData, nil
	}

	// Build simple EXIF with just DateTime
	exifData := buildSimpleEXIF(dateTime)
	return injectEXIF(imageData, exifData)
}

// buildSimpleEXIF creates a minimal EXIF segment with DateTimeOriginal
func buildSimpleEXIF(dateTime string) []byte {
	// EXIF structure:
	// - "Exif\0\0" identifier (6 bytes)
	// - TIFF header (8 bytes)
	// - IFD0 (root IFD)
	// - EXIF SubIFD with DateTimeOriginal

	var buf bytes.Buffer

	// Exif identifier
	buf.WriteString("Exif\x00\x00")

	// TIFF header (little endian)
	buf.Write([]byte{0x49, 0x49})             // Little endian marker "II"
	buf.Write([]byte{0x2A, 0x00})             // TIFF magic number
	buf.Write([]byte{0x08, 0x00, 0x00, 0x00}) // Offset to IFD0 (8 bytes from start of TIFF header)

	// IFD0 - we'll put 1 entry pointing to EXIF SubIFD
	tiffStart := 6 // After "Exif\0\0"

	// IFD0 entry count
	binary.Write(&buf, binary.LittleEndian, uint16(1))

	// IFD0 entry: ExifOffset tag (0x8769)
	// Tag ID, Type (LONG=4), Count, Value/Offset
	exifSubIFDOffset := uint32(8 + 2 + 12 + 4)              // IFD0 header + 1 entry + next IFD offset
	binary.Write(&buf, binary.LittleEndian, uint16(0x8769)) // ExifOffset tag
	binary.Write(&buf, binary.LittleEndian, uint16(4))      // LONG type
	binary.Write(&buf, binary.LittleEndian, uint32(1))      // Count
	binary.Write(&buf, binary.LittleEndian, exifSubIFDOffset)

	// Next IFD offset (0 = no more IFDs)
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	// EXIF SubIFD
	// Entry count
	binary.Write(&buf, binary.LittleEndian, uint16(1))

	// DateTime value offset (after this IFD)
	dateTimeOffset := uint32(buf.Len() - tiffStart + 2 + 12 + 4)

	// DateTimeOriginal entry (0x9003)
	binary.Write(&buf, binary.LittleEndian, uint16(0x9003)) // DateTimeOriginal tag
	binary.Write(&buf, binary.LittleEndian, uint16(2))      // ASCII type
	binary.Write(&buf, binary.LittleEndian, uint32(20))     // Count (19 chars + null)
	binary.Write(&buf, binary.LittleEndian, dateTimeOffset)

	// Next IFD offset (0 = no more)
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	// DateTime value (19 chars + null terminator)
	buf.WriteString(dateTime)
	buf.WriteByte(0)

	return buf.Bytes()
}

// buildBridgeEXIF creates an EXIF segment with bridge markers
func buildBridgeEXIF(dateTime, marker string, observationUTC time.Time) []byte {
	// For now, use simple EXIF and embed marker in a way that's detectable
	// A full implementation would properly set UserComment tag

	var buf bytes.Buffer

	// Exif identifier
	buf.WriteString("Exif\x00\x00")

	// TIFF header (little endian)
	buf.Write([]byte{0x49, 0x49})             // Little endian "II"
	buf.Write([]byte{0x2A, 0x00})             // TIFF magic
	buf.Write([]byte{0x08, 0x00, 0x00, 0x00}) // Offset to IFD0

	tiffStart := 6

	// IFD0 - 1 entry for ExifOffset
	binary.Write(&buf, binary.LittleEndian, uint16(1))

	exifSubIFDOffset := uint32(8 + 2 + 12 + 4)
	binary.Write(&buf, binary.LittleEndian, uint16(0x8769)) // ExifOffset
	binary.Write(&buf, binary.LittleEndian, uint16(4))      // LONG
	binary.Write(&buf, binary.LittleEndian, uint32(1))
	binary.Write(&buf, binary.LittleEndian, exifSubIFDOffset)

	binary.Write(&buf, binary.LittleEndian, uint32(0)) // No next IFD

	// EXIF SubIFD - 3 entries: DateTimeOriginal, OffsetTimeOriginal, UserComment
	binary.Write(&buf, binary.LittleEndian, uint16(3))

	currentOffset := buf.Len() - tiffStart

	// Calculate offsets for values that don't fit inline
	valuesOffset := uint32(currentOffset + 3*12 + 4) // After 3 entries + next IFD

	// DateTimeOriginal (0x9003) - 20 bytes, stored externally
	dateTimeOffset := valuesOffset
	binary.Write(&buf, binary.LittleEndian, uint16(0x9003))
	binary.Write(&buf, binary.LittleEndian, uint16(2)) // ASCII
	binary.Write(&buf, binary.LittleEndian, uint32(20))
	binary.Write(&buf, binary.LittleEndian, dateTimeOffset)

	// OffsetTimeOriginal (0x9011) - "+00:00" = 7 bytes, stored externally
	offsetTimeOffset := dateTimeOffset + 20
	binary.Write(&buf, binary.LittleEndian, uint16(0x9011))
	binary.Write(&buf, binary.LittleEndian, uint16(2)) // ASCII
	binary.Write(&buf, binary.LittleEndian, uint32(7))
	binary.Write(&buf, binary.LittleEndian, offsetTimeOffset)

	// UserComment (0x9286) - variable length
	markerBytes := []byte(marker)
	// UserComment format: 8-byte charset identifier + data
	userCommentData := append([]byte("ASCII\x00\x00\x00"), markerBytes...)
	userCommentOffset := offsetTimeOffset + 7
	binary.Write(&buf, binary.LittleEndian, uint16(0x9286))
	binary.Write(&buf, binary.LittleEndian, uint16(7)) // UNDEFINED type
	binary.Write(&buf, binary.LittleEndian, uint32(len(userCommentData)))
	binary.Write(&buf, binary.LittleEndian, userCommentOffset)

	// Next IFD offset
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	// Values
	buf.WriteString(dateTime)
	buf.WriteByte(0)          // Null terminator
	buf.WriteString("+00:00") // UTC offset
	buf.WriteByte(0)          // Null terminator
	buf.Write(userCommentData)

	return buf.Bytes()
}

// injectEXIF injects EXIF data into a JPEG
func injectEXIF(imageData []byte, exifData []byte) ([]byte, error) {
	if len(imageData) < 2 {
		return nil, fmt.Errorf("image too small")
	}

	// Find position after SOI marker (first 2 bytes)
	// We'll insert APP1 (EXIF) segment there

	var result bytes.Buffer

	// Write SOI marker
	result.Write(imageData[:2])

	// Write APP1 marker (0xFF 0xE1)
	result.WriteByte(0xFF)
	result.WriteByte(0xE1)

	// Write APP1 length (2 bytes for length + exif data)
	length := uint16(len(exifData) + 2)
	binary.Write(&result, binary.BigEndian, length)

	// Write EXIF data
	result.Write(exifData)

	// Write rest of original image (skip SOI, and any existing APP1)
	pos := 2
	for pos < len(imageData)-1 {
		if imageData[pos] == 0xFF {
			marker := imageData[pos+1]
			if marker == 0xE1 {
				// Skip existing APP1 segment
				if pos+3 < len(imageData) {
					segLen := int(imageData[pos+2])<<8 | int(imageData[pos+3])
					pos += 2 + segLen
					continue
				}
			}
		}
		break
	}

	// Write remaining image data
	result.Write(imageData[pos:])

	return result.Bytes(), nil
}

// findEXIFSegment finds and returns the EXIF segment data
func findEXIFSegment(imageData []byte) []byte {
	if len(imageData) < 4 {
		return nil
	}

	pos := 2 // After SOI
	for pos < len(imageData)-3 {
		if imageData[pos] != 0xFF {
			pos++
			continue
		}

		marker := imageData[pos+1]
		if marker == 0xE1 { // APP1
			segLen := int(imageData[pos+2])<<8 | int(imageData[pos+3])
			if pos+2+segLen <= len(imageData) {
				segment := imageData[pos+4 : pos+2+segLen]
				// Check for "Exif\0\0" prefix
				if len(segment) > 6 && bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
					return segment[6:] // Return TIFF data after "Exif\0\0"
				}
			}
		}

		// Move to next segment
		if pos+3 < len(imageData) {
			segLen := int(imageData[pos+2])<<8 | int(imageData[pos+3])
			pos += 2 + segLen
		} else {
			break
		}
	}

	return nil
}

// findEXIFTag finds a tag value in EXIF data (simplified)
func findEXIFTag(exifData []byte, tagID uint16) string {
	// This is a simplified implementation
	// A full implementation would properly parse TIFF/EXIF structure

	if len(exifData) < 8 {
		return ""
	}

	// Check byte order
	var byteOrder binary.ByteOrder
	if exifData[0] == 0x49 && exifData[1] == 0x49 {
		byteOrder = binary.LittleEndian
	} else if exifData[0] == 0x4D && exifData[1] == 0x4D {
		byteOrder = binary.BigEndian
	} else {
		return ""
	}

	// Get IFD0 offset
	ifdOffset := byteOrder.Uint32(exifData[4:8])
	if int(ifdOffset) >= len(exifData) {
		return ""
	}

	// Parse IFD entries (simplified - just looks for the tag)
	// This would need to be expanded to properly handle EXIF SubIFD
	return searchIFDForTag(exifData, int(ifdOffset), tagID, byteOrder)
}

func searchIFDForTag(data []byte, offset int, tagID uint16, order binary.ByteOrder) string {
	if offset+2 > len(data) {
		return ""
	}

	entryCount := order.Uint16(data[offset : offset+2])
	offset += 2

	for i := 0; i < int(entryCount); i++ {
		if offset+12 > len(data) {
			break
		}

		tag := order.Uint16(data[offset : offset+2])
		tagType := order.Uint16(data[offset+2 : offset+4])
		count := order.Uint32(data[offset+4 : offset+8])

		if tag == tagID && tagType == 2 { // ASCII type
			valueOffset := order.Uint32(data[offset+8 : offset+12])
			if int(valueOffset)+int(count) <= len(data) {
				value := string(data[valueOffset : valueOffset+count-1]) // -1 for null terminator
				return value
			}
		}

		// Check for ExifOffset tag to recurse into EXIF SubIFD
		if tag == 0x8769 { // ExifOffset
			exifSubIFDOffset := order.Uint32(data[offset+8 : offset+12])
			result := searchIFDForTag(data, int(exifSubIFDOffset), tagID, order)
			if result != "" {
				return result
			}
		}

		offset += 12
	}

	return ""
}

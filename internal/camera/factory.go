package camera

import (
	"fmt"
)

// NewCamera creates a camera instance based on the configuration type.
// Supports "http", "onvif", and "rtsp" camera types.
// Returns an error if the camera type is unsupported or configuration is invalid.
func NewCamera(config Config) (Camera, error) {
	switch config.Type {
	case "http":
		return NewHTTPCamera(config)
	case "onvif":
		return NewONVIFCamera(config)
	case "rtsp":
		return NewRTSPCamera(config)
	default:
		return nil, fmt.Errorf("unsupported camera type: %s", config.Type)
	}
}

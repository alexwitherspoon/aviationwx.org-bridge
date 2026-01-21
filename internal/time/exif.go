package time

import "time"

// EXIFStampResult contains the result of EXIF stamping
type EXIFStampResult struct {
	Data           []byte    // Image data (possibly with EXIF added)
	Stamped        bool      // Whether EXIF was successfully stamped
	Marker         string    // The marker string that was added
	ObservationUTC time.Time // The observation time that was stamped
}

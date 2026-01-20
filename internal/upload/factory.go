package upload

import (
	"github.com/alexwitherspoon/aviationwx-bridge/internal/config"
)

// NewClientFromConfig creates an upload client from the config package's Upload type.
// Converts the config.Upload to upload.Config and creates the appropriate client.
func NewClientFromConfig(cfg config.Upload) (Client, error) {
	// Default DisableEPSV to true (use standard PASV) if not specified
	disableEPSV := true
	if cfg.DisableEPSV != nil {
		disableEPSV = *cfg.DisableEPSV
	}

	uploadConfig := Config{
		Host:                  cfg.Host,
		Port:                  cfg.Port,
		Username:              cfg.Username,
		Password:              cfg.Password,
		TLS:                   cfg.TLS,
		TLSVerify:             cfg.TLSVerify,
		CABundlePath:          cfg.CABundlePath,
		TimeoutConnectSeconds: cfg.TimeoutConnectSeconds,
		TimeoutUploadSeconds:  cfg.TimeoutUploadSeconds,
		DisableEPSV:           disableEPSV,
	}

	return NewFTPSClient(uploadConfig)
}

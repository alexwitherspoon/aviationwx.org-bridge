package config

import (
	"testing"
)

func TestNormalizeUploadConfig_Nil(t *testing.T) {
	NormalizeUploadConfig(nil)
	// Must not panic
}

func TestNormalizeUploadConfig_FTPSMigration(t *testing.T) {
	tests := []struct {
		name     string
		upload   *Upload
		wantPort int
	}{
		{
			name: "ftps port 21 migrates to 2222",
			upload: &Upload{
				Protocol: "ftps",
				Port:     21,
			},
			wantPort: 2222,
		},
		{
			name: "ftps port 2121 migrates to 2222",
			upload: &Upload{
				Protocol: "ftps",
				Port:     2121,
			},
			wantPort: 2222,
		},
		{
			name: "ftps port 990 migrates to 2222",
			upload: &Upload{
				Protocol: "ftps",
				Port:     990,
			},
			wantPort: 2222,
		},
		{
			name: "ftp port 21 migrates to 2222",
			upload: &Upload{
				Protocol: "ftp",
				Port:     21,
			},
			wantPort: 2222,
		},
		{
			name: "ftps custom port preserved",
			upload: &Upload{
				Protocol: "ftps",
				Port:     9900,
			},
			wantPort: 9900,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			NormalizeUploadConfig(tt.upload)
			if tt.upload.Protocol != "sftp" {
				t.Errorf("Protocol = %q, want sftp", tt.upload.Protocol)
			}
			if tt.upload.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", tt.upload.Port, tt.wantPort)
			}
		})
	}
}

func TestNormalizeUploadConfig_EmptyProtocolPortInference(t *testing.T) {
	tests := []struct {
		name     string
		upload   *Upload
		wantPort int
	}{
		{
			name: "empty protocol port 21 migrates to sftp 2222",
			upload: &Upload{
				Port: 21,
			},
			wantPort: 2222,
		},
		{
			name: "empty protocol port 2121 migrates to sftp 2222",
			upload: &Upload{
				Port: 2121,
			},
			wantPort: 2222,
		},
		{
			name: "empty protocol port 990 migrates to sftp 2222",
			upload: &Upload{
				Port: 990,
			},
			wantPort: 2222,
		},
		{
			name: "empty protocol port 22 stays sftp 22",
			upload: &Upload{
				Port: 22,
			},
			wantPort: 22,
		},
		{
			name: "empty protocol port 2222 stays sftp 2222",
			upload: &Upload{
				Port: 2222,
			},
			wantPort: 2222,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			NormalizeUploadConfig(tt.upload)
			if tt.upload.Protocol != "sftp" {
				t.Errorf("Protocol = %q, want sftp", tt.upload.Protocol)
			}
			if tt.upload.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", tt.upload.Port, tt.wantPort)
			}
		})
	}
}

func TestNormalizeUploadConfig_Defaults(t *testing.T) {
	upload := &Upload{
		Host:     "upload.example.com",
		Username: "user",
		Password: "pass",
	}
	NormalizeUploadConfig(upload)
	if upload.Protocol != "sftp" {
		t.Errorf("Protocol = %q, want sftp", upload.Protocol)
	}
	if upload.Port != 2222 {
		t.Errorf("Port = %d, want 2222", upload.Port)
	}
	if upload.TimeoutConnectSeconds != 60 {
		t.Errorf("TimeoutConnectSeconds = %d, want 60", upload.TimeoutConnectSeconds)
	}
	if upload.TimeoutUploadSeconds != 300 {
		t.Errorf("TimeoutUploadSeconds = %d, want 300", upload.TimeoutUploadSeconds)
	}
}

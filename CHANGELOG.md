# Changelog

All notable changes to AviationWX Bridge will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Web Console**: Modern web-based configuration interface
  - Dashboard with real-time status, camera overview, and queue health
  - Camera management (add, edit, delete cameras via UI)
  - Timezone configuration with live UTC/local time preview
  - Setup wizard for first-time installation
  - Mobile-friendly responsive design
  - Version display with update notification badge
  
- **Per-Camera Upload Credentials**: Each camera now has its own FTP credentials
  - Default upload server: `upload.aviationwx.org`
  - Contact `contact@aviationwx.org` for credentials

- **Image Quality & Bandwidth Control**: Optional per-camera image processing
  - Default: use original image as-is (no processing)
  - Optional max resolution (width/height) for bandwidth control
  - Optional JPEG quality setting (1-100) for re-encoding
  - Presets in UI: Original (default), High (1080p), Medium (720p), Low (480p)
  
- **Queue Management System**: File-based queue for historic image replay
  - Memory-backed tmpfs storage for Raspberry Pi SD card longevity
  - Configurable queue limits (files, size, age)
  - Health-based thinning to prevent memory exhaustion
  - Emergency pruning for critical situations
  
- **Time Authority System**: Accurate observation timestamp handling
  - NTP health monitoring with configurable servers
  - Camera EXIF validation and correction
  - Bridge-stamped UTC times with markers
  - Configurable drift tolerance
  
- **Exiftool Integration**: Server-compatible EXIF parsing
  - Uses same `exiftool` as aviationwx.org server
  - Validates bridge-written EXIF is readable
  - Bundled in Docker container (`perl-image-exiftool`)
  
- **Scheduler Orchestration**: Decoupled capture and upload workers
  - Per-camera capture goroutines
  - Round-robin upload worker
  - Fail2ban-aware retry logic with backoff
  
- **Automatic Updates** (Raspberry Pi install):
  - Supervisor script checks for updates every 6 hours
  - Normal updates: notification only
  - Critical updates: auto-apply after 24hr grace period
  - Automatic rollback on health check failure
  
- **GitHub Actions CI/CD**:
  - Automated testing with exiftool
  - golangci-lint for code quality
  - Multi-architecture Docker builds (amd64, arm64, armv7)
  - Pre-built binaries for Linux and macOS
  - Semantic versioning with release metadata

- **Structured Logging**: Using Go's `log/slog`
  - Configurable log level (debug, info, warn, error)
  - JSON or text format output

- **Update Checker**: Compares running version with GitHub releases
  - Shows update badge in web UI
  - Provides release URL for manual updates

- **Local CI Testing**: `./scripts/test-ci-local.sh` for pre-push validation

### Changed
- Config version upgraded to v2
- `interval_seconds` renamed to `capture_interval_seconds`
- Upload configuration moved from global to per-camera
- Default web console port changed to `1229` (was 8080)
- Default web console password is `aviationwx`
- EXIF handling migrated from PHP to exiftool CLI

### Deprecated
- Global `upload` configuration (use per-camera instead)
- `basic_auth` in web_console (use `password` instead)
- `remote_path` in camera config (always uploads to root folder)

### Removed
- PHP EXIF helper (replaced by exiftool)

## [0.1.0] - Initial Development

### Added
- Camera capture support (HTTP, RTSP, ONVIF)
- FTPS upload with TLS
- Basic configuration via JSON
- Degraded mode for network issues
- Backoff and retry logic

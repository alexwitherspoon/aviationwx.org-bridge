# Local Development Setup

This guide covers setting up a local development environment for AviationWX Bridge.

## Prerequisites

- **Go 1.24+**: [Install Go](https://go.dev/doc/install)
- **Docker & Docker Compose**: [Install Docker](https://docs.docker.com/get-docker/)
- **exiftool**: For EXIF metadata handling
- **Git**: For version control

### Install exiftool

```bash
# macOS
brew install exiftool

# Ubuntu/Debian
sudo apt-get install libimage-exiftool-perl

# Verify installation
exiftool -ver
```

## Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/alexwitherspoon/aviationwx-bridge.git
cd aviationwx-bridge
```

### 2. Install Dependencies

```bash
# Download Go dependencies
go mod download

# Verify dependencies
go mod verify
```

### 3. Run Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package tests
go test -v ./internal/time/...
```

### 4. Run Locally (Go)

```bash
# Build binary
go build -o bridge ./cmd/bridge

# Run (will use defaults, web console at :1229)
./bridge

# Or use go run
go run ./cmd/bridge
```

### 5. Run with Docker

```bash
# Build and start
cd docker
docker compose up --build

# Or from project root
docker compose -f docker/docker-compose.yml up --build
```

Access the web console at `http://localhost:1229`  
Default password: `aviationwx`

## Development Workflow

### Local CI Testing

We provide a script that mimics CI locally:

```bash
# Run all checks (tests, lint, build, docker)
./scripts/test-ci-local.sh all

# Run individual steps
./scripts/test-ci-local.sh test    # Go tests
./scripts/test-ci-local.sh lint    # golangci-lint  
./scripts/test-ci-local.sh build   # Multi-arch build
./scripts/test-ci-local.sh docker  # Docker build
```

### Code Quality

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
golangci-lint run ./...

# Format code
go fmt ./...

# Vet code
go vet ./...
```

### Building

```bash
# Build for current platform
go build -o bridge ./cmd/bridge

# Build with version info
GIT_COMMIT=$(git rev-parse --short HEAD)
go build -ldflags="-X main.Version=dev -X main.GitCommit=${GIT_COMMIT}" -o bridge ./cmd/bridge

# Cross-compile for Raspberry Pi
GOOS=linux GOARCH=arm64 go build -o bridge-arm64 ./cmd/bridge
GOOS=linux GOARCH=arm GOARM=7 go build -o bridge-armv7 ./cmd/bridge
```

## Configuration

### Local Config File

Create a local config at `docker/data/config.json`:

```json
{
  "version": 2,
  "timezone": "America/Chicago",
  "cameras": [
    {
      "id": "test-camera",
      "name": "Test Camera",
      "type": "http",
      "enabled": true,
      "snapshot_url": "http://192.168.1.100/snapshot.jpg",
      "capture_interval_seconds": 60,
      "upload": {
        "host": "upload.aviationwx.org",
        "username": "test-user",
        "password": "test-password",
        "tls": true
      }
    }
  ],
  "web_console": {
    "enabled": true,
    "port": 1229,
    "password": "dev-password"
  }
}
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `AVIATIONWX_CONFIG` | `/data/config.json` | Config file path |
| `AVIATIONWX_QUEUE_PATH` | `/dev/shm/aviationwx` | Queue storage path |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `LOG_FORMAT` | `text` | Log format (text, json) |

## Docker Development

### Docker Compose

```bash
cd docker

# Start with live rebuild
docker compose up --build

# View logs
docker compose logs -f

# Stop
docker compose down

# Clean rebuild
docker compose build --no-cache
docker compose up
```

### Building Multi-Arch Images

```bash
# Enable buildx
docker buildx create --use

# Build for multiple platforms (no push)
docker buildx build \
  --platform linux/amd64,linux/arm64,linux/arm/v7 \
  -f docker/Dockerfile \
  -t aviationwx-bridge:local \
  .
```

## Testing

### Unit Tests

```bash
# All tests
go test ./...

# With race detection
go test -race ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Integration Tests

```bash
# Tests that require exiftool
go test -v ./internal/time/...

# Tests that require network (NTP)
go test -v -run Integration ./internal/time/...
```

### Docker Smoke Test

```bash
# Build and test container starts
docker build -f docker/Dockerfile -t aviationwx-bridge:test .
docker run --rm aviationwx-bridge:test exiftool -ver
```

## Troubleshooting

### Go Module Issues

```bash
# Clean and re-download
go clean -modcache
go mod download
go mod verify
```

### exiftool Not Found

Ensure exiftool is in PATH:

```bash
which exiftool
exiftool -ver

# If not found, tests will be skipped with message:
# "exiftool not available: exiftool not found"
```

### Port Already in Use

Change the port in docker-compose.yml or config:

```yaml
ports:
  - "8080:1229"  # Map to different host port
```

### Docker Build Failures

```bash
# Clean Docker cache
docker builder prune -f

# Rebuild without cache
docker compose build --no-cache
```

## IDE Setup

### VS Code

Recommended extensions:
- Go (official)
- Docker
- YAML
- EditorConfig

Settings (`.vscode/settings.json`):
```json
{
  "go.lintTool": "golangci-lint",
  "go.lintFlags": ["--fast"],
  "go.testTimeout": "60s"
}
```

### GoLand / IntelliJ

1. Open project directory
2. Go SDK should auto-detect (ensure Go 1.24+)
3. Enable Go Modules integration
4. Set test timeout to 60s

## Next Steps

- Read [CONFIG_SCHEMA.md](CONFIG_SCHEMA.md) for configuration reference
- Read [DEPLOYMENT.md](DEPLOYMENT.md) for production deployment
- Read [../CONTRIBUTING.md](../CONTRIBUTING.md) for contribution guidelines

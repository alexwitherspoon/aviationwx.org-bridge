# Local Development & Testing Guide

This guide explains how to test the AviationWX Bridge locally using Docker, matching the production environment.

## ⚠️ CRITICAL: Data Preservation

**SAFETY-CRITICAL SYSTEM - NEVER delete production data!**

The `docker/data/` directory contains your **production camera configurations**. Before any destructive testing:

1. **BACKUP your data**: `cp -r docker/data docker/data.backup`
2. **Use test directories**: Create separate test directories for experiments
3. **NEVER run `rm -rf data/` on production systems**

### Safe Testing Commands

```bash
# ✅ SAFE: Clean test environment
cd docker
mkdir -p data-test
AVIATIONWX_CONFIG_DIR=./data-test docker compose up --build

# ❌ DANGEROUS: Deletes all camera configs!
rm -rf data/  # NEVER DO THIS on production data!

# ✅ SAFE: Backup before clean slate testing
cp -r data data.backup
rm -rf data/
docker compose up --build
# Restore if needed: rm -rf data && mv data.backup data
```

## Quick Start (Docker - Recommended)

**This is the preferred way to test locally as it matches production deployment.**

### Production Testing (Uses Real Data)
```bash
# 1. Build and start the bridge with production config
cd docker
docker compose up --build

# 2. Open web UI
open http://localhost:1229
# Default password: aviationwx

# 3. View logs (automatically limited to 10MB with rotation)
docker compose logs -f

# 4. Stop
docker compose down
```

### Test/Development Mode (Isolated from Production)
**⚠️ USE THIS for CI testing and development to avoid affecting production data!**

```bash
# 1. Use the test override (different port + separate data directory)
cd docker
docker compose -f docker-compose.yml -f docker-compose.test.yml up --build

# 2. Open test web UI on different port
open http://localhost:1230  # Note: 1230, not 1229!
# Default password: aviationwx

# 3. Test data goes to separate directory
ls -la data-test/  # Test configs here
ls -la data/       # Production configs safe here

# 4. Stop test container
docker compose -f docker-compose.yml -f docker-compose.test.yml down

# 5. Clean test environment (safe - doesn't touch production)
rm -rf data-test/
```

### When to Use Each Mode

| Mode | Port | Data Dir | Container Name | Use Case |
|------|------|----------|----------------|----------|
| **Production** | 1229 | `data/` | `aviationwx-bridge` | Real cameras, production testing |
| **Test** | 1230 | `data-test/` | `aviationwx-bridge-test` | Development, CI, experiments |

### Why Test Mode is Critical
✅ **No data loss**: Production camera configs remain untouched  
✅ **No port conflicts**: Run prod and test containers simultaneously  
✅ **Clean testing**: Wipe `data-test/` without fear  
✅ **CI safety**: Automated tests never corrupt real data  
✅ **Parallel testing**: Test new features while prod runs

### Log Management
Logs are automatically limited to 10MB total:
- 2 files × 5MB each
- Automatic rotation when 5MB is reached
- Old logs are compressed
- No manual cleanup needed

## Docker Testing Workflow

### Initial Setup
```bash
cd /Users/alexwitherspoon/GitHub/aviationwx-bridge/docker

# Build image
docker compose build

# Start in foreground (see logs immediately)
docker compose up

# Or start in background
docker compose up -d

# View logs
docker compose logs -f bridge
```

### Configuration Structure
```
docker/
  └── data/
      ├── global.json       # Bridge-wide settings
      └── cameras/
          ├── cam1.json     # Individual camera configs
          └── cam2.json
```

### Testing Scenarios

#### 1. Fresh Installation (⚠️ DESTRUCTIVE - Backup First!)
```bash
# ALWAYS backup production data first!
cd docker
cp -r data data.backup.$(date +%Y%m%d_%H%M%S)

# Now safe to wipe
rm -rf data/
docker compose up --build

# Expected: Bridge starts with defaults
# - Web UI on http://localhost:1229
# - Timezone: UTC
# - No cameras configured
# - SNTP enabled with defaults
```

#### 2. Migration from Legacy Config
```bash
# Create old-style config.json
cd docker
mkdir -p data
cat > data/config.json << 'EOF'
{
  "version": 2,
  "timezone": "America/New_York",
  "cameras": [
    {
      "id": "test-cam",
      "name": "Test Camera",
      "type": "http",
      "enabled": false,
      "snapshot_url": "http://example.com/snap.jpg",
      "capture_interval_seconds": 60,
      "upload": {
        "host": "upload.aviationwx.org",
        "port": 2121,
        "username": "youruser",
        "password": "yourpass",
        "tls": true
      }
    }
  ],
  "web_console": {
    "enabled": true,
    "port": 1229,
    "password": "aviationwx"
  }
}
EOF

# Start bridge
docker compose up

# Expected:
# - Sees config.json
# - Migrates to new structure (global.json + cameras/test-cam.json)
# - Backs up original as config.json.migrated
# - All settings preserved
```

#### 3. Config Persistence
```bash
# Add camera via web UI
# Then restart
docker compose restart

# Expected:
# - Camera still there
# - Settings preserved
# - No data loss
```

#### 4. Hot-Reload Testing
```bash
# While bridge is running:
# 1. Add a camera via web UI
# 2. Check logs: docker compose logs -f
# Expected:
# - "Config event received type=camera_added"
# - "Camera worker started successfully"
# - No restart needed

# 3. Update camera via web UI
# Expected:
# - "Config event received type=camera_updated"
# - Worker stops and restarts with new config

# 4. Delete camera via web UI
# Expected:
# - "Config event received type=camera_deleted"
# - Worker stops gracefully
```

#### 5. File Permission Testing
```bash
# Check file permissions inside container
docker compose exec bridge ls -la /data/
docker compose exec bridge ls -la /data/cameras/

# Expected:
# drwxr-xr-x  /data/cameras/
# -rw-r--r--  /data/global.json
# -rw-r--r--  /data/cameras/*.json
```

#### 6. Multi-Camera Testing
```bash
# Add multiple cameras via web UI
# Each should get its own file

docker compose exec bridge ls -la /data/cameras/

# Expected:
# cam1.json
# cam2.json
# cam3.json

# Update one camera - others should be unaffected
```

### Troubleshooting

#### View Real-Time Logs
```bash
docker compose logs -f bridge
```

#### Access Container Shell
```bash
docker compose exec bridge /bin/sh

# Inside container:
ls -la /data/
cat /data/global.json
cat /data/cameras/*.json
ps aux | grep bridge
```

#### Check Config Files
```bash
# From host (macOS)
cat docker/data/global.json | jq '.'
cat docker/data/cameras/*.json | jq '.'
```

#### Reset Everything (⚠️ DATA LOSS WARNING!)
```bash
# CRITICAL: This deletes ALL camera configurations!
# Only use for testing with backed-up or non-production data

# 1. Create timestamped backup
cd docker
cp -r data data.backup.$(date +%Y%m%d_%H%M%S)

# 2. Now safe to reset
docker compose down
rm -rf data/
docker compose up --build

# 3. To restore backup if needed:
# rm -rf data && mv data.backup.YYYYMMDD_HHMMSS data
```

## Native Testing (Alternative)

If you need to test without Docker:

```bash
# Build
cd /Users/alexwitherspoon/GitHub/aviationwx-bridge
make build

# Create test config directory
mkdir -p /tmp/aviationwx-test

# Run
AVIATIONWX_CONFIG_DIR=/tmp/aviationwx-test \
AVIATIONWX_QUEUE_PATH=/tmp/aviationwx-test/queue \
AVIATIONWX_LOG_LEVEL=debug \
./bin/bridge

# Open http://localhost:1229
```

## API Testing

### Using curl
```bash
# Get status
curl -u "admin:aviationwx" http://localhost:1229/api/status | jq '.'

# Get config
curl -u "admin:aviationwx" http://localhost:1229/api/config | jq '.'

# List cameras
curl -u "admin:aviationwx" http://localhost:1229/api/cameras | jq '.'

# Update timezone
curl -u "admin:aviationwx" -X PUT \
  -H "Content-Type: application/json" \
  -d '{"timezone":"America/Los_Angeles"}' \
  http://localhost:1229/api/time

# Add camera
curl -u "admin:aviationwx" -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-cam",
    "name": "Test Camera",
    "type": "http",
    "enabled": false,
    "snapshot_url": "http://example.com/snap.jpg",
    "capture_interval_seconds": 60,
    "upload": {
      "host": "upload.aviationwx.org",
      "port": 2121,
      "username": "user",
      "password": "pass",
      "tls": true
    }
  }' \
  http://localhost:1229/api/cameras | jq '.'

# Update camera
curl -u "admin:aviationwx" -X PUT \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-cam",
    "name": "Updated Name",
    "type": "http",
    "enabled": false,
    "snapshot_url": "http://example.com/snap2.jpg",
    "capture_interval_seconds": 120,
    "upload": {
      "host": "upload.aviationwx.org",
      "port": 2121,
      "username": "user",
      "password": "",
      "tls": true
    }
  }' \
  http://localhost:1229/api/cameras/test-cam | jq '.'

# Delete camera
curl -u "admin:aviationwx" -X DELETE \
  http://localhost:1229/api/cameras/test-cam
```

## Automated Tests

```bash
# Run all tests
cd /Users/alexwitherspoon/GitHub/aviationwx-bridge
go test ./...

# Run specific package tests
go test ./internal/config -v
go test ./internal/web -v
go test ./cmd/bridge -v

# Run with coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## CI/CD Local Testing

```bash
# Test what CI will run
cd /Users/alexwitherspoon/GitHub/aviationwx-bridge
./scripts/test-ci-local.sh
```

## Common Issues

### Issue: "Permission denied" saving config
**Cause**: Directory permissions incorrect  
**Fix**: 
```bash
cd docker
docker compose down
sudo chown -R 1000:1000 data/
docker compose up
```

### Issue: "Config data lost after update"
**Cause**: Old version before v2.0 refactor  
**Fix**: 
```bash
docker compose down
docker compose pull  # Get latest
docker compose up
```

### Issue: "Web UI shows old data"
**Cause**: Browser cache  
**Fix**: Hard refresh (Cmd+Shift+R on Mac, Ctrl+Shift+R on Linux/Windows)

### Issue: "Camera not starting"
**Cause**: Check worker status in web UI or logs  
**Fix**: 
```bash
docker compose logs -f | grep -E "(error|WARN|worker)"
```

## Production-Like Testing

To test exactly like production (Raspberry Pi):

```bash
# Build for ARM64
cd /Users/alexwitherspoon/GitHub/aviationwx-bridge
docker buildx build --platform linux/arm64 \
  -f docker/Dockerfile \
  -t aviationwx-bridge:test-arm64 .

# Run ARM64 image (on Mac with Apple Silicon)
docker run --rm -p 1229:1229 \
  -v $(pwd)/docker/data:/data \
  --tmpfs /dev/shm:size=200m \
  aviationwx-bridge:test-arm64
```

## Best Practices

1. **Always use Docker for local testing** - Matches production
2. **Test config changes** - Add/edit/delete cameras, change settings
3. **Verify persistence** - Restart container, config should survive
4. **Check logs** - Look for errors/warnings
5. **Test hot-reload** - Changes should apply without restart
6. **Verify file structure** - Check `data/global.json` and `data/cameras/*.json`
7. **Test migration** - Start with old `config.json`, verify migration
8. **Clean slate testing** - Regularly test fresh installations

## Quick Verification Checklist

After any code changes, verify:

- [ ] `docker compose up --build` succeeds
- [ ] Web UI loads at http://localhost:1229
- [ ] Can add a camera
- [ ] Can edit camera (no data loss!)
- [ ] Can delete camera
- [ ] Can change timezone
- [ ] Config persists after restart
- [ ] Files created in `data/cameras/`
- [ ] Logs show no errors
- [ ] All tests pass: `go test ./...`

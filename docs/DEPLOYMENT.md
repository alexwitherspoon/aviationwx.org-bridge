# Deployment Guide

This guide covers deploying AviationWX Bridge to production.

## Deployment Options

Choose the deployment method that matches your environment:

| Method | Best For | Updates |
|--------|----------|---------|
| [Raspberry Pi Install Script](#raspberry-pi-recommended) | Set-and-forget remote devices | Automatic with rollback |
| [Docker (IT-Managed)](#docker-it-managed) | Professional environments | Manual via your tooling |

---

## Raspberry Pi (Recommended)

### One-Command Installation

```bash
curl -fsSL https://raw.githubusercontent.com/alexwitherspoon/aviationwx-bridge/main/scripts/install.sh | sudo bash
```

This script:
1. Installs Docker (if not present)
2. Installs the update supervisor
3. Pulls and starts the AviationWX Bridge container
4. Configures automatic security updates
5. Sets up automatic restart on boot

### After Installation

- **Web Console**: `http://<device-ip>:1229`
- **Default Password**: `aviationwx` (change immediately!)
- **Health Check**: `http://<device-ip>:1229/healthz`

### Automatic Updates

The supervisor checks for updates every 6 hours:

| Update Type | Behavior |
|-------------|----------|
| Normal | Shows in web UI; user applies when ready |
| Critical (security) | Auto-applies after 24-hour grace period |
| Emergency | Applies immediately |

All updates automatically rollback if health checks fail.

**Manual update:**
```bash
sudo /usr/local/bin/aviationwx-supervisor update
```

**Check status:**
```bash
sudo /usr/local/bin/aviationwx-supervisor status
```

**Rollback:**
```bash
sudo /usr/local/bin/aviationwx-supervisor rollback
```

---

## Docker (IT-Managed)

For environments with existing Docker infrastructure.

### Basic Deployment

```bash
docker pull ghcr.io/alexwitherspoon/aviationwx-bridge:latest

docker run -d \
  --name aviationwx-bridge \
  --restart=unless-stopped \
  -p 1229:1229 \
  -v /opt/aviationwx/data:/data \
  --tmpfs /dev/shm:size=100m \
  ghcr.io/alexwitherspoon/aviationwx-bridge:latest
```

### Docker Compose

```yaml
services:
  aviationwx-bridge:
    image: ghcr.io/alexwitherspoon/aviationwx-bridge:latest
    container_name: aviationwx-bridge
    restart: unless-stopped
    ports:
      - "1229:1229"
    volumes:
      - ./data:/data
    tmpfs:
      - /dev/shm:size=100m
    environment:
      - LOG_LEVEL=info
```

### Version Pinning

For production stability:

```yaml
# Pin to specific version
image: ghcr.io/alexwitherspoon/aviationwx-bridge:1.0.0

# Or pin to minor version (gets patches)
image: ghcr.io/alexwitherspoon/aviationwx-bridge:1.0
```

### Updating

```bash
docker pull ghcr.io/alexwitherspoon/aviationwx-bridge:latest
docker stop aviationwx-bridge
docker rm aviationwx-bridge
docker run -d ... # same run command
```

Or use your existing update tooling (Watchtower, Portainer, ArgoCD, etc.)

---

## Configuration

### Initial Setup

1. Open web console at `http://<ip>:1229`
2. Log in with default password (`aviationwx`)
3. **Change the password immediately**
4. Set your timezone
5. Add cameras with their FTP credentials

### Get FTP Credentials

Contact [contact@aviationwx.org](mailto:contact@aviationwx.org) to obtain upload credentials for your cameras.

### Example Configuration

```json
{
  "version": 2,
  "timezone": "America/Chicago",
  "cameras": [
    {
      "id": "kord-west",
      "name": "KORD West Runway",
      "type": "http",
      "enabled": true,
      "snapshot_url": "http://192.168.1.100/snapshot.jpg",
      "capture_interval_seconds": 60,
      "upload": {
        "host": "upload.aviationwx.org",
        "username": "your-username",
        "password": "your-password",
        "tls": true
      }
    }
  ],
  "web_console": {
    "enabled": true,
    "port": 1229,
    "password": "your-secure-password"
  }
}
```

---

## Security

### Hardening Checklist

- [ ] Change default web console password
- [ ] Use HTTPS proxy if exposing to internet (Caddy, nginx, Tailscale)
- [ ] Restrict file permissions: `chmod 600 /data/aviationwx/config.json`
- [ ] Enable firewall, allow only necessary ports
- [ ] Keep system and Docker updated

### Remote Access

**Recommended: Tailscale**

```bash
# Install Tailscale on host
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up

# Access via Tailscale IP (no port forwarding needed)
http://100.x.x.x:1229
```

**Alternative: Reverse Proxy with HTTPS**

```bash
# Example with Caddy
# Caddyfile
aviationwx.yourdomain.com {
    reverse_proxy localhost:1229
}
```

---

## Monitoring

### Health Endpoints

| Endpoint | Purpose |
|----------|---------|
| `/healthz` | Container health (for Docker/K8s) |
| `/api/status` | Detailed system status (JSON) |

```bash
# Check health
curl http://localhost:1229/healthz

# Get detailed status
curl http://localhost:1229/api/status | jq
```

### Logs

```bash
# Docker logs
docker logs aviationwx-bridge -f

# Last 100 lines
docker logs aviationwx-bridge --tail=100

# Supervisor logs (Pi install only)
cat /data/aviationwx/supervisor.log
```

### Metrics (Status Response)

```json
{
  "version": "1.0.0",
  "git_commit": "abc1234",
  "running": true,
  "uptime_seconds": 86400,
  "cameras": [...],
  "upload_stats": {
    "total": 1440,
    "success": 1438,
    "failed": 2
  },
  "ntp_healthy": true,
  "update": {
    "available": false,
    "latest_version": "1.0.0"
  }
}
```

---

## Resource Limits

### Raspberry Pi Zero 2 W (512MB RAM)

The bridge is optimized for low memory:

- Queue stored in tmpfs (RAM): 100MB default
- Heap limit: 400MB
- Typical usage: 150-250MB total

```yaml
# Optional: enforce limits in Docker
deploy:
  resources:
    limits:
      memory: 300M
```

### Disk Usage

Config and logs stored in `/data` volume (~10MB typical).

Queue stored in tmpfs (`/dev/shm`) - no SD card wear.

---

## Backup & Recovery

### Backup Config

```bash
# Manual backup
cp /data/aviationwx/config.json ~/config-backup.json

# Or via Docker volume
docker run --rm \
  -v aviationwx-data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/config-backup.tar.gz -C /data .
```

### Restore Config

```bash
# Copy back
cp ~/config-backup.json /data/aviationwx/config.json

# Restart
docker restart aviationwx-bridge
```

### Complete Recovery

If device fails:

1. Flash fresh Raspberry Pi OS
2. Run install script: `curl ... | sudo bash`
3. Restore config backup
4. Cameras will resume uploading

---

## Troubleshooting

### Container Won't Start

```bash
# Check logs
docker logs aviationwx-bridge

# Validate config
python3 -m json.tool /data/aviationwx/config.json

# Check resources
free -h
df -h
```

### Camera Capture Fails

```bash
# Test camera URL from host
curl -v http://192.168.1.100/snapshot.jpg

# Check camera auth in config
# Verify network connectivity to camera
```

### Upload Fails

```bash
# Check FTP credentials
# Verify TLS is enabled (required by server)
# Check firewall allows outbound port 21

# Test connection
curl -v ftps://upload.aviationwx.org:21
```

### Memory Issues

```bash
# Check container memory
docker stats aviationwx-bridge

# If consistently high:
# - Reduce queue size in config
# - Reduce number of cameras
# - Increase capture interval
```

---

## Support

- **Documentation**: [github.com/alexwitherspoon/aviationwx-bridge](https://github.com/alexwitherspoon/aviationwx-bridge)
- **Issues**: [GitHub Issues](https://github.com/alexwitherspoon/aviationwx-bridge/issues)
- **Credentials**: [contact@aviationwx.org](mailto:contact@aviationwx.org)

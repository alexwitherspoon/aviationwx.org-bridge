# Queue Storage & Memory Management

This document explains how AviationWX.org Bridge manages image queues, memory, and storage to ensure reliable operation on resource-constrained devices like the Raspberry Pi Zero 2 W.

## Overview

The bridge uses a **RAM-based filesystem (tmpfs)** mounted at `/dev/shm` to store images waiting for upload. This design:

- **Protects SD cards** from write wear (critical for reliability)
- **Provides fast I/O** for image capture and upload
- **Limits memory usage** to prevent system instability

```
┌─────────────────────────────────────────────────────────────┐
│                    System Memory (512 MB)                   │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌──────────────────────────────────┐  │
│  │   Application   │  │         tmpfs (/dev/shm)         │  │
│  │   (~150-250 MB) │  │         (200 MB default)         │  │
│  │                 │  │  ┌────────────────────────────┐  │  │
│  │  • Go runtime   │  │  │     Image Queue            │  │  │
│  │  • HTTP server  │  │  │  ┌──────┐ ┌──────┐        │  │  │
│  │  • Schedulers   │  │  │  │cam-1 │ │cam-2 │  ...   │  │  │
│  │  • Upload       │  │  │  └──────┘ └──────┘        │  │  │
│  └─────────────────┘  │  └────────────────────────────┘  │  │
│                       └──────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Queue Health Levels

The queue system monitors capacity and takes action at different thresholds:

| Level | Capacity | Color | Action |
|-------|----------|-------|--------|
| **Healthy** | < 50% | 🟢 Green | Normal operation |
| **Catching Up** | 50-80% | 🟡 Yellow | Gradual thinning starts |
| **Degraded** | 80-95% | 🟠 Orange | Aggressive thinning |
| **Critical** | > 95% | 🔴 Red | Capture paused, emergency thin |

## Multi-Layer Defense System

The bridge has **9 layers of protection** to prevent storage exhaustion:

```
┌─────────────────────────────────────────────────────────────┐
│                    DEFENSE LAYERS                           │
├─────────────────────────────────────────────────────────────┤
│ Layer 1: Health-Based Thinning     (50%+ → thin middle)     │
│ Layer 2: Capture Pause             (95% → pause captures)   │
│ Layer 3: Application Limit         (max_total_size_mb)      │
│ Layer 4: Heap Pressure             (max_heap_mb → GC)       │
│ Layer 5: Filesystem Monitor        (<20% free → thin 50%)   │
│ Layer 6: Filesystem Critical       (<10% free → thin 70%)   │
│ Layer 7: Pre-Write Check           (check space first)      │
│ Layer 8: Write Failure Recovery    (ENOSPC → retry)         │
│ Layer 9: Final Fallback            (pause + error)          │
└─────────────────────────────────────────────────────────────┘
```

### Layer Details

| Layer | Trigger | Action |
|-------|---------|--------|
| **1. Health-Based Thinning** | Queue at 50%+ capacity | Removes images from middle, preserving oldest/newest |
| **2. Capture Pause** | Queue at 95% capacity | Stops captures until queue drops to 70% |
| **3. Application Limit** | Exceeds `max_total_size_mb` | Emergency thin all queues to 50% |
| **4. Heap Pressure** | Go heap > `max_heap_mb` | Emergency thin to 30% + force GC |
| **5. Filesystem Monitor** | tmpfs < 20% free | Thin all queues to 50% |
| **6. Filesystem Critical** | tmpfs < 10% free | Aggressive thin to 30% |
| **7. Pre-Write Check** | Before each enqueue | Check space, preemptive cleanup |
| **8. Write Failure Recovery** | ENOSPC error on write | Emergency thin + retry once |
| **9. Final Fallback** | Can't free space | Pause capture, return error |

## What Happens When Space Runs Out

The following flowchart shows the complete recovery process:

```
                    New image arrives
                           │
                           ▼
                ┌─────────────────────┐
                │ Pre-check: enough   │──No──► Emergency thin for space
                │ space for 2x image? │               │
                └─────────────────────┘               │
                       │ Yes                          │
                       ▼                              ▼
                ┌─────────────────────┐        ┌──────────────┐
                │ Attempt write       │        │ Re-check     │
                └─────────────────────┘        │ space        │
                       │                       └──────────────┘
                       │                              │
                  Success?                     Enough now?
                   │    │                        │      │
                  Yes   No (ENOSPC)            Yes     No
                   │    │                        │      │
                   ▼    ▼                        │      ▼
                 Done  ┌──────────────┐          │   ┌───────────────┐
                       │ Emergency    │──────────┘   │ Pause capture │
                       │ thin (30%)   │              │ Log error     │
                       │ Retry write  │              └───────────────┘
                       └──────────────┘
                              │
                         Success?
                          │    │
                        Yes    No
                         │     │
                         ▼     ▼
                       Done   Pause capture
                              (critical state)
```

## Queue Thinning Strategy

When the queue needs to reduce size, it uses an intelligent thinning strategy that preserves temporal coverage:

```
Before Thinning (20 images):
┌────────────────────────────────────────────────────────────┐
│ [1] [2] [3] [4] [5] [6] [7] [8] [9] [10] ... [18] [19] [20]│
│  ▲                                                      ▲  │
│  │                                                      │  │
│ Oldest                                               Newest│
│ (protected)                                      (protected)│
└────────────────────────────────────────────────────────────┘

After Thinning (12 images):
┌────────────────────────────────────────────────────────────┐
│ [1] [2] [3] [4] [5]     [10]     [15] [16] [17] [18] [19] [20]│
│  ▲   ▲   ▲   ▲   ▲                 ▲   ▲   ▲   ▲   ▲    ▲  │
│  │   │   │   │   │                 │   │   │   │   │    │  │
│  └───┴───┴───┴───┘                 └───┴───┴───┴───┴────┘  │
│     Protected Oldest                  Protected Newest     │
│                                                            │
│           Removed from middle (evenly spaced)              │
└────────────────────────────────────────────────────────────┘
```

**Why this strategy?**
- **Oldest images** are kept for historical record
- **Newest images** are kept as they're most current
- **Middle images** are expendable as they represent redundant timeframes
- Maintains temporal coverage even when heavily thinned

## Configuration

### tmpfs Size (Docker Level)

The tmpfs size is set at container startup. Default is **200 MB**.

**Sizing Recommendations:**

| Setup | Image Size (typical) | 10-min Backlog | Recommended tmpfs |
|-------|---------------------|----------------|-------------------|
| 1-2 cameras @ 1080p | ~1 MB | ~20 images | `100m` |
| 3-4 cameras @ 1080p | ~1 MB | ~40 images | `200m` (default) |
| 1-2 cameras @ 4K | ~3-5 MB | ~20 images | `200m` |
| 3-4 cameras @ 4K | ~3-5 MB | ~40 images | `300m` or higher |

**To change tmpfs size:**

```bash
# Raspberry Pi (via environment file)
sudo nano /data/aviationwx/environment
# Uncomment and set: AVIATIONWX_TMPFS_SIZE=300m
sudo /usr/local/bin/aviationwx-supervisor update

# Docker run
docker run ... --tmpfs /dev/shm:size=300m ...

# Docker Compose
tmpfs:
  - /dev/shm:size=300m
```

### Application Queue Settings

In `config.json`:

```json
{
  "queue": {
    "base_path": "/dev/shm/aviationwx",
    "max_total_size_mb": 100,
    "memory_check_seconds": 5,
    "emergency_thin_ratio": 0.5,
    "max_heap_mb": 400,
    "defaults": {
      "max_files": 100,
      "max_size_mb": 50,
      "max_age_seconds": 3600,
      "thinning_enabled": true,
      "protect_newest": 10,
      "protect_oldest": 5,
      "threshold_catching_up": 0.50,
      "threshold_degraded": 0.80,
      "threshold_critical": 0.95,
      "pause_capture_critical": true,
      "resume_threshold": 0.70
    }
  }
}
```

| Setting | Default | Description |
|---------|---------|-------------|
| `max_total_size_mb` | 100 | Max queue size across all cameras (MB) |
| `max_heap_mb` | 400 | Go heap limit before emergency thin |
| `memory_check_seconds` | 5 | How often to check memory pressure |
| `emergency_thin_ratio` | 0.5 | Keep this ratio during emergency thin |
| `max_files` | 100 | Max images per camera queue |
| `max_age_seconds` | 3600 | Expire images older than this (1 hour) |
| `protect_newest` | 10 | Always keep this many newest images |
| `protect_oldest` | 5 | Always keep this many oldest images |
| `threshold_critical` | 0.95 | Pause capture at this capacity |
| `resume_threshold` | 0.70 | Resume capture when capacity drops to this |

## System Resources Monitoring

The web UI displays real-time system resource usage with color-coded health indicators:

```
┌─────────────────────────────────────────────────────────────┐
│  System Resources                              [● Healthy]  │
├─────────────────────────────────────────────────────────────┤
│  ⚡ CPU          🧠 Memory        📦 Queue                   │
│  12%             45%              23%                        │
│  [████░░░░░░]    [████████░░]     [███░░░░░░░]               │
│  (healthy)       (healthy)        (healthy)                  │
├─────────────────────────────────────────────────────────────┤
│  CPU: 12% • Memory: 234 MB • Queue: 15 images • Uptime: 2h  │
└─────────────────────────────────────────────────────────────┘
```

### Health Level Thresholds

| Resource | Healthy (🟢) | Warning (🟡) | Critical (🔴) |
|----------|-------------|--------------|---------------|
| **CPU** | < 70% | 70-90% | > 90% |
| **Memory** | < 70% | 70-85% | > 85% |
| **Queue/Disk** | < 70% | 70-85% | > 85% |

The **overall system health** is the worst of all individual levels. If any resource is critical, the overall status shows critical.

## API Endpoints

### Status Endpoint

`GET /api/status` returns comprehensive system information:

```json
{
  "queue_storage": {
    "total_images": 25,
    "total_size_mb": 45.2,
    "memory_limit_mb": 100,
    "capacity_percent": 45.2,
    "filesystem_free_mb": 154.8,
    "filesystem_used_mb": 45.2
  },
  "system": {
    "cpu_percent": 12.5,
    "cpu_level": "healthy",
    "mem_used_mb": 234.5,
    "mem_total_mb": 512.0,
    "mem_percent": 45.8,
    "mem_level": "healthy",
    "disk_percent": 22.5,
    "disk_level": "healthy",
    "overall_level": "healthy",
    "uptime": "2h15m30s"
  }
}
```

## Troubleshooting

### Queue Shows "Critical" Frequently

**Symptoms:** Queue capacity regularly exceeds 80%, images being thinned

**Possible Causes:**
1. Upload connection is slow or unreliable
2. Too many high-resolution cameras
3. tmpfs size too small for workload

**Solutions:**
- Check network connectivity to upload server
- Reduce image resolution via `image.max_width` setting
- Increase capture interval
- Increase tmpfs size (see Configuration section)

### "No Space Left on Device" Errors

**Symptoms:** Log shows ENOSPC errors, images failing to queue

**What Happens:**
1. System detects space exhaustion
2. Emergency thin removes 70% of queued images
3. Write is retried
4. If still fails, capture is paused

**Solutions:**
- Increase tmpfs size
- Check if uploads are working
- Review camera count and resolution

### High Memory Usage

**Symptoms:** Memory indicator shows yellow/red, system feels slow

**Possible Causes:**
1. Large images in queue
2. Too many cameras configured
3. Memory leak (rare)

**Solutions:**
- Check queue storage - if high, there may be upload issues
- Reduce image quality settings
- Restart container if usage doesn't decrease after queue drains

## Design Principles

1. **Never crash** - All space errors are caught and recovered
2. **Graceful degradation** - Lose old data before losing new data
3. **Self-healing** - System recovers automatically when resources free up
4. **Visibility** - All resource usage visible in web UI
5. **Conservative defaults** - Works out-of-box on Pi Zero 2 W (512 MB RAM)

## Related Documentation

- [DEPLOYMENT.md](DEPLOYMENT.md) - Deployment and configuration guide
- [CONFIG_SCHEMA.md](CONFIG_SCHEMA.md) - Complete configuration reference


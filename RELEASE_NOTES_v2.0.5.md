# Release v2.0.5

## 🎯 Critical FTPS Fix + Performance Improvements

This release resolves persistent FTPS upload failures and adds significant performance improvements for handling upload backlogs.

---

## 🔥 Critical Fixes

### **FTPS: Default to PASV Mode**
- **Issue:** Persistent "426 Failure reading network stream" errors with EPSV
- **Root Cause:** EPSV mode incompatible with Docker networking, NAT firewalls
- **Solution:** Default to PASV mode (proven reliable with kmancam1 client)
- **Configuration:** `disable_epsv: true` by default

**Before (EPSV - fails):**
```
FTP command: "EPSV"
FTP response: "229 Entering Extended Passive Mode (|||50310|)"
FTP response: "426 Failure reading network stream." ← FAIL
```

**After (PASV - works):**
```
FTP command: "PASV"
FTP response: "227 Entering Passive Mode (178,128,130,116,196,138)"
FTP response: "226 Transfer complete." ← SUCCESS
```

**Impact:** Should eliminate most FTPS connection failures.

---

## 🚀 Performance Improvements

### **1. Concurrent Uploads (3x Faster)**
- **Default:** 3 concurrent uploads (configurable)
- **Previous:** 1 upload at a time (sequential)
- **Throughput:** ~180 images/hour (vs 60 previously)

### **2. LIFO Catch-Up Mode**
- **Trigger:** When queue > 20 images
- **Behavior:** Upload newest images first
- **Why:** Recent images more valuable during backlogs
- **Normal Mode:** FIFO (oldest first) when queue < 20

### **3. Connection Rate Limiting**
- **Protection:** Only 1 new FTP connection every 2 seconds
- **Why:** Prevents fail2ban triggers from rapid connection attempts
- **Impact:** Avoids IP bans while maintaining throughput

---

## ✨ UI Enhancements

### **1. Update Channel Badge**
Shows which update track you're on:
- 🟢 **[LATEST]** - Stable releases (default)
- 🟠 **[EDGE]** - Development builds

**Display:** `v2.0.5 [LATEST]` in header

**Configuration:** Add to `/data/aviationwx/global.json`:
```json
{
  "version": 2,
  "update_channel": "latest"  // or "edge"
}
```

### **2. Working "Uploads Today" Counter**
- **Fixed:** Dashboard counter now increments correctly
- **Resets:** Automatically at midnight (00:00 local time)
- **Tracking:** Per-camera uploads aggregated to total

---

## 📊 Technical Details

### **Upload Worker Enhancements:**

```go
type UploadWorkerConfig struct {
    MaxConcurrentUploads      int // Default: 3
    QueueCatchupThreshold     int // Default: 20 images
    ConnectionInterval        time.Duration // Default: 2s
    MinUploadInterval         time.Duration // Default: 1s
}
```

### **Daily Upload Counter:**

```go
type UploadStats struct {
    UploadsToday int64 `json:"uploads_today"` // NEW
    // Resets at midnight automatically
}
```

### **PASV Configuration:**

```json
{
  "cameras": [
    {
      "upload": {
        "disable_epsv": true  // Default: true (use PASV)
      }
    }
  ]
}
```

---

## 🔄 Upgrade Path

### **From v2.0.4:**

1. **Automatic Update:**
   ```bash
   # On Bridge host:
   sudo aviationwx force-update
   ```

2. **Or wait for automatic daily update check**

3. **No configuration changes required** - PASV is now the default

### **Verify Update:**
- Check web UI header shows `v2.0.5`
- Check update channel badge appears
- Check "Uploads Today" counter increments

---

## 🐛 Bug Fixes

- Fixed FTPS EPSV mode failures (default to PASV)
- Fixed "Uploads Today" counter always showing 0
- Fixed concurrent upload race conditions
- Fixed gofmt alignment in GlobalSettings

---

## 📈 Performance Metrics

**Upload Throughput (with 3 concurrent workers):**
- **Empty queue (normal):** ~60-180 images/hour (same as before when queue is small)
- **Backlog catch-up:** ~180 images/hour (3x faster)
- **Connection rate:** 1 connection/2s (fail2ban safe)

**Queue Behavior:**
- **< 20 images:** FIFO (oldest first, normal operation)
- **> 20 images:** LIFO (newest first, catch-up mode)

---

## 🧪 Testing

- ✅ All 13 test packages passing
- ✅ CI/CD validation successful
- ✅ PASV mode verified with production server
- ✅ Concurrent uploads stress tested
- ✅ Daily counter reset logic verified

---

## 📝 Configuration Examples

### **Disable EPSV (default in v2.0.5):**
```json
{
  "cameras": [
    {
      "upload": {
        "type": "ftps",
        "host": "ftp.example.com",
        "port": 2121,
        "disable_epsv": true
      }
    }
  ]
}
```

### **Set Update Channel:**
```json
{
  "version": 2,
  "update_channel": "latest",
  "timezone": "America/Chicago"
}
```

---

## 🔗 Related Issues

- Fixes persistent FTPS "426 Failure reading network stream" errors
- Resolves "connection refused" during rapid uploads
- Addresses fail2ban triggers from concurrent connections
- Fixes dashboard "Uploads Today" counter

---

## 👥 Credits

Thanks to the production deployment for identifying the EPSV/PASV compatibility issue!

---

## 📚 Documentation

- [DEFENSIVE_ARCHITECTURE_V2.md](docs/DEFENSIVE_ARCHITECTURE_V2.md) - System resilience
- [RESOURCE_LIMITS.md](docs/RESOURCE_LIMITS.md) - Dynamic resource management
- [CONFIG_SCHEMA.md](docs/CONFIG_SCHEMA.md) - Configuration reference
- [LOCAL_TESTING.md](docs/LOCAL_TESTING.md) - Testing guide

---

## 🚦 Production Readiness

- ✅ All tests passing
- ✅ PASV mode proven in production (kmancam1)
- ✅ Connection rate limiting protects against fail2ban
- ✅ Concurrent uploads tested under load
- ✅ Daily counter logic validated
- ✅ UI enhancements verified

**Recommended for immediate deployment.**

---

## 🔮 Coming in v2.1.0

- Configurable concurrent upload limits per camera
- Upload queue prioritization strategies
- Enhanced FTP connection pooling
- Detailed per-camera upload statistics

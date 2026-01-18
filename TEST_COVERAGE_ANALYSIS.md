# Test Coverage Analysis - AviationWX Bridge

## Current Coverage: 57.4%

### By Package (Sorted by Priority)

| Package | Coverage | Priority | Status |
|---------|----------|----------|--------|
| **cmd/bridge** | 1.9% | 🔴 CRITICAL | Needs E2E tests |
| **internal/time** | 84.6% | 🟢 GOOD | Add NTP failure tests |
| **internal/queue** | 74.2% | 🟡 OK | Add disk full tests |
| **internal/scheduler** | 67.0% | 🟡 OK | Add failure scenarios |
| **internal/config** | 54.3% | 🟡 OK | Add corruption tests |
| **internal/upload** | 31.1% | 🔴 LOW | Add retry/failure tests |
| **internal/camera** | 32.9% | 🟡 OK | Mostly integration tests |
| **internal/web** | 39.8% | 🟡 OK | Add error path tests |

---

## Safety-Critical Code Paths

### 1. 🚨 EXIF Timestamp Stamping (HIGH PRIORITY)
**Why Critical**: Wrong timestamps = wrong weather observations = safety risk

**Current Coverage**: 84.6% (internal/time)

**What's Tested** ✅:
- `authority_test.go`: UTC authority logic
- `exif_test.go`: EXIF reading/parsing  
- `exiftool_test.go`: exiftool integration

**Missing Tests** ❌:
- [ ] **CRITICAL**: UTC enforcement - verify all timestamps are UTC
- [ ] **CRITICAL**: Marker validation - "AviationWX-Bridge:UTC:v1:..." format
- [ ] NTP failure scenario - what happens when bridge time is wrong?
- [ ] Camera clock drift - reject if > 5 minutes drift
- [ ] Timezone conversion errors - malformed timezone handling

**Risk**: MEDIUM (good coverage, but edge cases untested)

---

### 2. 🚨 Queue Management (HIGH PRIORITY)
**Why Critical**: Queue overflow = missed observations

**Current Coverage**: 74.2% (internal/queue)

**What's Tested** ✅:
- `queue_test.go`: Basic enqueue/dequeue
- Emergency thinning (line 200)
- Image validation

**Missing Tests** ❌:
- [ ] **CRITICAL**: Disk full scenario (tmpfs exhausted)
- [ ] **CRITICAL**: Concurrent access (multiple workers)
- [ ] Corruption recovery (interrupted writes)
- [ ] Capture pause/resume flow
- [ ] Queue persistence across restarts

**Risk**: MEDIUM (core logic tested, but failure modes untested)

---

### 3. 🚨 Upload Reliability (HIGH PRIORITY)
**Why Critical**: Failed uploads = lost historical data

**Current Coverage**: 31.1% (internal/upload) ⚠️ **LOW**

**What's Tested** ✅:
- `ftps_test.go`: Basic config validation
- `factory_test.go`: Client creation

**Missing Tests** ❌:
- [ ] **CRITICAL**: Network timeout during upload
- [ ] **CRITICAL**: Auth failure + retry logic
- [ ] **CRITICAL**: Partial upload (connection drop mid-transfer)
- [ ] Per-camera credential isolation
- [ ] TLS certificate validation
- [ ] Fail2ban backoff (60s after auth failure)

**Risk**: HIGH ⚠️ (low coverage, critical for data integrity)

---

### 4. 🚨 Configuration Integrity (MEDIUM PRIORITY)
**Why Critical**: Config corruption = system inoperable

**Current Coverage**: 54.3% (internal/config)

**What's Tested** ✅:
- `service_test.go`: CRUD operations
- Event-driven updates
- Migration from legacy config

**Missing Tests** ❌:
- [ ] **IMPORTANT**: Corrupted config file recovery
- [ ] **IMPORTANT**: Backup restoration
- [ ] Concurrent config updates (race conditions)
- [ ] Invalid JSON handling
- [ ] File permission errors

**Risk**: MEDIUM (backup exists, but recovery untested)

---

### 5. 🚨 Main Application Flow (CRITICAL PRIORITY)
**Why Critical**: Integration testing ensures all components work together

**Current Coverage**: 1.9% (cmd/bridge) ⚠️ **VERY LOW**

**What's Tested** ✅:
- `integration_test.go`: Config service integration only

**Missing Tests** ❌:
- [ ] **CRITICAL**: Full E2E flow: capture → process → EXIF → queue → upload
- [ ] **CRITICAL**: Hot-reload: timezone/SNTP/camera changes
- [ ] **CRITICAL**: Graceful shutdown (signal handling)
- [ ] **CRITICAL**: Panic recovery
- [ ] Orchestrator initialization
- [ ] Worker lifecycle management

**Risk**: VERY HIGH ⚠️ (no integration tests for core flow)

---

## Recommended Test Plan

### Phase 1: Safety-Critical E2E Tests (IMMEDIATE)

#### Test 1: Full Capture → Upload Flow
```go
// Mocked E2E test
func TestE2E_CaptureToUpload(t *testing.T) {
    // Setup: Mock camera, mock FTP server, real queue
    // Execute: Trigger capture
    // Verify:
    // 1. Image captured
    // 2. EXIF stamped with UTC time
    // 3. Marker: "AviationWX-Bridge:UTC:v1:bridge_clock:high"
    // 4. Queued successfully
    // 5. Uploaded with correct filename
    // 6. Queue emptied
}
```

#### Test 2: EXIF UTC Enforcement
```go
func TestEXIF_UTCEnforcement(t *testing.T) {
    // Verify ALL timestamps are UTC, never local
    // Test with various timezones
    // Test with DST transitions
}
```

#### Test 3: Upload Failure Recovery
```go
func TestUpload_NetworkFailureRecovery(t *testing.T) {
    // Simulate: Connection drop mid-upload
    // Verify: Image requeued, retry after backoff
    // Simulate: Auth failure
    // Verify: 60s backoff, no fail2ban trigger
}
```

---

### Phase 2: Failure Scenarios (HIGH PRIORITY)

#### Test 4: Queue Disk Full
```go
func TestQueue_DiskFullRecovery(t *testing.T) {
    // Simulate: tmpfs full
    // Verify: Emergency thinning, capture pause
    // Simulate: Space freed
    // Verify: Capture resumes
}
```

#### Test 5: Config Corruption Recovery
```go
func TestConfig_CorruptionRecovery(t *testing.T) {
    // Corrupt config file (invalid JSON)
    // Verify: Loads from .bak file
    // Verify: Web UI remains accessible
}
```

#### Test 6: Time Authority Failure
```go
func TestTimeAuthority_NTPFailure(t *testing.T) {
    // Simulate: All NTP servers unreachable
    // Verify: Warning in EXIF marker
    // Verify: Uses bridge clock + warning
    // Verify: Dashboard shows NTP unhealthy
}
```

---

### Phase 3: Concurrency & Race Conditions (MEDIUM PRIORITY)

#### Test 7: Concurrent Config Updates
```go
func TestConfig_ConcurrentUpdates(t *testing.T) {
    // 10 goroutines updating different cameras
    // Verify: No race conditions (use -race flag)
    // Verify: All updates applied correctly
}
```

#### Test 8: Hot-Reload Under Load
```go
func TestHotReload_UnderLoad(t *testing.T) {
    // Workers actively capturing/uploading
    // Update: Timezone, SNTP servers, camera interval
    // Verify: Workers reload without data loss
    // Verify: No dropped captures
}
```

---

## Test Infrastructure Needs

### Mocks Required
1. **MockCamera** ✅ (exists in scheduler tests)
2. **MockFTPServer** ❌ (NEEDED for upload tests)
3. **MockNTPServer** ❌ (NEEDED for time tests)
4. **MockFilesystem** ❌ (NEEDED for disk full tests)

### Test Helpers Needed
1. **CreateTestImage()** - Generate valid JPEG with/without EXIF
2. **VerifyEXIFTimestamp()** - Parse and validate EXIF format
3. **VerifyUTCMarker()** - Check "AviationWX-Bridge:UTC:v1" marker
4. **SimulateNetworkFailure()** - Inject connection errors
5. **FillDisk()** - Simulate tmpfs full condition

---

## Test Execution Strategy

### Local Development
```bash
# Run all tests
go test ./...

# Run with race detector
go test -race ./...

# Run with coverage
go test -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out
```

### CI Pipeline
```yaml
# Already configured in .github/workflows/ci.yml
- Run tests on every commit
- Fail if coverage drops below 60%
- Run race detector
```

### Pre-Release Checklist
- [ ] All tests pass
- [ ] Coverage > 70%
- [ ] No race conditions detected
- [ ] All safety-critical paths tested
- [ ] E2E tests pass
- [ ] Manual testing on Pi

---

## Coverage Goals

| Phase | Target | Packages |
|-------|--------|----------|
| **Current** | 57.4% | All |
| **Phase 1** | 70% | cmd/bridge, internal/upload |
| **Phase 2** | 75% | internal/time, internal/queue |
| **Phase 3** | 80% | All safety-critical paths |

---

## Priority Matrix

```
HIGH PRIORITY (Do First):
├── E2E test: capture → upload
├── EXIF UTC enforcement
├── Upload failure recovery
└── Queue disk full handling

MEDIUM PRIORITY (Do Next):
├── Config corruption recovery
├── Time authority NTP failure
├── Concurrent config updates
└── Hot-reload under load

LOW PRIORITY (Nice to Have):
├── Camera integration tests
├── Web UI error paths
└── Update checker edge cases
```

---

## Action Items

### Immediate (v2.0.2)
1. ✅ Create this analysis document
2. ⏳ Add E2E test for capture → upload
3. ⏳ Add EXIF UTC enforcement tests
4. ⏳ Add upload retry tests
5. ⏳ Target: 70% coverage

### Near-term (v2.1)
6. Add queue failure tests
7. Add config corruption tests
8. Add concurrency tests
9. Target: 75% coverage

### Long-term
10. Achieve 80% coverage on all safety-critical paths
11. Add property-based testing for EXIF
12. Add fuzzing for config parsing

---

## Notes

- **Aviation Safety Context**: Incorrect timestamps could lead to:
  - Wrong weather observations in historical data
  - Incorrect time-series analysis
  - Regulatory compliance issues
  
- **Current Risk Assessment**: MEDIUM
  - Core logic is tested
  - Integration tests are missing
  - Failure scenarios untested
  
- **Recommendation**: Prioritize E2E and upload tests before v2.0.2 release

---

Last Updated: 2026-01-18
Coverage Target: 70% → 75% → 80%

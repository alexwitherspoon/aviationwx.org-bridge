package scheduler

import (
	"testing"
)

func TestDefaultDegradedConfig(t *testing.T) {
	config := DefaultDegradedConfig()

	if !config.Enabled {
		t.Error("Enabled should default to true")
	}
	if config.FailureThreshold != 3 {
		t.Errorf("FailureThreshold = %d, want 3", config.FailureThreshold)
	}
	if config.ConcurrencyLimit != 1 {
		t.Errorf("ConcurrencyLimit = %d, want 1", config.ConcurrencyLimit)
	}
	if config.SlowIntervalMultiplier != 2.0 {
		t.Errorf("SlowIntervalMultiplier = %f, want 2.0", config.SlowIntervalMultiplier)
	}
}

func TestNewDegradedMode(t *testing.T) {
	config := DefaultDegradedConfig()
	dm := NewDegradedMode(config)

	if dm == nil {
		t.Fatal("NewDegradedMode() returned nil")
	}
	if !dm.enabled {
		t.Error("enabled should be true")
	}
	if dm.active {
		t.Error("active should start as false")
	}
	if dm.failureCount != 0 {
		t.Errorf("failureCount = %d, want 0", dm.failureCount)
	}
}

func TestDegradedMode_RecordFailure(t *testing.T) {
	config := DefaultDegradedConfig()
	dm := NewDegradedMode(config)

	// Record failures up to threshold
	for i := 0; i < config.FailureThreshold-1; i++ {
		dm.RecordFailure()
		if dm.IsActive() {
			t.Errorf("Should not be active after %d failures", i+1)
		}
	}

	// Record threshold failure - should activate
	dm.RecordFailure()
	if !dm.IsActive() {
		t.Error("Should be active after threshold failures")
	}

	// Record more failures - should stay active
	dm.RecordFailure()
	if !dm.IsActive() {
		t.Error("Should stay active after more failures")
	}
}

func TestDegradedMode_RecordSuccess(t *testing.T) {
	config := DefaultDegradedConfig()
	dm := NewDegradedMode(config)

	// Activate degraded mode
	for i := 0; i < config.FailureThreshold; i++ {
		dm.RecordFailure()
	}
	if !dm.IsActive() {
		t.Fatal("Should be active after threshold failures")
	}

	// Record success - should deactivate
	dm.RecordSuccess()
	if dm.IsActive() {
		t.Error("Should not be active after success")
	}
	if dm.GetFailureCount() != 0 {
		t.Errorf("FailureCount = %d, want 0", dm.GetFailureCount())
	}
}

func TestDegradedMode_GetConcurrencyLimit(t *testing.T) {
	config := DefaultDegradedConfig()
	dm := NewDegradedMode(config)

	// Not active - should return -1 (no limit)
	limit := dm.GetConcurrencyLimit()
	if limit != -1 {
		t.Errorf("ConcurrencyLimit = %d, want -1 (no limit)", limit)
	}

	// Activate degraded mode
	for i := 0; i < config.FailureThreshold; i++ {
		dm.RecordFailure()
	}

	// Active - should return configured limit
	limit = dm.GetConcurrencyLimit()
	if limit != config.ConcurrencyLimit {
		t.Errorf("ConcurrencyLimit = %d, want %d", limit, config.ConcurrencyLimit)
	}
}

func TestDegradedMode_GetIntervalMultiplier(t *testing.T) {
	config := DefaultDegradedConfig()
	dm := NewDegradedMode(config)

	// Not active - should return 1.0
	multiplier := dm.GetIntervalMultiplier()
	if multiplier != 1.0 {
		t.Errorf("IntervalMultiplier = %f, want 1.0", multiplier)
	}

	// Activate degraded mode
	for i := 0; i < config.FailureThreshold; i++ {
		dm.RecordFailure()
	}

	// Active - should return configured multiplier
	multiplier = dm.GetIntervalMultiplier()
	if multiplier != config.SlowIntervalMultiplier {
		t.Errorf("IntervalMultiplier = %f, want %f", multiplier, config.SlowIntervalMultiplier)
	}
}

func TestDegradedMode_Disabled(t *testing.T) {
	config := DegradedConfig{
		Enabled: false,
	}
	dm := NewDegradedMode(config)

	// Record failures - should not activate
	for i := 0; i < 10; i++ {
		dm.RecordFailure()
	}

	if dm.IsActive() {
		t.Error("Should not be active when disabled")
	}
	if dm.GetFailureCount() != 0 {
		t.Errorf("FailureCount = %d, want 0 when disabled", dm.GetFailureCount())
	}
}

func TestDegradedMode_ConcurrentAccess(t *testing.T) {
	config := DefaultDegradedConfig()
	dm := NewDegradedMode(config)

	// Test concurrent access
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			dm.RecordFailure()
			dm.IsActive()
			dm.GetFailureCount()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic and should have recorded failures
	count := dm.GetFailureCount()
	if count == 0 {
		t.Error("Should have recorded some failures")
	}
}

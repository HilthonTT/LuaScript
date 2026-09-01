package gctune

import "testing"

func restoreDefault(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { SetPercent(DefaultPercent) })
}

func TestSetPercentReturnsPrevious(t *testing.T) {
	restoreDefault(t)
	SetPercent(100)
	if prev := SetPercent(200); prev != 100 {
		t.Errorf("SetPercent(200) returned previous %d, want 100", prev)
	}
	if !IsRunning() {
		t.Error("GC should be running after a positive SetPercent")
	}
}

func TestStopRestartRoundTrips(t *testing.T) {
	restoreDefault(t)
	SetPercent(150)
	Stop()
	if IsRunning() {
		t.Error("IsRunning should be false after Stop")
	}
	Restart()
	if !IsRunning() {
		t.Error("IsRunning should be true after Restart")
	}
	if prev := SetPercent(150); prev != 150 {
		t.Errorf("Restart did not restore remembered percent: got previous %d, want 150", prev)
	}
}

func TestApplyLeavesZeroFieldsUntouched(t *testing.T) {
	restoreDefault(t)
	SetPercent(123)
	Apply(Options{})
	if prev := SetPercent(123); prev != 123 {
		t.Errorf("Apply with zero Options changed GOGC: previous %d, want 123", prev)
	}
}

func TestHeapBytesNonZero(t *testing.T) {
	if HeapBytes() == 0 {
		t.Error("HeapBytes returned 0, expected a non-zero live heap")
	}
}

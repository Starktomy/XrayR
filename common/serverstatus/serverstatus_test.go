package serverstatus

import (
	"strings"
	"testing"
)

// GetSystemInfo shells out to gopsutil. On a machine where
// the gopsutil collectors work (any Linux host in the test
// runner) the calls should succeed and return sensible
// non-negative numbers. The function is small, so the test
// only asserts the contract rather than exact values.
func TestGetSystemInfoSuccess(t *testing.T) {
	cpu, mem, disk, uptime, err := GetSystemInfo()
	if err != nil {
		// Some sandboxes (very minimal containers) may fail
		// the disk probe; treat that as a soft skip rather
		// than a hard failure because the function still
		// returns a structured error.
		if !strings.Contains(err.Error(), "get disk usage") {
			t.Fatalf("unexpected error: %s", err)
		}
		t.Skipf("disk collector unavailable: %s", err)
	}
	if cpu < 0 || cpu > 100 {
		t.Errorf("cpu = %f, want [0,100]", cpu)
	}
	if mem < 0 || mem > 100 {
		t.Errorf("mem = %f, want [0,100]", mem)
	}
	if disk < 0 || disk > 100 {
		t.Errorf("disk = %f, want [0,100]", disk)
	}
	// Uptime is a uint64 count of seconds. It can be 0 only
	// in containers that have been freshly started.
	_ = uptime
}

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"testing"
	"time"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// TestNewDebounceState verifies that NewDebounceState creates properly initialized state.
func TestNewDebounceState(t *testing.T) {
	ds := NewDebounceState()

	if ds == nil {
		t.Fatal("NewDebounceState() returned nil")
	}

	if ds.lastChangeTime == nil {
		t.Error("lastChangeTime map is nil")
	}

	if ds.lastHash == nil {
		t.Error("lastHash map is nil")
	}

	if ds.poolSpecHash == nil {
		t.Error("poolSpecHash map is nil")
	}
}

// TestCheckAndUpdate_FirstCall verifies behavior on first call for a pool.
func TestCheckAndUpdate_FirstCall(t *testing.T) {
	ds := NewDebounceState()

	shouldProceed, requeueAfter := ds.CheckAndUpdate("worker", "abc123", "spec1", 30)

	if shouldProceed {
		t.Error("First call should not proceed (need to wait for debounce)")
	}

	if requeueAfter != 30*time.Second {
		t.Errorf("requeueAfter = %v, want %v", requeueAfter, 30*time.Second)
	}

	// Verify state was stored
	if ds.GetLastHash("worker") != "abc123" {
		t.Errorf("Hash not stored correctly")
	}

	if ds.GetPoolSpecHash("worker") != "spec1" {
		t.Errorf("Pool spec hash not stored correctly")
	}
}

// TestCheckAndUpdate_HashChanged verifies timer reset when config hash changes.
func TestCheckAndUpdate_HashChanged(t *testing.T) {
	ds := NewDebounceState()

	ds.CheckAndUpdate("worker", "hash1", "spec1", 30)
	firstChangeTime := ds.GetLastChangeTime("worker")

	time.Sleep(10 * time.Millisecond)

	shouldProceed, requeueAfter := ds.CheckAndUpdate("worker", "hash2", "spec1", 30)

	if shouldProceed {
		t.Error("Hash change should reset timer, not proceed")
	}

	if requeueAfter != 30*time.Second {
		t.Errorf("requeueAfter = %v, want full debounce %v", requeueAfter, 30*time.Second)
	}

	secondChangeTime := ds.GetLastChangeTime("worker")
	if !secondChangeTime.After(firstChangeTime) {
		t.Error("Timer should be reset on hash change")
	}

	if ds.GetLastHash("worker") != "hash2" {
		t.Errorf("Hash not updated, got %s", ds.GetLastHash("worker"))
	}
}

// TestCheckAndUpdate_SameHashDebouncing verifies partial debounce returns remaining time.
func TestCheckAndUpdate_SameHashDebouncing(t *testing.T) {
	ds := NewDebounceState()

	ds.CheckAndUpdate("worker", "hash1", "spec1", 1) // 1 second debounce for faster test

	time.Sleep(200 * time.Millisecond)

	shouldProceed, requeueAfter := ds.CheckAndUpdate("worker", "hash1", "spec1", 1)

	if shouldProceed {
		t.Error("Should not proceed during debounce window")
	}

	if requeueAfter >= 1*time.Second {
		t.Errorf("requeueAfter = %v, should be less than 1s", requeueAfter)
	}

	if requeueAfter < 500*time.Millisecond {
		t.Errorf("requeueAfter = %v, should be around 800ms", requeueAfter)
	}
}

// TestCheckAndUpdate_DebounceElapsed verifies proceed when debounce elapsed.
func TestCheckAndUpdate_DebounceElapsed(t *testing.T) {
	ds := NewDebounceState()

	ds.CheckAndUpdate("worker", "hash1", "spec1", 0) // 0 second debounce

	shouldProceed, requeueAfter := ds.CheckAndUpdate("worker", "hash1", "spec1", 0)

	if !shouldProceed {
		t.Error("Should proceed after debounce elapsed")
	}

	if requeueAfter != 0 {
		t.Errorf("requeueAfter = %v, want 0", requeueAfter)
	}
}

// TestCheckAndUpdate_MultiplePools verifies independent state per pool.
func TestCheckAndUpdate_MultiplePools(t *testing.T) {
	ds := NewDebounceState()

	ds.CheckAndUpdate("worker", "hash-worker", "spec-worker", 30)
	ds.CheckAndUpdate("master", "hash-master", "spec-master", 60)

	if ds.GetLastHash("worker") != "hash-worker" {
		t.Errorf("worker hash = %s, want hash-worker", ds.GetLastHash("worker"))
	}

	if ds.GetLastHash("master") != "hash-master" {
		t.Errorf("master hash = %s, want hash-master", ds.GetLastHash("master"))
	}

	ds.CheckAndUpdate("worker", "hash-worker-new", "spec-worker", 30)

	if ds.GetLastHash("master") != "hash-master" {
		t.Error("Changing worker pool affected master pool")
	}
}

// TestReset verifies that Reset clears state for a pool.
func TestReset(t *testing.T) {
	ds := NewDebounceState()

	// Setup state
	ds.CheckAndUpdate("worker", "hash1", "spec1", 30)
	ds.CheckAndUpdate("master", "hash2", "spec2", 30)

	ds.Reset("worker")

	if ds.GetLastHash("worker") != "" {
		t.Errorf("worker hash should be empty after reset, got %s", ds.GetLastHash("worker"))
	}

	if ds.GetPoolSpecHash("worker") != "" {
		t.Errorf("worker pool spec hash should be empty after reset, got %s", ds.GetPoolSpecHash("worker"))
	}

	if !ds.GetLastChangeTime("worker").IsZero() {
		t.Error("worker time should be zero after reset")
	}

	if ds.GetLastHash("master") != "hash2" {
		t.Error("Reset of worker affected master")
	}
}

// TestReset_NonExistent verifies Reset handles non-existent pool gracefully.
func TestReset_NonExistent(t *testing.T) {
	ds := NewDebounceState()

	ds.Reset("nonexistent")

	if ds.GetLastHash("nonexistent") != "" {
		t.Error("Non-existent pool should have empty hash")
	}
}

// TestGetLastHash_NonExistent verifies GetLastHash returns empty for unknown pool.
func TestGetLastHash_NonExistent(t *testing.T) {
	ds := NewDebounceState()

	hash := ds.GetLastHash("unknown")

	if hash != "" {
		t.Errorf("GetLastHash for unknown pool = %s, want empty", hash)
	}
}

// TestGetLastChangeTime_NonExistent verifies GetLastChangeTime returns zero for unknown pool.
func TestGetLastChangeTime_NonExistent(t *testing.T) {
	ds := NewDebounceState()

	changeTime := ds.GetLastChangeTime("unknown")

	if !changeTime.IsZero() {
		t.Errorf("GetLastChangeTime for unknown pool = %v, want zero", changeTime)
	}
}

// TestCheckAndUpdate_Concurrent verifies thread safety.
func TestCheckAndUpdate_Concurrent(t *testing.T) {
	ds := NewDebounceState()

	// Run concurrent operations
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(n int) {
			pool := "pool"
			for j := 0; j < 100; j++ {
				ds.CheckAndUpdate(pool, "hash", "spec", 30)
				ds.GetLastHash(pool)
				ds.GetPoolSpecHash(pool)
				ds.GetLastChangeTime(pool)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestCheckAndUpdate_ZeroDebounce verifies behavior with zero debounce time.
func TestCheckAndUpdate_ZeroDebounce(t *testing.T) {
	ds := NewDebounceState()

	shouldProceed, requeueAfter := ds.CheckAndUpdate("worker", "hash1", "spec1", 0)

	if shouldProceed {
		t.Error("First call should still return false to record state")
	}

	if requeueAfter != 0 {
		t.Errorf("requeueAfter = %v, want 0 for zero debounce", requeueAfter)
	}

	shouldProceed, requeueAfter = ds.CheckAndUpdate("worker", "hash1", "spec1", 0)

	if !shouldProceed {
		t.Error("Second call with zero debounce should proceed")
	}
}

// TestCheckAndUpdate_ZeroTime verifies handling of zero lastChangeTime.
// This tests the edge case where hash exists but time is zero (shouldn't happen normally).
func TestCheckAndUpdate_ZeroTime(t *testing.T) {
	ds := NewDebounceState()

	ds.lastHash["worker"] = "hash1"
	ds.poolSpecHash["worker"] = "spec1"

	shouldProceed, requeueAfter := ds.CheckAndUpdate("worker", "hash1", "spec1", 30)

	if shouldProceed {
		t.Error("Should not proceed with zero time (treats as new)")
	}

	if requeueAfter != 30*time.Second {
		t.Errorf("requeueAfter = %v, want %v", requeueAfter, 30*time.Second)
	}

	if ds.GetLastChangeTime("worker").IsZero() {
		t.Error("Time should be set after call")
	}
}

// TestCheckAndUpdate_PoolSpecHashChanged verifies timer reset when pool spec hash changes.
func TestCheckAndUpdate_PoolSpecHashChanged(t *testing.T) {
	ds := NewDebounceState()

	ds.CheckAndUpdate("worker", "hash1", "spec1", 30)
	firstChangeTime := ds.GetLastChangeTime("worker")

	time.Sleep(10 * time.Millisecond)

	shouldProceed, requeueAfter := ds.CheckAndUpdate("worker", "hash1", "spec2", 30)

	if shouldProceed {
		t.Error("Pool spec change should reset timer, not proceed")
	}

	if requeueAfter != 30*time.Second {
		t.Errorf("requeueAfter = %v, want full debounce %v", requeueAfter, 30*time.Second)
	}

	secondChangeTime := ds.GetLastChangeTime("worker")
	if !secondChangeTime.After(firstChangeTime) {
		t.Error("Timer should be reset on pool spec change")
	}

	if ds.GetPoolSpecHash("worker") != "spec2" {
		t.Errorf("Pool spec hash not updated, got %s", ds.GetPoolSpecHash("worker"))
	}
}

// TestCheckAndUpdate_BothHashesChanged verifies behavior when both hashes change.
func TestCheckAndUpdate_BothHashesChanged(t *testing.T) {
	ds := NewDebounceState()

	ds.CheckAndUpdate("worker", "hash1", "spec1", 30)

	time.Sleep(10 * time.Millisecond)

	shouldProceed, requeueAfter := ds.CheckAndUpdate("worker", "hash2", "spec2", 30)

	if shouldProceed {
		t.Error("Both hashes changed should reset timer, not proceed")
	}

	if requeueAfter != 30*time.Second {
		t.Errorf("requeueAfter = %v, want full debounce %v", requeueAfter, 30*time.Second)
	}

	if ds.GetLastHash("worker") != "hash2" {
		t.Errorf("Config hash not updated, got %s", ds.GetLastHash("worker"))
	}
	if ds.GetPoolSpecHash("worker") != "spec2" {
		t.Errorf("Pool spec hash not updated, got %s", ds.GetPoolSpecHash("worker"))
	}
}

// TestGetPoolSpecHash_NonExistent verifies GetPoolSpecHash returns empty for unknown pool.
func TestGetPoolSpecHash_NonExistent(t *testing.T) {
	ds := NewDebounceState()

	hash := ds.GetPoolSpecHash("unknown")

	if hash != "" {
		t.Errorf("GetPoolSpecHash for unknown pool = %s, want empty", hash)
	}
}

// TestComputePoolSpecHash_Deterministic verifies hash is deterministic.
func TestComputePoolSpecHash_Deterministic(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Reboot: mcov1alpha1.RebootPolicy{
				Strategy:           "Never",
				MinIntervalSeconds: 1800,
			},
		},
	}

	hash1 := ComputePoolSpecHash(pool)
	hash2 := ComputePoolSpecHash(pool)

	if hash1 != hash2 {
		t.Errorf("Hash not deterministic: %s != %s", hash1, hash2)
	}

	if len(hash1) != 16 {
		t.Errorf("Hash length = %d, want 16", len(hash1))
	}
}

// TestComputePoolSpecHash_StrategyChange verifies hash changes when strategy changes.
func TestComputePoolSpecHash_StrategyChange(t *testing.T) {
	pool1 := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Reboot: mcov1alpha1.RebootPolicy{
				Strategy:           "Never",
				MinIntervalSeconds: 1800,
			},
		},
	}

	pool2 := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Reboot: mcov1alpha1.RebootPolicy{
				Strategy:           "IfRequired",
				MinIntervalSeconds: 1800,
			},
		},
	}

	hash1 := ComputePoolSpecHash(pool1)
	hash2 := ComputePoolSpecHash(pool2)

	if hash1 == hash2 {
		t.Error("Hash should change when strategy changes")
	}
}

// TestComputePoolSpecHash_MinIntervalChange verifies hash changes when minInterval changes.
func TestComputePoolSpecHash_MinIntervalChange(t *testing.T) {
	pool1 := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Reboot: mcov1alpha1.RebootPolicy{
				Strategy:           "Never",
				MinIntervalSeconds: 1800,
			},
		},
	}

	pool2 := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Reboot: mcov1alpha1.RebootPolicy{
				Strategy:           "Never",
				MinIntervalSeconds: 0,
			},
		},
	}

	hash1 := ComputePoolSpecHash(pool1)
	hash2 := ComputePoolSpecHash(pool2)

	if hash1 == hash2 {
		t.Error("Hash should change when minIntervalSeconds changes")
	}
}

// TestComputePoolSpecHash_Unchanged verifies hash stable when unchanged.
func TestComputePoolSpecHash_Unchanged(t *testing.T) {
	pool1 := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Reboot: mcov1alpha1.RebootPolicy{
				Strategy:           "IfRequired",
				MinIntervalSeconds: 3600,
			},
		},
	}

	// Create identical pool
	pool2 := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Reboot: mcov1alpha1.RebootPolicy{
				Strategy:           "IfRequired",
				MinIntervalSeconds: 3600,
			},
		},
	}

	hash1 := ComputePoolSpecHash(pool1)
	hash2 := ComputePoolSpecHash(pool2)

	if hash1 != hash2 {
		t.Errorf("Hash should be same for identical pools: %s != %s", hash1, hash2)
	}
}

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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// DebounceState tracks debounce timing per pool.
// This prevents rapid re-renders when multiple MachineConfigs change in sequence.
type DebounceState struct {
	mu             sync.RWMutex
	lastChangeTime map[string]time.Time
	lastHash       map[string]string
	poolSpecHash   map[string]string
}

// NewDebounceState creates a new debounce state tracker.
func NewDebounceState() *DebounceState {
	return &DebounceState{
		lastChangeTime: make(map[string]time.Time),
		lastHash:       make(map[string]string),
		poolSpecHash:   make(map[string]string),
	}
}

// CheckAndUpdate checks if we should proceed with rendering and updates state.
// Returns (shouldProceed, requeueAfter).
func (d *DebounceState) CheckAndUpdate(pool string, configHash string, poolSpecHash string, debounceSeconds int) (bool, time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	debounce := time.Duration(debounceSeconds) * time.Second

	configChanged := d.lastHash[pool] != configHash
	specChanged := d.poolSpecHash[pool] != poolSpecHash

	if configChanged || specChanged {
		d.lastChangeTime[pool] = now
		d.lastHash[pool] = configHash
		d.poolSpecHash[pool] = poolSpecHash
		return false, debounce
	}

	lastChange := d.lastChangeTime[pool]
	if lastChange.IsZero() {
		d.lastChangeTime[pool] = now
		return false, debounce
	}

	elapsed := now.Sub(lastChange)
	if elapsed < debounce {
		remaining := debounce - elapsed
		return false, remaining
	}

	return true, 0
}

// Reset clears state for a pool.
func (d *DebounceState) Reset(pool string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.lastChangeTime, pool)
	delete(d.lastHash, pool)
	delete(d.poolSpecHash, pool)
}

// GetLastHash returns the last known hash for a pool.
func (d *DebounceState) GetLastHash(pool string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastHash[pool]
}

// GetLastChangeTime returns the last change time for a pool.
func (d *DebounceState) GetLastChangeTime(pool string) time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastChangeTime[pool]
}

// GetPoolSpecHash returns the last known pool spec hash for a pool.
func (d *DebounceState) GetPoolSpecHash(pool string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.poolSpecHash[pool]
}

type poolSpecData struct {
	Strategy           string `json:"strategy"`
	MinIntervalSeconds int    `json:"minIntervalSeconds"`
}

// ComputePoolSpecHash computes a hash of pool spec fields that affect RMC content.
// This allows detecting pool policy changes (strategy, minIntervalSeconds) separately
// from config content changes. Returns a short hex string (16 chars).
func ComputePoolSpecHash(pool *mcov1alpha1.MachineConfigPool) string {
	data := poolSpecData{
		Strategy:           pool.Spec.Reboot.Strategy,
		MinIntervalSeconds: pool.Spec.Reboot.MinIntervalSeconds,
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		// Should never happen with this simple struct
		return ""
	}

	hash := sha256.Sum256(bytes)
	return hex.EncodeToString(hash[:8])
}

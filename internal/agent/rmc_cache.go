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

package agent

import (
	"sync"
	"time"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// Default cache settings.
const (
	// DefaultRMCCacheTTL is the default time-to-live for cached RMCs.
	// RMCs don't change after creation, but we still expire them to handle
	// rare cases like garbage collection and recreation.
	DefaultRMCCacheTTL = 5 * time.Minute
)

// cachedRMC holds an RMC with its fetch timestamp.
type cachedRMC struct {
	rmc       *mcov1alpha1.RenderedMachineConfig
	fetchedAt time.Time
}

// RMCCache provides a simple TTL-based cache for RenderedMachineConfigs.
// It is safe for concurrent access.
//
// The cache helps reduce API calls when the agent needs to fetch the same
// RMC multiple times during diff-based reboot determination.
type RMCCache struct {
	mu    sync.RWMutex
	items map[string]*cachedRMC
	ttl   time.Duration
}

// NewRMCCache creates a new RMC cache with the given TTL.
// If ttl <= 0, DefaultRMCCacheTTL is used.
func NewRMCCache(ttl time.Duration) *RMCCache {
	if ttl <= 0 {
		ttl = DefaultRMCCacheTTL
	}
	return &RMCCache{
		items: make(map[string]*cachedRMC),
		ttl:   ttl,
	}
}

// Get retrieves an RMC from the cache by name.
// Returns nil if not found or if the cached entry has expired.
func (c *RMCCache) Get(name string) *mcov1alpha1.RenderedMachineConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[name]
	if !ok {
		return nil
	}

	if time.Since(item.fetchedAt) >= c.ttl {
		return nil
	}

	return item.rmc
}

// Set stores an RMC in the cache.
func (c *RMCCache) Set(name string, rmc *mcov1alpha1.RenderedMachineConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[name] = &cachedRMC{
		rmc:       rmc,
		fetchedAt: time.Now(),
	}
}

// Delete removes an RMC from the cache.
func (c *RMCCache) Delete(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, name)
}

// Clear removes all items from the cache.
func (c *RMCCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cachedRMC)
}

// Len returns the number of items in the cache (including expired ones).
func (c *RMCCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.items)
}

// Cleanup removes expired entries from the cache.
func (c *RMCCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for name, item := range c.items {
		if now.Sub(item.fetchedAt) >= c.ttl {
			delete(c.items, name)
		}
	}
}

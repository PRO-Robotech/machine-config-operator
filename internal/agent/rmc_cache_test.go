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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

func makeTestRMC(name string) *mcov1alpha1.RenderedMachineConfig {
	return &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			PoolName: "workers",
			Revision: "abc123",
		},
	}
}

// TestNewRMCCache verifies cache creation.
func TestNewRMCCache(t *testing.T) {
	t.Run("with custom TTL", func(t *testing.T) {
		cache := NewRMCCache(10 * time.Second)
		if cache == nil {
			t.Fatal("NewRMCCache returned nil")
		}
		if cache.ttl != 10*time.Second {
			t.Errorf("Expected TTL 10s, got %v", cache.ttl)
		}
	})

	t.Run("with zero TTL uses default", func(t *testing.T) {
		cache := NewRMCCache(0)
		if cache.ttl != DefaultRMCCacheTTL {
			t.Errorf("Expected default TTL %v, got %v", DefaultRMCCacheTTL, cache.ttl)
		}
	})

	t.Run("with negative TTL uses default", func(t *testing.T) {
		cache := NewRMCCache(-1 * time.Second)
		if cache.ttl != DefaultRMCCacheTTL {
			t.Errorf("Expected default TTL %v, got %v", DefaultRMCCacheTTL, cache.ttl)
		}
	})
}

// TestRMCCache_GetSet verifies basic get/set operations.
func TestRMCCache_GetSet(t *testing.T) {
	cache := NewRMCCache(1 * time.Minute)
	rmc := makeTestRMC("workers-abc123")

	// Get non-existent
	if got := cache.Get("workers-abc123"); got != nil {
		t.Errorf("Expected nil for non-existent key, got %v", got)
	}

	// Set and get
	cache.Set("workers-abc123", rmc)
	got := cache.Get("workers-abc123")
	if got == nil {
		t.Fatal("Expected to get cached RMC, got nil")
	}
	if got.Name != rmc.Name {
		t.Errorf("Expected name %s, got %s", rmc.Name, got.Name)
	}
}

// TestRMCCache_TTLExpiration verifies items expire after TTL.
func TestRMCCache_TTLExpiration(t *testing.T) {
	// Use very short TTL for testing
	cache := NewRMCCache(50 * time.Millisecond)
	rmc := makeTestRMC("workers-abc123")

	cache.Set("workers-abc123", rmc)

	// Should be present immediately
	if got := cache.Get("workers-abc123"); got == nil {
		t.Fatal("Expected cached RMC immediately after set")
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Should be expired now
	if got := cache.Get("workers-abc123"); got != nil {
		t.Error("Expected nil after TTL expiration")
	}
}

// TestRMCCache_Delete verifies delete operation.
func TestRMCCache_Delete(t *testing.T) {
	cache := NewRMCCache(1 * time.Minute)
	rmc := makeTestRMC("workers-abc123")

	cache.Set("workers-abc123", rmc)
	if got := cache.Get("workers-abc123"); got == nil {
		t.Fatal("Expected cached RMC after set")
	}

	cache.Delete("workers-abc123")
	if got := cache.Get("workers-abc123"); got != nil {
		t.Error("Expected nil after delete")
	}

	// Delete non-existent should not panic
	cache.Delete("non-existent")
}

// TestRMCCache_Clear verifies clear operation.
func TestRMCCache_Clear(t *testing.T) {
	cache := NewRMCCache(1 * time.Minute)

	cache.Set("rmc1", makeTestRMC("rmc1"))
	cache.Set("rmc2", makeTestRMC("rmc2"))
	cache.Set("rmc3", makeTestRMC("rmc3"))

	if cache.Len() != 3 {
		t.Fatalf("Expected 3 items, got %d", cache.Len())
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("Expected 0 items after clear, got %d", cache.Len())
	}

	// All items should be gone
	if cache.Get("rmc1") != nil || cache.Get("rmc2") != nil || cache.Get("rmc3") != nil {
		t.Error("Expected all items to be cleared")
	}
}

// TestRMCCache_Len verifies length reporting.
func TestRMCCache_Len(t *testing.T) {
	cache := NewRMCCache(1 * time.Minute)

	if cache.Len() != 0 {
		t.Errorf("Expected 0 for empty cache, got %d", cache.Len())
	}

	cache.Set("rmc1", makeTestRMC("rmc1"))
	if cache.Len() != 1 {
		t.Errorf("Expected 1 after first set, got %d", cache.Len())
	}

	cache.Set("rmc2", makeTestRMC("rmc2"))
	if cache.Len() != 2 {
		t.Errorf("Expected 2 after second set, got %d", cache.Len())
	}

	// Overwrite existing key shouldn't increase count
	cache.Set("rmc1", makeTestRMC("rmc1-updated"))
	if cache.Len() != 2 {
		t.Errorf("Expected 2 after overwrite, got %d", cache.Len())
	}
}

// TestRMCCache_Cleanup verifies cleanup of expired entries.
func TestRMCCache_Cleanup(t *testing.T) {
	cache := NewRMCCache(50 * time.Millisecond)

	cache.Set("rmc1", makeTestRMC("rmc1"))
	cache.Set("rmc2", makeTestRMC("rmc2"))

	// Wait for TTL
	time.Sleep(60 * time.Millisecond)

	// Add a fresh item
	cache.Set("rmc3", makeTestRMC("rmc3"))

	// Len includes expired items
	if cache.Len() != 3 {
		t.Fatalf("Expected 3 items before cleanup, got %d", cache.Len())
	}

	// Cleanup should remove expired
	cache.Cleanup()

	if cache.Len() != 1 {
		t.Errorf("Expected 1 item after cleanup, got %d", cache.Len())
	}

	// rmc3 should still be there
	if cache.Get("rmc3") == nil {
		t.Error("Expected rmc3 to survive cleanup")
	}
}

// TestRMCCache_Overwrite verifies overwriting resets TTL.
func TestRMCCache_Overwrite(t *testing.T) {
	cache := NewRMCCache(100 * time.Millisecond)

	rmc1 := makeTestRMC("workers-abc123")
	rmc2 := makeTestRMC("workers-abc123")
	rmc2.Spec.Revision = "def456" // Different revision

	cache.Set("workers-abc123", rmc1)

	// Wait 60ms (not yet expired)
	time.Sleep(60 * time.Millisecond)

	// Overwrite with new value
	cache.Set("workers-abc123", rmc2)

	// Wait another 60ms (would be expired if not overwritten)
	time.Sleep(60 * time.Millisecond)

	// Should still be valid with new value
	got := cache.Get("workers-abc123")
	if got == nil {
		t.Fatal("Expected cached RMC after overwrite")
	}
	if got.Spec.Revision != "def456" {
		t.Errorf("Expected revision def456, got %s", got.Spec.Revision)
	}
}

// TestRMCCache_ConcurrentAccess verifies thread safety.
func TestRMCCache_ConcurrentAccess(t *testing.T) {
	cache := NewRMCCache(1 * time.Minute)
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				name := "rmc"
				cache.Set(name, makeTestRMC(name))
				cache.Get(name)
				cache.Len()
				if j%10 == 0 {
					cache.Delete(name)
				}
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

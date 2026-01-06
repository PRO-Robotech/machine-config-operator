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

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordDrainDuration(t *testing.T) {
	drainDuration.Reset()

	RecordDrainDuration("workers", "node-1", 120.5)
	RecordDrainDuration("workers", "node-2", 60.0)

	count := testutil.CollectAndCount(drainDuration)
	if count != 2 {
		t.Errorf("expected 2 observations, got %d", count)
	}
}

func TestRecordDrainStuck(t *testing.T) {
	drainStuckTotal.Reset()

	RecordDrainStuck("workers")
	RecordDrainStuck("workers")
	RecordDrainStuck("infra")

	workersVal := testutil.ToFloat64(drainStuckTotal.WithLabelValues("workers"))
	if workersVal != 2 {
		t.Errorf("workers drain stuck count = %f, want 2", workersVal)
	}

	infraVal := testutil.ToFloat64(drainStuckTotal.WithLabelValues("infra"))
	if infraVal != 1 {
		t.Errorf("infra drain stuck count = %f, want 1", infraVal)
	}
}

func TestUpdateCordonedNodesGauge(t *testing.T) {
	cordonedNodes.Reset()

	UpdateCordonedNodesGauge("workers", 3)

	val := testutil.ToFloat64(cordonedNodes.WithLabelValues("workers"))
	if val != 3 {
		t.Errorf("cordoned nodes gauge = %f, want 3", val)
	}

	UpdateCordonedNodesGauge("workers", 1)
	val = testutil.ToFloat64(cordonedNodes.WithLabelValues("workers"))
	if val != 1 {
		t.Errorf("cordoned nodes gauge = %f, want 1 after update", val)
	}
}

func TestUpdateDrainingNodesGauge(t *testing.T) {
	drainingNodes.Reset()

	UpdateDrainingNodesGauge("workers", 2)

	val := testutil.ToFloat64(drainingNodes.WithLabelValues("workers"))
	if val != 2 {
		t.Errorf("draining nodes gauge = %f, want 2", val)
	}

	UpdateDrainingNodesGauge("workers", 0)
	val = testutil.ToFloat64(drainingNodes.WithLabelValues("workers"))
	if val != 0 {
		t.Errorf("draining nodes gauge = %f, want 0 after update", val)
	}
}

func TestResetPoolMetrics_ClearsNewMetrics(t *testing.T) {
	cordonedNodes.Reset()
	drainingNodes.Reset()

	UpdateCordonedNodesGauge("workers", 5)
	UpdateDrainingNodesGauge("workers", 3)

	ResetPoolMetrics("workers")

	collected := testutil.CollectAndCount(cordonedNodes)
	if collected != 0 {
		t.Errorf("expected cordonedNodes to be cleared for workers, got %d metrics", collected)
	}
}

func TestRecordReconcileResult(t *testing.T) {
	poolReconcileTotal.Reset()

	RecordReconcileResult("workers", "success")
	RecordReconcileResult("workers", "success")
	RecordReconcileResult("workers", "error")

	successVal := testutil.ToFloat64(poolReconcileTotal.WithLabelValues("workers", "success"))
	if successVal != 2 {
		t.Errorf("success count = %f, want 2", successVal)
	}

	errorVal := testutil.ToFloat64(poolReconcileTotal.WithLabelValues("workers", "error"))
	if errorVal != 1 {
		t.Errorf("error count = %f, want 1", errorVal)
	}
}

func TestRecordReconcileDuration(t *testing.T) {
	poolReconcileDuration.Reset()

	RecordReconcileDuration("workers", 0.5)
	RecordReconcileDuration("workers", 1.2)

	count := testutil.CollectAndCount(poolReconcileDuration)
	if count != 1 {
		t.Errorf("expected 1 histogram, got %d", count)
	}
}

func TestRecordPoolOverlapMetrics_NilOverlap(t *testing.T) {
	poolOverlapNodesTotal.Reset()
	poolOverlapConflictsTotal.Set(5) // Set to non-zero first

	RecordPoolOverlapMetrics(nil, []string{"workers", "infra"})

	val := testutil.ToFloat64(poolOverlapConflictsTotal)
	if val != 0 {
		t.Errorf("conflicts total = %f, want 0 for nil overlap", val)
	}

	workersVal := testutil.ToFloat64(poolOverlapNodesTotal.WithLabelValues("workers"))
	if workersVal != 0 {
		t.Errorf("workers overlap = %f, want 0", workersVal)
	}
}

func TestRecordPoolOverlapMetrics_WithConflicts(t *testing.T) {
	poolOverlapNodesTotal.Reset()
	poolOverlapConflictsTotal.Set(0)

	overlap := &OverlapResult{
		ConflictingNodes: map[string][]string{
			"node1": {"workers", "infra"},
			"node2": {"workers", "prod"},
		},
	}

	RecordPoolOverlapMetrics(overlap, []string{"workers", "infra", "prod"})

	conflictsVal := testutil.ToFloat64(poolOverlapConflictsTotal)
	if conflictsVal != 2 {
		t.Errorf("conflicts total = %f, want 2", conflictsVal)
	}

	workersVal := testutil.ToFloat64(poolOverlapNodesTotal.WithLabelValues("workers"))
	if workersVal != 2 {
		t.Errorf("workers overlap count = %f, want 2", workersVal)
	}

	infraVal := testutil.ToFloat64(poolOverlapNodesTotal.WithLabelValues("infra"))
	if infraVal != 1 {
		t.Errorf("infra overlap count = %f, want 1", infraVal)
	}
}

func TestDrainDurationBuckets(t *testing.T) {
	drainDuration.Reset()

	RecordDrainDuration("test", "node", 15.0)   // Should fall in first bucket
	RecordDrainDuration("test", "node", 300.0)  // Should fall in later bucket
	RecordDrainDuration("test", "node", 3600.0) // Should fall in last bucket

	count := testutil.CollectAndCount(drainDuration)
	if count != 1 {
		t.Errorf("expected 1 histogram series, got %d", count)
	}
}

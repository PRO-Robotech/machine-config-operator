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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	poolOverlapNodesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mco_pool_overlap_nodes_total",
			Help: "Number of nodes that match multiple MachineConfigPools",
		},
		[]string{"pool"},
	)

	poolOverlapConflictsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "mco_pool_overlap_conflicts_total",
			Help: "Total number of nodes involved in pool overlap conflicts",
		},
	)

	poolReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mco_pool_reconcile_total",
			Help: "Total number of MachineConfigPool reconciliations",
		},
		[]string{"pool", "result"},
	)

	poolReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mco_pool_reconcile_duration_seconds",
			Help:    "Duration of MachineConfigPool reconciliation in seconds",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),
		},
		[]string{"pool"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		poolOverlapNodesTotal,
		poolOverlapConflictsTotal,
		poolReconcileTotal,
		poolReconcileDuration,
	)
}

func RecordPoolOverlapMetrics(overlap *OverlapResult, pools []string) {
	if overlap == nil {
		for _, pool := range pools {
			poolOverlapNodesTotal.WithLabelValues(pool).Set(0)
		}
		poolOverlapConflictsTotal.Set(0)
		return
	}

	for _, pool := range pools {
		poolOverlapNodesTotal.WithLabelValues(pool).Set(0)
	}

	poolConflictCounts := make(map[string]int)
	for _, poolNames := range overlap.ConflictingNodes {
		for _, pool := range poolNames {
			poolConflictCounts[pool]++
		}
	}

	for pool, count := range poolConflictCounts {
		poolOverlapNodesTotal.WithLabelValues(pool).Set(float64(count))
	}

	poolOverlapConflictsTotal.Set(float64(overlap.ConflictCount()))
}

func RecordReconcileResult(pool, result string) {
	poolReconcileTotal.WithLabelValues(pool, result).Inc()
}

func RecordReconcileDuration(pool string, durationSeconds float64) {
	poolReconcileDuration.WithLabelValues(pool).Observe(durationSeconds)
}

func ResetPoolMetrics(pool string) {
	poolOverlapNodesTotal.DeleteLabelValues(pool)
}

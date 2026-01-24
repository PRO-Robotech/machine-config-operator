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
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/internal/renderer"
	"in-cloud.io/machine-config/pkg/drain"
)

// DefaultMCONamespace is the fallback namespace when POD_NAMESPACE is not set.
const DefaultMCONamespace = "machine-config-system"

// MachineConfigPoolReconciler reconciles a MachineConfigPool object.
type MachineConfigPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Namespace is the namespace where MCO components run.
	// Read from POD_NAMESPACE env var, defaults to "machine-config-system".
	Namespace string

	// Components
	debounce  *DebounceState
	annotator *NodeAnnotator
	cleaner   *RMCCleaner
	events    *EventRecorder
}

// NewMachineConfigPoolReconciler creates a new reconciler with all components.
func NewMachineConfigPoolReconciler(c client.Client, scheme *runtime.Scheme) *MachineConfigPoolReconciler {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = DefaultMCONamespace
	}

	return &MachineConfigPoolReconciler{
		Client:    c,
		Scheme:    scheme,
		Namespace: namespace,
		debounce:  NewDebounceState(),
		annotator: NewNodeAnnotator(c),
		cleaner:   NewRMCCleaner(c),
		events:    &EventRecorder{}, // nil-safe: methods check for nil recorder
	}
}

// +kubebuilder:rbac:groups=mco.in-cloud.io,resources=machineconfigpools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mco.in-cloud.io,resources=machineconfigpools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mco.in-cloud.io,resources=machineconfigpools/finalizers,verbs=update
// +kubebuilder:rbac:groups=mco.in-cloud.io,resources=machineconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=mco.in-cloud.io,resources=renderedmachineconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile handles MachineConfigPool reconciliation.
func (r *MachineConfigPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// 1. Get the MachineConfigPool
	pool := &mcov1alpha1.MachineConfigPool{}
	if err := r.Get(ctx, req.NamespacedName, pool); err != nil {
		if apierrors.IsNotFound(err) {
			r.debounce.Reset(req.Name)
			ResetPoolMetrics(req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Migration cleanup: remove deprecated conditions from earlier versions.
	if CleanupLegacyConditions(pool) {
		log.Info("cleaned up legacy conditions", "pool", pool.Name)
		if err := r.Status().Update(ctx, pool); err != nil {
			log.Error(err, "failed to update pool after legacy cleanup")
		}
	}

	if pool.Spec.Paused {
		log.Info("pool is paused, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Detect pool overlap before processing nodes
	overlap, err := DetectPoolOverlap(ctx, r.Client)
	if err != nil {
		log.Error(err, "failed to detect pool overlap")
	}

	// Check if overlap was previously true (for PoolOverlapResolved event)
	wasOverlapping := hasPoolOverlapCondition(pool)

	// Record overlap metrics and emit events
	if overlap != nil && overlap.HasConflicts() {
		conflictingPools := overlap.GetAllConflictingPools()
		RecordPoolOverlapMetrics(overlap, conflictingPools)
		conflictingNodes := overlap.GetConflictsForPool(pool.Name)
		if len(conflictingNodes) > 0 {
			r.events.PoolOverlapDetected(pool, conflictingNodes)
		}
		log.Info("pool overlap detected",
			"totalConflicts", overlap.ConflictCount(),
			"poolsAffected", conflictingPools)
	} else {
		// Reset metrics when no conflicts - reset all pools to clear any stale metrics
		// List all pools in cluster to ensure we clear metrics for previously conflicting pools
		allPoolNames, listErr := r.listAllPoolNames(ctx)
		if listErr != nil {
			log.Error(listErr, "failed to list pool names for metrics reset")
			allPoolNames = []string{pool.Name}
		}
		RecordPoolOverlapMetrics(nil, allPoolNames)
		// Emit PoolOverlapResolved if it was previously overlapping
		if wasOverlapping {
			r.events.PoolOverlapResolved(pool)
		}
	}

	nodes, err := SelectNodes(ctx, r.Client, pool)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to select nodes: %w", err)
	}

	// 3. Filter out conflicting nodes - they should not receive desired-revision
	nonConflictingNodes := FilterNonConflictingNodes(nodes, overlap)
	conflictingNodeCount := len(nodes) - len(nonConflictingNodes)
	if conflictingNodeCount > 0 {
		log.Info("skipping conflicting nodes",
			"conflicting", conflictingNodeCount,
			"total", len(nodes))
	}

	configs, err := SelectMachineConfigs(ctx, r.Client, pool)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to select MachineConfigs: %w", err)
	}

	configPtrs := make([]*mcov1alpha1.MachineConfig, len(configs))
	for i := range configs {
		configPtrs[i] = &configs[i]
	}

	merged := renderer.Merge(configPtrs)

	// Skip rollout if no MachineConfigs exist
	// This prevents unnecessary cordon/drain when pool is created without configs
	if len(configs) == 0 {
		log.Info("no MachineConfigs for pool, skipping rollout",
			"pool", pool.Name,
			"nodeCount", len(nodes))

		// Update status to reflect current state without triggering rollout
		// But still update overlap condition so PoolOverlap works for empty pools
		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := r.Get(ctx, req.NamespacedName, pool); err != nil {
				return err
			}
			pool.Status.MachineCount = len(nodes)
			pool.Status.ReadyMachineCount = len(nodes)
			pool.Status.UpdatedMachineCount = len(nodes)
			pool.Status.TargetRevision = ""
			// Apply overlap condition even for empty pools
			ApplyOverlapCondition(pool, overlap)
			return r.Status().Update(ctx, pool)
		}); err != nil {
			log.Error(err, "failed to update pool status for empty config")
		}
		return ctrl.Result{}, nil
	}

	hash := renderer.ComputeHash(merged)
	debounceSeconds := pool.Spec.Rollout.DebounceSeconds
	poolSpecHash := ComputePoolSpecHash(pool)
	shouldProceed, requeueAfter := r.debounce.CheckAndUpdate(pool.Name, hash.Full, poolSpecHash, debounceSeconds)
	if !shouldProceed {
		// Even during debounce, we still want to keep PoolOverlap status fresh.
		// Otherwise overlap conditions can get stuck until debounce completes.
		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := r.Get(ctx, req.NamespacedName, pool); err != nil {
				return err
			}
			ApplyOverlapCondition(pool, overlap)
			return r.Status().Update(ctx, pool)
		}); err != nil {
			log.Error(err, "failed to update overlap condition during debounce")
		}

		log.Info("debounce active, requeuing", "after", requeueAfter)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	rmc, err := r.ensureRMC(ctx, pool, merged)
	if err != nil {
		SetRenderDegradedCondition(pool, err.Error())
		if updateErr := r.Status().Update(ctx, pool); updateErr != nil {
			log.Error(updateErr, "failed to update pool status with RenderDegraded")
		}
		return ctrl.Result{}, fmt.Errorf("failed to ensure RMC: %w", err)
	}

	// Clear RenderDegraded on success
	ClearRenderDegradedCondition(pool)

	// 4. Select nodes for update respecting maxUnavailable
	// This returns nodes that can START a new update
	newNodesToUpdate := SelectNodesForUpdate(pool, nonConflictingNodes, rmc.Name)

	// Also include nodes that are already in-progress (cordoned/draining)
	// These need to continue their update lifecycle
	nodesToProcess := collectNodesInProgress(nonConflictingNodes, rmc.Name)

	// Merge: add new nodes to process list (avoiding duplicates)
	inProgressNames := make(map[string]bool)
	for i := range nodesToProcess {
		inProgressNames[nodesToProcess[i].Name] = true
	}
	for i := range newNodesToUpdate {
		if !inProgressNames[newNodesToUpdate[i].Name] {
			nodesToProcess = append(nodesToProcess, newNodesToUpdate[i])
		}
	}

	log.Info("processing node updates",
		"pool", pool.Name,
		"totalNodes", len(nonConflictingNodes),
		"newNodes", len(newNodesToUpdate),
		"inProgress", len(nodesToProcess)-len(newNodesToUpdate),
		"maxUnavailable", pool.Spec.Rollout.MaxUnavailable)

	// Emit event for new batch of nodes starting update
	if len(newNodesToUpdate) > 0 {
		newNodeNames := make([]string, len(newNodesToUpdate))
		for i := range newNodesToUpdate {
			newNodeNames[i] = newNodesToUpdate[i].Name
		}
		r.events.RolloutBatchStarted(pool, len(newNodesToUpdate), newNodeNames)
	}

	// 5. Process each node through cordon/drain/update/uncordon lifecycle
	var minRequeueAfter time.Duration
	var drainStuckNodes []string
	var uncordonedCount int

	// Get drain timeout from spec (defaults to 3600)
	drainTimeoutSeconds := pool.Spec.Rollout.DrainTimeoutSeconds
	if drainTimeoutSeconds == 0 {
		drainTimeoutSeconds = DefaultDrainTimeoutSeconds
	}

	// Get drain retry interval from spec (0 means auto-calculate)
	drainRetrySeconds := pool.Spec.Rollout.DrainRetrySeconds

	// Load drain exclusion config from ConfigMap in MCO namespace
	drainConfigResult, err := drain.LoadDrainConfig(ctx, r.Client, r.Namespace)
	if err != nil {
		log.Error(err, "failed to load drain config, using defaults")
		drainConfigResult.Config = drain.DefaultDrainConfig()
	}
	drainConfig := drainConfigResult.Config

	// Emit warning event if ConfigMap had invalid YAML (soft failure)
	if drainConfigResult.ParseWarning != nil {
		log.Error(drainConfigResult.ParseWarning, "drain config invalid, using defaults",
			"configMapRef", drainConfigResult.ConfigMapRef)
		r.events.DrainConfigInvalid(pool, drainConfigResult.ConfigMapRef, drainConfigResult.ParseWarning)
	}

	for i := range nodesToProcess {
		node := &nodesToProcess[i]

		result := ProcessNodeUpdate(ctx, r.Client, pool, node, rmc.Name, drainTimeoutSeconds, drainRetrySeconds, drainConfig, r.Namespace, r.events)

		// Emit lifecycle events based on result flags
		if result.Cordoned {
			r.events.NodeCordonStarted(pool, node.Name)
		}
		if result.DrainStarted {
			r.events.NodeDrainStarted(pool, node.Name)
		}
		if result.DrainFailed && result.DrainFailedMsg != "" {
			r.events.DrainFailed(pool, node.Name, result.DrainFailedMsg)
		}
		if result.DrainComplete {
			r.events.DrainComplete(pool, node.Name)
		}
		if result.Uncordoned {
			r.events.NodeUncordoned(pool, node.Name)
			uncordonedCount++
		}

		// Track drain stuck nodes
		if result.DrainStuck {
			drainStuckNodes = append(drainStuckNodes, node.Name)
		}

		// Track minimum requeue time
		if result.Result.RequeueAfter > 0 {
			if minRequeueAfter == 0 || result.Result.RequeueAfter < minRequeueAfter {
				minRequeueAfter = result.Result.RequeueAfter
			}
		}
	}

	// 6. Update status - use ALL nodes for accurate counts, but apply overlap and drain stuck conditions.
	// Re-fetch nodes to get latest annotations for accurate status calculation.
	// This is necessary because annotations may have been updated during reconcile
	// (by controller cordon/drain actions or by agent applying config).
	nodes, err = SelectNodes(ctx, r.Client, pool)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to re-fetch nodes for status: %w", err)
	}

	// Track if rollout just completed for event emission
	wasNotComplete := pool.Status.UpdatedMachineCount != pool.Status.MachineCount ||
		pool.Status.ReadyMachineCount != pool.Status.MachineCount

	// Compute status once outside retry loop for event emission
	applyTimeout := pool.Spec.Rollout.ApplyTimeoutSeconds
	if applyTimeout <= 0 {
		applyTimeout = DefaultApplyTimeoutSeconds
	}
	aggregatedStatus := AggregateStatus(rmc.Name, nodes, applyTimeout)

	// Track whether rollout just completed for event emission after retry loop
	var rolloutJustCompleted bool

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.Get(ctx, req.NamespacedName, pool); err != nil {
			return err
		}
		// Recompute status with potentially updated pool spec
		status := AggregateStatus(rmc.Name, nodes, pool.Spec.Rollout.ApplyTimeoutSeconds)
		ApplyStatusToPool(pool, status)
		// Apply overlap condition (adds PoolOverlap and potentially Degraded)
		ApplyOverlapCondition(pool, overlap)

		// Apply drain stuck condition
		if len(drainStuckNodes) > 0 {
			msg := fmt.Sprintf("Drain stuck on nodes: %s", strings.Join(drainStuckNodes, ", "))
			SetDrainStuckCondition(pool, msg)
		} else {
			ClearDrainStuckCondition(pool)
		}

		// Update metrics
		UpdateCordonedNodesGauge(pool.Name, status.CordonedMachineCount)
		UpdateDrainingNodesGauge(pool.Name, status.DrainingMachineCount)

		// Track rollout completion for event emission outside retry loop
		rolloutJustCompleted = wasNotComplete && status.MachineCount > 0 &&
			status.UpdatedMachineCount == status.MachineCount &&
			status.ReadyMachineCount == status.MachineCount

		return r.Status().Update(ctx, pool)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update pool status: %w", err)
	}

	// Emit DrainStuck events
	if len(drainStuckNodes) > 0 {
		for _, nodeName := range drainStuckNodes {
			r.events.DrainStuck(pool, nodeName)
		}
		log.Info("drain stuck detected",
			"pool", pool.Name,
			"nodes", drainStuckNodes)
	}

	// Emit RolloutComplete if all nodes just became updated and ready
	if rolloutJustCompleted {
		r.events.RolloutComplete(pool)
		log.Info("rollout complete", "pool", pool.Name)
	}

	// Emit ApplyTimeout events
	if len(aggregatedStatus.TimedOutNodes) > 0 {
		for _, nodeName := range aggregatedStatus.TimedOutNodes {
			r.events.ApplyTimeout(pool, nodeName, applyTimeout)
		}
		log.Info("apply timeout detected",
			"pool", pool.Name,
			"nodes", aggregatedStatus.TimedOutNodes,
			"timeoutSeconds", applyTimeout)
	}

	deleted, err := r.cleaner.CleanupOldRMCs(ctx, pool, nodes)
	if err != nil {
		log.Error(err, "failed to cleanup old RMCs")
		// Don't fail reconciliation for cleanup errors
	}
	if deleted > 0 {
		log.Info("cleaned up old RMCs", "count", deleted)
	}

	// Return with requeue if node updates are in progress
	if minRequeueAfter > 0 {
		return ctrl.Result{RequeueAfter: minRequeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

func (r *MachineConfigPoolReconciler) ensureRMC(
	ctx context.Context,
	pool *mcov1alpha1.MachineConfigPool,
	merged *renderer.MergedConfig,
) (*mcov1alpha1.RenderedMachineConfig, error) {
	log := log.FromContext(ctx)

	rmc := renderer.BuildRMC(pool.Name, merged, pool)
	if rmc.Labels == nil {
		rmc.Labels = make(map[string]string)
	}
	rmc.Labels["mco.in-cloud.io/pool"] = pool.Name

	existing, err := renderer.CheckExistingRMC(ctx, r.Client, rmc)
	if err != nil {
		// If this is a hash collision, continue into the suffix retry loop below.
		if !errors.Is(err, renderer.ErrHashCollision) {
			return nil, err
		}
	}

	if existing != nil {
		if existing.Spec.ConfigHash == rmc.Spec.ConfigHash {
			needsUpdate := existing.Spec.Reboot.Strategy != rmc.Spec.Reboot.Strategy ||
				existing.Spec.Reboot.MinIntervalSeconds != rmc.Spec.Reboot.MinIntervalSeconds

			if needsUpdate {
				log.Info("updating RMC reboot spec",
					"name", existing.Name,
					"oldStrategy", existing.Spec.Reboot.Strategy,
					"newStrategy", rmc.Spec.Reboot.Strategy)
				existing.Spec.Reboot = rmc.Spec.Reboot
				if err := r.Update(ctx, existing); err != nil {
					return nil, fmt.Errorf("failed to update RMC reboot spec: %w", err)
				}
			}
			return existing, nil
		}
		// Hash collision detected - find an available name with retry loop
		originalName := rmc.Name
		log.Info("hash collision detected, trying with suffix",
			"rmc", originalName,
			"existingHash", existing.Spec.ConfigHash,
			"newHash", rmc.Spec.ConfigHash)

		const maxCollisionRetries = 10
		for suffix := 1; suffix <= maxCollisionRetries; suffix++ {
			candidateName := fmt.Sprintf("%s-%d", originalName, suffix)
			candidate := &mcov1alpha1.RenderedMachineConfig{}
			err := r.Get(ctx, client.ObjectKey{Name: candidateName}, candidate)
			if err != nil && apierrors.IsNotFound(err) {
				// Name is available
				rmc.Name = candidateName
				log.Info("hash collision resolved",
					"originalName", originalName,
					"newName", candidateName)
				break
			}
			if err != nil {
				return nil, fmt.Errorf("failed to check RMC name availability: %w", err)
			}
			// Name exists - check if same hash
			if candidate.Spec.ConfigHash == rmc.Spec.ConfigHash {
				// Same config, can reuse
				return candidate, nil
			}
			// Different hash, continue to next suffix
			if suffix == maxCollisionRetries {
				return nil, fmt.Errorf("hash collision: exhausted all %d suffix attempts for %s", maxCollisionRetries, originalName)
			}
		}
	}

	if err := ctrl.SetControllerReference(pool, rmc, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := r.Create(ctx, rmc); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if err := r.Get(ctx, client.ObjectKeyFromObject(rmc), rmc); err != nil {
				return nil, err
			}
			return rmc, nil
		}
		return nil, err
	}

	log.Info("created RenderedMachineConfig", "name", rmc.Name)
	return rmc, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MachineConfigPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize EventRecorder
	r.events = NewEventRecorder(mgr.GetEventRecorderFor("machineconfigpool-controller"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&mcov1alpha1.MachineConfigPool{}).
		Owns(&mcov1alpha1.RenderedMachineConfig{}).
		Watches(&mcov1alpha1.MachineConfig{}, handler.EnqueueRequestsFromMapFunc(r.mapMachineConfigToPool)).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(r.mapNodeToPool)).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(r.mapDrainConfigToStuckPools)).
		Complete(r)
}

// mapMachineConfigToPool maps a MachineConfig to the pools that select it.
func (r *MachineConfigPoolReconciler) mapMachineConfigToPool(ctx context.Context, obj client.Object) []reconcile.Request {
	mc, ok := obj.(*mcov1alpha1.MachineConfig)
	if !ok {
		return nil
	}

	pools := &mcov1alpha1.MachineConfigPoolList{}
	if err := r.List(ctx, pools); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, pool := range pools.Items {
		matches, err := MachineConfigMatchesPool(mc, &pool)
		if err != nil || !matches {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&pool),
		})
	}

	return requests
}

// mapNodeToPool maps a Node to the pools that select it.
func (r *MachineConfigPoolReconciler) mapNodeToPool(ctx context.Context, obj client.Object) []reconcile.Request {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil
	}

	pools := &mcov1alpha1.MachineConfigPoolList{}
	if err := r.List(ctx, pools); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, pool := range pools.Items {
		matches, err := NodeMatchesPool(node, &pool)
		if err != nil || !matches {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&pool),
		})
	}

	return requests
}

// hasPoolOverlapCondition checks if the pool has a PoolOverlap condition set to True.
func hasPoolOverlapCondition(pool *mcov1alpha1.MachineConfigPool) bool {
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionPoolOverlap && c.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// listAllPoolNames returns names of all MachineConfigPools in the cluster.
// Used to reset metrics for all pools when overlap is resolved.
func (r *MachineConfigPoolReconciler) listAllPoolNames(ctx context.Context) ([]string, error) {
	pools := &mcov1alpha1.MachineConfigPoolList{}
	if err := r.List(ctx, pools); err != nil {
		return nil, err
	}

	names := make([]string, len(pools.Items))
	for i, pool := range pools.Items {
		names[i] = pool.Name
	}
	return names, nil
}

// mapDrainConfigToStuckPools triggers reconcile ONLY for pools with DrainStuck=True
// when drain config ConfigMap changes. This allows fast reaction to config changes
// without affecting healthy pools.
func (r *MachineConfigPoolReconciler) mapDrainConfigToStuckPools(ctx context.Context, obj client.Object) []reconcile.Request {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return nil
	}

	if cm.Namespace != r.Namespace {
		return nil
	}
	if cm.Labels == nil || cm.Labels[drain.DrainConfigLabel] != drain.DrainConfigLabelValue {
		return nil
	}

	// List all pools
	pools := &mcov1alpha1.MachineConfigPoolList{}
	if err := r.List(ctx, pools); err != nil {
		return nil
	}

	// Only enqueue pools with DrainStuck=True condition
	var requests []reconcile.Request
	for _, pool := range pools.Items {
		if hasConditionStatus(&pool, mcov1alpha1.ConditionDrainStuck, metav1.ConditionTrue) {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&pool),
			})
		}
	}
	return requests
}

// hasConditionStatus checks if pool has a specific condition with given status.
func hasConditionStatus(pool *mcov1alpha1.MachineConfigPool, condType string, status metav1.ConditionStatus) bool {
	for _, c := range pool.Status.Conditions {
		if c.Type == condType && c.Status == status {
			return true
		}
	}
	return false
}

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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/internal/renderer"
)

// MachineConfigPoolReconciler reconciles a MachineConfigPool object.
type MachineConfigPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Components
	debounce  *DebounceState
	annotator *NodeAnnotator
	cleaner   *RMCCleaner
}

// NewMachineConfigPoolReconciler creates a new reconciler with all components.
func NewMachineConfigPoolReconciler(c client.Client, scheme *runtime.Scheme) *MachineConfigPoolReconciler {
	return &MachineConfigPoolReconciler{
		Client:    c,
		Scheme:    scheme,
		debounce:  NewDebounceState(),
		annotator: NewNodeAnnotator(c),
		cleaner:   NewRMCCleaner(c),
	}
}

// +kubebuilder:rbac:groups=mco.in-cloud.io,resources=machineconfigpools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mco.in-cloud.io,resources=machineconfigpools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mco.in-cloud.io,resources=machineconfigpools/finalizers,verbs=update
// +kubebuilder:rbac:groups=mco.in-cloud.io,resources=machineconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=mco.in-cloud.io,resources=renderedmachineconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;patch

// Reconcile handles MachineConfigPool reconciliation.
func (r *MachineConfigPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// 1. Get the MachineConfigPool
	pool := &mcov1alpha1.MachineConfigPool{}
	if err := r.Get(ctx, req.NamespacedName, pool); err != nil {
		if apierrors.IsNotFound(err) {
			r.debounce.Reset(req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if pool.Spec.Paused {
		log.Info("pool is paused, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	nodes, err := SelectNodes(ctx, r.Client, pool)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to select nodes: %w", err)
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
	hash := renderer.ComputeHash(merged)
	debounceSeconds := pool.Spec.Rollout.DebounceSeconds
	poolSpecHash := ComputePoolSpecHash(pool)
	shouldProceed, requeueAfter := r.debounce.CheckAndUpdate(pool.Name, hash.Full, poolSpecHash, debounceSeconds)
	if !shouldProceed {
		log.Info("debounce active, requeuing", "after", requeueAfter)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	rmc, err := r.ensureRMC(ctx, pool, merged)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure RMC: %w", err)
	}

	updated, err := r.annotator.SetDesiredRevisionForNodes(ctx, nodes, rmc.Name, pool.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set desired revision: %w", err)
	}
	if updated > 0 {
		log.Info("updated node annotations", "count", updated)
	}

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.Get(ctx, req.NamespacedName, pool); err != nil {
			return err
		}
		status := AggregateStatus(rmc.Name, nodes)
		ApplyStatusToPool(pool, status)
		return r.Status().Update(ctx, pool)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update pool status: %w", err)
	}

	deleted, err := r.cleaner.CleanupOldRMCs(ctx, pool, nodes)
	if err != nil {
		log.Error(err, "failed to cleanup old RMCs")
		// Don't fail reconciliation for cleanup errors
	}
	if deleted > 0 {
		log.Info("cleaned up old RMCs", "count", deleted)
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
		return nil, err
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
		log.Info("hash collision detected, adding suffix", "rmc", rmc.Name)
		rmc.Name = rmc.Name + "-1"
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcov1alpha1.MachineConfigPool{}).
		Owns(&mcov1alpha1.RenderedMachineConfig{}).
		Watches(&mcov1alpha1.MachineConfig{}, handler.EnqueueRequestsFromMapFunc(r.mapMachineConfigToPool)).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(r.mapNodeToPool)).
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

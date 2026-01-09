//go:build envtest || e2e

package fixtures

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// CleanupFunc is a function that cleans up test resources.
type CleanupFunc func(ctx context.Context, c client.Client) error

// CreateAndCleanup creates a resource and returns a cleanup function.
// Use with DeferCleanup in Ginkgo tests.
func CreateAndCleanup(c client.Client, obj client.Object) CleanupFunc {
	return func(ctx context.Context, cl client.Client) error {
		return client.IgnoreNotFound(cl.Delete(ctx, obj))
	}
}

// GetPool fetches a MachineConfigPool by name.
func GetPool(ctx context.Context, c client.Client, name string) (*mcov1alpha1.MachineConfigPool, error) {
	pool := &mcov1alpha1.MachineConfigPool{}
	err := c.Get(ctx, client.ObjectKey{Name: name}, pool)
	return pool, err
}

// GetNode fetches a Node by name.
func GetNode(ctx context.Context, c client.Client, name string) (*corev1.Node, error) {
	node := &corev1.Node{}
	err := c.Get(ctx, client.ObjectKey{Name: name}, node)
	return node, err
}

// GetRMC fetches a RenderedMachineConfig by name.
func GetRMC(ctx context.Context, c client.Client, name string) (*mcov1alpha1.RenderedMachineConfig, error) {
	rmc := &mcov1alpha1.RenderedMachineConfig{}
	err := c.Get(ctx, client.ObjectKey{Name: name}, rmc)
	return rmc, err
}

// ListRMCs lists all RenderedMachineConfigs.
func ListRMCs(ctx context.Context, c client.Client) (*mcov1alpha1.RenderedMachineConfigList, error) {
	list := &mcov1alpha1.RenderedMachineConfigList{}
	err := c.List(ctx, list)
	return list, err
}

// ListPodsOnNode lists all pods scheduled on a node.
func ListPodsOnNode(ctx context.Context, c client.Client, nodeName string) (*corev1.PodList, error) {
	list := &corev1.PodList{}
	err := c.List(ctx, list, client.MatchingFields{"spec.nodeName": nodeName})
	return list, err
}

// NodeIsCordoned returns true if node is cordoned (unschedulable).
func NodeIsCordoned(node *corev1.Node) bool {
	return node.Spec.Unschedulable
}

// NodeHasAnnotation checks if node has a specific annotation with expected value.
func NodeHasAnnotation(node *corev1.Node, key, expectedValue string) bool {
	value, exists := node.Annotations[key]
	return exists && value == expectedValue
}

// PoolHasCondition checks if pool has a condition with the specified status.
func PoolHasCondition(pool *mcov1alpha1.MachineConfigPool, condType string, status metav1.ConditionStatus) bool {
	for _, cond := range pool.Status.Conditions {
		if cond.Type == condType {
			return cond.Status == status
		}
	}
	return false
}

// CleanupPool deletes a pool and all its associated RMCs.
func CleanupPool(ctx context.Context, c client.Client, poolName string) error {
	// Delete pool
	pool := &mcov1alpha1.MachineConfigPool{}
	if err := c.Get(ctx, client.ObjectKey{Name: poolName}, pool); err == nil {
		_ = c.Delete(ctx, pool)
	}

	// Delete associated RMCs
	rmcList := &mcov1alpha1.RenderedMachineConfigList{}
	if err := c.List(ctx, rmcList, client.MatchingLabels{"mco.in-cloud.io/pool": poolName}); err == nil {
		for i := range rmcList.Items {
			_ = c.Delete(ctx, &rmcList.Items[i])
		}
	}
	return nil
}

// GetNodeAnnotation gets an annotation from a node, returning empty if not found.
func GetNodeAnnotation(ctx context.Context, c client.Client, nodeName, key string) string {
	node, err := GetNode(ctx, c, nodeName)
	if err != nil {
		return ""
	}
	return node.Annotations[key]
}

// CountCordonedNodes counts how many nodes in a pool are cordoned.
func CountCordonedNodes(ctx context.Context, c client.Client, poolName string) int {
	nodeList := &corev1.NodeList{}
	err := c.List(ctx, nodeList, client.MatchingLabels{"test-pool": poolName})
	if err != nil {
		return 0
	}
	count := 0
	for _, node := range nodeList.Items {
		if node.Spec.Unschedulable {
			count++
		}
	}
	return count
}

// SimulateAgentApply simulates an agent completing apply by setting annotations.
func SimulateAgentApply(ctx context.Context, c client.Client, nodeName, revision string) error {
	// The controller may be patching the Node concurrently; use optimistic retries.
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		node := &corev1.Node{}
		if err := c.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
			return err
		}

		if node.Annotations == nil {
			node.Annotations = make(map[string]string)
		}
		// Agent responsibility: report observed state only.
		node.Annotations["mco.in-cloud.io/current-revision"] = revision
		node.Annotations["mco.in-cloud.io/agent-state"] = "done"

		return c.Update(ctx, node)
	})
}

// GetPoolStatus returns the status of a MachineConfigPool, or nil if not found.
func GetPoolStatus(ctx context.Context, c client.Client, poolName string) *mcov1alpha1.MachineConfigPoolStatus {
	pool, err := GetPool(ctx, c, poolName)
	if err != nil {
		return nil
	}
	return &pool.Status
}

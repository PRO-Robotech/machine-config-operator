//go:build envtest || e2e

// Package fixtures provides builder-pattern helpers for creating test resources.
// These builders ensure consistent and flexible test resource creation.
package fixtures

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// PoolBuilder builds MachineConfigPool resources for testing.
type PoolBuilder struct {
	pool *mcov1alpha1.MachineConfigPool
}

// NewPool creates a new PoolBuilder with sensible defaults for testing.
func NewPool(name string) *PoolBuilder {
	return &PoolBuilder{
		pool: &mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					"test-pool": name,
				},
			},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test-pool": name,
					},
				},
				MachineConfigSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"mco.in-cloud.io/pool": name,
					},
				},
				Rollout: mcov1alpha1.RolloutConfig{
					// NOTE: DebounceSeconds has a CRD default (=30) and `omitempty` on the field.
					// If we set it to 0 here, it will be omitted from JSON and the apiserver will
					// default it back to 30, making tests very slow/flaky. Use 1s instead.
					DebounceSeconds:     1,
					ApplyTimeoutSeconds: 60,
					MaxUnavailable:      ptr.To(intstr.FromInt(1)),
				},
				Reboot: mcov1alpha1.RebootPolicy{
					Strategy:           "Never",
					MinIntervalSeconds: 0,
				},
				RevisionHistory: mcov1alpha1.RevisionHistoryConfig{
					Limit: 3,
				},
				Paused: false,
			},
		},
	}
}

// WithNodeSelector sets custom node selector labels.
func (b *PoolBuilder) WithNodeSelector(labels map[string]string) *PoolBuilder {
	b.pool.Spec.NodeSelector = &metav1.LabelSelector{
		MatchLabels: labels,
	}
	return b
}

// WithMachineConfigSelector sets custom MC selector labels.
func (b *PoolBuilder) WithMachineConfigSelector(labels map[string]string) *PoolBuilder {
	b.pool.Spec.MachineConfigSelector = &metav1.LabelSelector{
		MatchLabels: labels,
	}
	return b
}

// WithMaxUnavailable sets maxUnavailable as integer.
func (b *PoolBuilder) WithMaxUnavailable(n int) *PoolBuilder {
	b.pool.Spec.Rollout.MaxUnavailable = ptr.To(intstr.FromInt(n))
	return b
}

// WithMaxUnavailablePercent sets maxUnavailable as percentage string.
func (b *PoolBuilder) WithMaxUnavailablePercent(percent string) *PoolBuilder {
	b.pool.Spec.Rollout.MaxUnavailable = ptr.To(intstr.FromString(percent))
	return b
}

// WithDebounce sets debounce seconds.
func (b *PoolBuilder) WithDebounce(seconds int) *PoolBuilder {
	b.pool.Spec.Rollout.DebounceSeconds = seconds
	return b
}

// WithApplyTimeout sets apply timeout seconds.
func (b *PoolBuilder) WithApplyTimeout(seconds int) *PoolBuilder {
	b.pool.Spec.Rollout.ApplyTimeoutSeconds = seconds
	return b
}

// WithDrainTimeout sets drain timeout seconds.
func (b *PoolBuilder) WithDrainTimeout(seconds int) *PoolBuilder {
	b.pool.Spec.Rollout.DrainTimeoutSeconds = seconds
	return b
}

// WithRebootStrategy sets reboot strategy.
func (b *PoolBuilder) WithRebootStrategy(strategy string) *PoolBuilder {
	b.pool.Spec.Reboot.Strategy = strategy
	return b
}

// WithPaused sets pool paused state.
func (b *PoolBuilder) WithPaused(paused bool) *PoolBuilder {
	b.pool.Spec.Paused = paused
	return b
}

// WithLabels adds labels to pool.
func (b *PoolBuilder) WithLabels(labels map[string]string) *PoolBuilder {
	if b.pool.Labels == nil {
		b.pool.Labels = make(map[string]string)
	}
	for k, v := range labels {
		b.pool.Labels[k] = v
	}
	return b
}

// Build returns the constructed MachineConfigPool.
func (b *PoolBuilder) Build() *mcov1alpha1.MachineConfigPool {
	return b.pool.DeepCopy()
}

// MCBuilder builds MachineConfig resources for testing.
type MCBuilder struct {
	mc *mcov1alpha1.MachineConfig
}

// NewMC creates a new MCBuilder with sensible defaults.
func NewMC(name string) *MCBuilder {
	return &MCBuilder{
		mc: &mcov1alpha1.MachineConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: map[string]string{},
			},
			Spec: mcov1alpha1.MachineConfigSpec{
				Priority: 50,
				Files:    []mcov1alpha1.FileSpec{},
				Systemd:  mcov1alpha1.SystemdSpec{},
				Reboot: mcov1alpha1.RebootRequirementSpec{
					Required: false,
				},
			},
		},
	}
}

// ForPool sets the pool label for this MC.
func (b *MCBuilder) ForPool(poolName string) *MCBuilder {
	b.mc.Labels["mco.in-cloud.io/pool"] = poolName
	return b
}

// WithPriority sets the priority.
func (b *MCBuilder) WithPriority(priority int) *MCBuilder {
	b.mc.Spec.Priority = priority
	return b
}

// WithFile adds a file to the MC.
func (b *MCBuilder) WithFile(path, content string, mode int) *MCBuilder {
	b.mc.Spec.Files = append(b.mc.Spec.Files, mcov1alpha1.FileSpec{
		Path:    path,
		Content: content,
		Mode:    mode,
		Owner:   "root:root",
		State:   "present",
	})
	return b
}

// WithFileAbsent adds a file to be deleted.
func (b *MCBuilder) WithFileAbsent(path string) *MCBuilder {
	b.mc.Spec.Files = append(b.mc.Spec.Files, mcov1alpha1.FileSpec{
		Path:  path,
		State: "absent",
	})
	return b
}

// WithSystemdUnit adds a systemd unit.
// Note: enabled is *bool in the real API
func (b *MCBuilder) WithSystemdUnit(name string, enabled bool, state string) *MCBuilder {
	b.mc.Spec.Systemd.Units = append(b.mc.Spec.Systemd.Units, mcov1alpha1.UnitSpec{
		Name:    name,
		Enabled: ptr.To(enabled),
		State:   state,
	})
	return b
}

// WithRebootRequired sets reboot required flag.
func (b *MCBuilder) WithRebootRequired(required bool, reason string) *MCBuilder {
	b.mc.Spec.Reboot.Required = required
	b.mc.Spec.Reboot.Reason = reason
	return b
}

// WithLabels adds labels to MC.
func (b *MCBuilder) WithLabels(labels map[string]string) *MCBuilder {
	for k, v := range labels {
		b.mc.Labels[k] = v
	}
	return b
}

// Build returns the constructed MachineConfig.
func (b *MCBuilder) Build() *mcov1alpha1.MachineConfig {
	return b.mc.DeepCopy()
}

// NodeBuilder builds Node resources for testing.
type NodeBuilder struct {
	node *corev1.Node
}

// NewNode creates a new NodeBuilder with sensible defaults.
func NewNode(name string) *NodeBuilder {
	return &NodeBuilder{
		node: &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			Spec: corev1.NodeSpec{
				Unschedulable: false,
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
	}
}

// ForPool sets the pool label for this node.
func (b *NodeBuilder) ForPool(poolName string) *NodeBuilder {
	b.node.Labels["test-pool"] = poolName
	return b
}

// WithLabels adds labels to node.
func (b *NodeBuilder) WithLabels(labels map[string]string) *NodeBuilder {
	for k, v := range labels {
		b.node.Labels[k] = v
	}
	return b
}

// WithAnnotations adds annotations to node.
func (b *NodeBuilder) WithAnnotations(annotations map[string]string) *NodeBuilder {
	for k, v := range annotations {
		b.node.Annotations[k] = v
	}
	return b
}

// WithCurrentRevision sets current-revision annotation.
func (b *NodeBuilder) WithCurrentRevision(revision string) *NodeBuilder {
	b.node.Annotations["mco.in-cloud.io/current-revision"] = revision
	return b
}

// WithDesiredRevision sets desired-revision annotation.
func (b *NodeBuilder) WithDesiredRevision(revision string) *NodeBuilder {
	b.node.Annotations["mco.in-cloud.io/desired-revision"] = revision
	return b
}

// WithAgentState sets agent-state annotation.
func (b *NodeBuilder) WithAgentState(state string) *NodeBuilder {
	b.node.Annotations["mco.in-cloud.io/agent-state"] = state
	return b
}

// WithPaused sets pause annotation.
func (b *NodeBuilder) WithPaused(paused bool) *NodeBuilder {
	if paused {
		b.node.Annotations["mco.in-cloud.io/paused"] = "true"
	} else {
		delete(b.node.Annotations, "mco.in-cloud.io/paused")
	}
	return b
}

// WithUnschedulable sets node as unschedulable (cordoned).
func (b *NodeBuilder) WithUnschedulable(unschedulable bool) *NodeBuilder {
	b.node.Spec.Unschedulable = unschedulable
	return b
}

// WithCondition adds or updates a node condition.
func (b *NodeBuilder) WithCondition(condType corev1.NodeConditionType, status corev1.ConditionStatus) *NodeBuilder {
	for i, c := range b.node.Status.Conditions {
		if c.Type == condType {
			b.node.Status.Conditions[i].Status = status
			return b
		}
	}
	b.node.Status.Conditions = append(b.node.Status.Conditions, corev1.NodeCondition{
		Type:   condType,
		Status: status,
	})
	return b
}

// Build returns the constructed Node.
func (b *NodeBuilder) Build() *corev1.Node {
	return b.node.DeepCopy()
}

// PodBuilder builds Pod resources for testing drain scenarios.
type PodBuilder struct {
	pod *corev1.Pod
}

// NewPod creates a new PodBuilder.
func NewPod(name, namespace string) *PodBuilder {
	return &PodBuilder{
		pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test",
						Image: "busybox:latest",
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		},
	}
}

// OnNode assigns pod to a node.
func (b *PodBuilder) OnNode(nodeName string) *PodBuilder {
	b.pod.Spec.NodeName = nodeName
	return b
}

// WithLabels adds labels to pod.
func (b *PodBuilder) WithLabels(labels map[string]string) *PodBuilder {
	for k, v := range labels {
		b.pod.Labels[k] = v
	}
	return b
}

// WithOwnerReference adds owner reference (for eviction testing).
func (b *PodBuilder) WithOwnerReference(kind, name, uid string) *PodBuilder {
	b.pod.OwnerReferences = append(b.pod.OwnerReferences, metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       kind,
		Name:       name,
		UID:        types.UID("test-uid-" + uid),
	})
	return b
}

// AsDaemonSetPod marks pod as owned by DaemonSet (skipped during drain).
func (b *PodBuilder) AsDaemonSetPod() *PodBuilder {
	return b.WithOwnerReference("DaemonSet", "test-ds", "ds")
}

// WithPhase sets pod phase.
func (b *PodBuilder) WithPhase(phase corev1.PodPhase) *PodBuilder {
	b.pod.Status.Phase = phase
	return b
}

// Build returns the constructed Pod.
func (b *PodBuilder) Build() *corev1.Pod {
	return b.pod.DeepCopy()
}

// CreateNodes creates multiple nodes for a pool.
func CreateNodes(poolName string, count int) []*corev1.Node {
	nodes := make([]*corev1.Node, count)
	for i := 0; i < count; i++ {
		nodes[i] = NewNode(fmt.Sprintf("%s-node-%d", poolName, i)).
			ForPool(poolName).
			Build()
	}
	return nodes
}

// CreateNodesWithState creates nodes with initial MCO state.
func CreateNodesWithState(poolName string, count int, currentRevision string) []*corev1.Node {
	nodes := make([]*corev1.Node, count)
	for i := 0; i < count; i++ {
		nodes[i] = NewNode(fmt.Sprintf("%s-node-%d", poolName, i)).
			ForPool(poolName).
			WithCurrentRevision(currentRevision).
			WithDesiredRevision(currentRevision).
			WithAgentState("done").
			Build()
	}
	return nodes
}

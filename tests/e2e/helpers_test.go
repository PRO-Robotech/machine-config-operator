//go:build e2e
// +build e2e

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

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/tests/testutil"
)

func createTestPool(name string, maxUnavailable interface{}) *mcov1alpha1.MachineConfigPool {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"node-role.kubernetes.io/worker": "",
				},
			},
			MachineConfigSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"mco.in-cloud.io/pool": name,
				},
			},
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds:     1,
				ApplyTimeoutSeconds: 300,
			},
			Reboot: mcov1alpha1.RebootPolicy{
				Strategy: "Never",
			},
			RevisionHistory: mcov1alpha1.RevisionHistoryConfig{
				Limit: 3,
			},
			Paused: false,
		},
	}

	switch v := maxUnavailable.(type) {
	case int:
		pool.Spec.Rollout.MaxUnavailable = intOrStringPtr(intstr.FromInt(v))
	case string:
		pool.Spec.Rollout.MaxUnavailable = intOrStringPtr(intstr.FromString(v))
	}

	return pool
}

func intOrStringPtr(val intstr.IntOrString) *intstr.IntOrString {
	return &val
}

func createTestMachineConfig(name, poolName string) *mcov1alpha1.MachineConfig {
	return &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"mco.in-cloud.io/pool": poolName,
			},
		},
		Spec: mcov1alpha1.MachineConfigSpec{
			Priority: 50,
			Files: []mcov1alpha1.FileSpec{
				{
					Path:    fmt.Sprintf("/etc/mco-test/%s.conf", name),
					Content: fmt.Sprintf("# Generated at %s\ntest=true", time.Now().Format(time.RFC3339)),
					Mode:    0644,
					Owner:   "root:root",
					State:   "present",
				},
			},
		},
	}
}

func getPoolStatus(ctx context.Context, name string) (*mcov1alpha1.MachineConfigPoolStatus, error) {
	cmd := exec.Command("kubectl", "get", "mcp", name, "-o", "json")
	output, err := testutil.Run(cmd)
	if err != nil {
		return nil, err
	}

	var pool mcov1alpha1.MachineConfigPool
	if err := json.Unmarshal([]byte(output), &pool); err != nil {
		return nil, fmt.Errorf("failed to parse MCP JSON: %w", err)
	}

	return &pool.Status, nil
}

func countCordonedNodes(ctx context.Context) (int, error) {
	cmd := exec.Command("kubectl", "get", "nodes",
		"-o", "jsonpath={.items[*].spec.unschedulable}")
	output, err := testutil.Run(cmd)
	if err != nil {
		return 0, err
	}

	count := 0
	fields := strings.Fields(output)
	for _, f := range fields {
		if f == "true" {
			count++
		}
	}
	return count, nil
}

func countNodesWithAnnotation(ctx context.Context, annotation, value string) (int, error) {
	// Escape dots in annotation key for jsonpath (kubectl requires this)
	escapedAnnotation := strings.ReplaceAll(annotation, ".", `\.`)
	cmd := exec.Command("kubectl", "get", "nodes",
		"-o", fmt.Sprintf("jsonpath={.items[*].metadata.annotations.%s}", escapedAnnotation))
	output, err := testutil.Run(cmd)
	if err != nil {
		return 0, err
	}

	count := 0
	fields := strings.Fields(output)
	for _, f := range fields {
		if f == value {
			count++
		}
	}
	return count, nil
}

func getPoolCondition(ctx context.Context, poolName, conditionType string) (string, error) {
	cmd := exec.Command("kubectl", "get", "mcp", poolName,
		"-o", fmt.Sprintf("jsonpath={.status.conditions[?(@.type=='%s')].status}", conditionType))
	output, err := testutil.Run(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func isPoolUpdated(ctx context.Context, poolName string) (bool, error) {
	status, err := getPoolStatus(ctx, poolName)
	if err != nil {
		return false, err
	}

	// Consider the pool "updated" when all machines are updated and there is no
	// in-progress / error state remaining.
	// Note: we intentionally don't treat "Paused" as updated here; paused pools
	// are handled by dedicated tests.
	if status.MachineCount == 0 {
		return false, nil
	}

	return status.UpdatedMachineCount == status.MachineCount &&
		status.ReadyMachineCount == status.MachineCount &&
		status.UpdatingMachineCount == 0 &&
		status.DegradedMachineCount == 0 &&
		status.CordonedMachineCount == 0 &&
		status.DrainingMachineCount == 0, nil
}

func getWorkerNodes() ([]string, error) {
	cmd := exec.Command("kubectl", "get", "nodes",
		"-l", "node-role.kubernetes.io/worker=",
		"-o", "jsonpath={.items[*].metadata.name}")
	output, err := testutil.Run(cmd)
	if err != nil {
		return nil, err
	}
	return strings.Fields(output), nil
}

func labelNode(nodeName, label string) error {
	cmd := exec.Command("kubectl", "label", "node", nodeName, label, "--overwrite")
	_, err := testutil.Run(cmd)
	return err
}

func unlabelNode(nodeName, label string) error {
	cmd := exec.Command("kubectl", "label", "node", nodeName, label+"-")
	_, err := testutil.Run(cmd)
	return err
}

func applyYAML(yaml string) error {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(yaml)
	_, err := testutil.Run(cmd)
	return err
}

func deleteResource(kind, name string) error {
	cmd := exec.Command("kubectl", "delete", kind, name, "--ignore-not-found")
	_, err := testutil.Run(cmd)
	return err
}

func waitForCondition(ctx context.Context, check func() (bool, error), timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ok, err := check()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for condition")
}

func createPDB(name, namespace, appLabel string, minAvailable int) error {
	yaml := fmt.Sprintf(`
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: %s
  namespace: %s
spec:
  minAvailable: %d
  selector:
    matchLabels:
      app: %s
`, name, namespace, minAvailable, appLabel)
	return applyYAML(yaml)
}

func createDeployment(name, namespace, nodeName string, replicas int) error {
	yaml := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
spec:
  replicas: %d
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      nodeSelector:
        kubernetes.io/hostname: %s
      containers:
      - name: app
        image: busybox
        command: ["sleep", "infinity"]
`, name, namespace, replicas, name, name, nodeName)
	return applyYAML(yaml)
}

func getNodes(labelSelector string) ([]corev1.Node, error) {
	args := []string{"get", "nodes", "-o", "json"}
	if labelSelector != "" {
		args = append(args, "-l", labelSelector)
	}
	cmd := exec.Command("kubectl", args...)
	output, err := testutil.Run(cmd)
	if err != nil {
		return nil, err
	}

	var nodeList corev1.NodeList
	if err := json.Unmarshal([]byte(output), &nodeList); err != nil {
		return nil, fmt.Errorf("failed to parse NodeList JSON: %w", err)
	}

	return nodeList.Items, nil
}

func getMCP(name string) (*mcov1alpha1.MachineConfigPool, error) {
	cmd := exec.Command("kubectl", "get", "mcp", name, "-o", "json")
	output, err := testutil.Run(cmd)
	if err != nil {
		return nil, err
	}

	var pool mcov1alpha1.MachineConfigPool
	if err := json.Unmarshal([]byte(output), &pool); err != nil {
		return nil, fmt.Errorf("failed to parse MCP JSON: %w", err)
	}

	return &pool, nil
}

func getUpdatingNodeCount(ctx context.Context, poolName string) (int, error) {
	cmd := exec.Command("kubectl", "get", "mcp", poolName,
		"-o", "jsonpath={.status.updatingMachineCount}")
	output, err := testutil.Run(cmd)
	if err != nil {
		return 0, err
	}
	var count int
	_, _ = fmt.Sscanf(output, "%d", &count)
	return count, nil
}

func getCordonedNodeCount(ctx context.Context, poolName string) (int, error) {
	cmd := exec.Command("kubectl", "get", "mcp", poolName,
		"-o", "jsonpath={.status.cordonedMachineCount}")
	output, err := testutil.Run(cmd)
	if err != nil {
		return 0, err
	}
	var count int
	_, _ = fmt.Sscanf(output, "%d", &count)
	return count, nil
}

func getDrainingNodeCount(ctx context.Context, poolName string) (int, error) {
	cmd := exec.Command("kubectl", "get", "mcp", poolName,
		"-o", "jsonpath={.status.drainingMachineCount}")
	output, err := testutil.Run(cmd)
	if err != nil {
		return 0, err
	}
	var count int
	_, _ = fmt.Sscanf(output, "%d", &count)
	return count, nil
}

const (
	// mcoNamespace is the namespace where MCO components run
	mcoNamespace = "machine-config-system"
	// agentLabelSelector selects agent pods
	agentLabelSelector = "app.kubernetes.io/name=mco-agent"
)

// listAgentPodsByNode returns a map of nodeName -> agentPodName
func listAgentPodsByNode() (map[string]string, error) {
	cmd := exec.Command("kubectl", "get", "pods",
		"-n", mcoNamespace,
		"-l", agentLabelSelector,
		"-o", "jsonpath={range .items[*]}{.spec.nodeName}={.metadata.name}{\"\\n\"}{end}")
	output, err := testutil.Run(cmd)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result, nil
}

// execInAgentPod runs a command in the agent pod and returns stdout
func execInAgentPod(podName, command string) (string, error) {
	cmd := exec.Command("kubectl", "exec", podName,
		"-n", mcoNamespace,
		"--", "sh", "-c", command)
	output, err := testutil.Run(cmd)
	if err != nil {
		return output, err
	}
	return strings.TrimSpace(output), nil
}

// fileExistsOnHost checks if a file exists on /host in the agent pod
func fileExistsOnHost(podName, filePath string) (bool, error) {
	command := fmt.Sprintf("test -f /host%s && echo 'EXISTS' || echo 'NOT_FOUND'", filePath)
	output, err := execInAgentPod(podName, command)
	if err != nil {
		return false, err
	}
	return output == "EXISTS", nil
}

// cleanupTestFilesOnHost removes /etc/mco-test directory from host filesystem via agent pods.
// This ensures tests don't see stale files from previous runs.
func cleanupTestFilesOnHost() {
	agentPods, err := listAgentPodsByNode()
	if err != nil {
		// Pods might not be running yet during initial setup
		return
	}
	for _, podName := range agentPods {
		// Remove the entire test directory to ensure clean state
		command := "rm -rf /host/etc/mco-test 2>/dev/null || true"
		_, _ = execInAgentPod(podName, command)
	}
}

// readFileOnHost reads file content from /host in the agent pod
func readFileOnHost(podName, filePath string) (string, error) {
	command := fmt.Sprintf("cat /host%s", filePath)
	return execInAgentPod(podName, command)
}

// getFileStatsOnHost gets file mode and owner from /host
func getFileStatsOnHost(podName, filePath string) (mode string, owner string, err error) {
	// stat -c '%a %U:%G' /host/path
	command := fmt.Sprintf("stat -c '%%a %%U:%%G' /host%s 2>/dev/null || echo 'ERROR'", filePath)
	output, err := execInAgentPod(podName, command)
	if err != nil {
		return "", "", err
	}
	if output == "ERROR" {
		return "", "", fmt.Errorf("file not found: %s", filePath)
	}
	parts := strings.SplitN(output, " ", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected stat output: %s", output)
	}
	return parts[0], parts[1], nil
}

// waitPoolUpdated waits for MCP to be fully updated
func waitPoolUpdated(poolName string, timeout time.Duration) error {
	return waitForCondition(context.Background(), func() (bool, error) {
		return isPoolUpdated(context.Background(), poolName)
	}, timeout)
}

// getNodeAnnotation gets a specific annotation from a node
func getNodeAnnotation(nodeName, annotation string) (string, error) {
	// Escape dots in annotation key for jsonpath (kubectl requires this)
	escapedAnnotation := strings.ReplaceAll(annotation, ".", `\.`)
	cmd := exec.Command("kubectl", "get", "node", nodeName,
		"-o", fmt.Sprintf("jsonpath={.metadata.annotations.%s}", escapedAnnotation))
	output, err := testutil.Run(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// isNodeUnschedulable checks if a node is cordoned
func isNodeUnschedulable(nodeName string) (bool, error) {
	cmd := exec.Command("kubectl", "get", "node", nodeName,
		"-o", "jsonpath={.spec.unschedulable}")
	output, err := testutil.Run(cmd)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) == "true", nil
}

// uncordonNode marks a node as schedulable
func uncordonNode(nodeName string) error {
	// First, use kubectl uncordon to set spec.unschedulable=false
	cmd := exec.Command("kubectl", "uncordon", nodeName)
	if _, err := testutil.Run(cmd); err != nil {
		return err
	}
	// Also remove MCO-specific annotations that track cordoned state
	// The controller uses these annotations, not just spec.unschedulable
	annotations := []string{
		"mco.in-cloud.io/cordoned",
		"mco.in-cloud.io/drain-started-at",
		"mco.in-cloud.io/drain-retry-count",
	}
	for _, ann := range annotations {
		cmd = exec.Command("kubectl", "annotate", "node", nodeName, ann+"-")
		_, _ = testutil.Run(cmd) // Ignore errors (annotation might not exist)
	}
	return nil
}

// uncordonAllWorkerNodes uncordons all worker nodes - useful for cleanup between tests
func uncordonAllWorkerNodes() {
	nodes, err := getWorkerNodes()
	if err != nil {
		return
	}
	for _, nodeName := range nodes {
		_ = uncordonNode(nodeName)
	}
}

// cleanupAllMCOResources performs a comprehensive cleanup of all MCO resources.
// This should be called before tests that are sensitive to leftover state.
func cleanupAllMCOResources() {
	By("cleaning up all MCO resources from previous tests")

	// Delete all MCPs
	cmd := exec.Command("kubectl", "delete", "mcp", "--all", "--ignore-not-found")
	_, _ = testutil.Run(cmd)

	// Clean up test files from host filesystem to ensure clean state.
	// This prevents issues where agent skips file apply because file already exists.
	cleanupTestFilesOnHost()

	// Delete all MCs
	cmd = exec.Command("kubectl", "delete", "mc", "--all", "--ignore-not-found")
	_, _ = testutil.Run(cmd)

	// Delete all RMCs
	cmd = exec.Command("kubectl", "delete", "rmc", "--all", "--ignore-not-found")
	_, _ = testutil.Run(cmd)

	// Uncordon all nodes and clean annotations
	uncordonAllWorkerNodes()

	// Also clean control-plane node annotations
	cmd = exec.Command("kubectl", "get", "nodes", "-o", "name")
	output, err := testutil.Run(cmd)
	if err == nil {
		for _, nodeName := range testutil.GetNonEmptyLines(output) {
			nodeName = strings.TrimPrefix(nodeName, "node/")
			cleanNodeAnnotations(nodeName)
		}
	}

	// Wait for agents to settle - they may be in retry loops trying to fetch deleted RMCs.
	time.Sleep(3 * time.Second)
}

// cleanNodeAnnotations removes all MCO-related annotations from a node
func cleanNodeAnnotations(nodeName string) {
	annotations := []string{
		"mco.in-cloud.io/desired-revision",
		"mco.in-cloud.io/current-revision",
		"mco.in-cloud.io/agent-state",
		"mco.in-cloud.io/cordoned",
		"mco.in-cloud.io/drain-started-at",
		"mco.in-cloud.io/drain-retry-count",
		"mco.in-cloud.io/reboot-pending",
		"mco.in-cloud.io/last-error",
		"mco.in-cloud.io/desired-revision-set-at",
	}
	for _, ann := range annotations {
		cmd := exec.Command("kubectl", "annotate", "node", nodeName, ann+"-")
		_, _ = testutil.Run(cmd)
	}
}

// getRMCsByPool returns list of RMC names for a pool
func getRMCsByPool(poolName string) ([]string, error) {
	cmd := exec.Command("kubectl", "get", "rmc",
		"-l", fmt.Sprintf("mco.in-cloud.io/pool=%s", poolName),
		"-o", "jsonpath={.items[*].metadata.name}")
	output, err := testutil.Run(cmd)
	if err != nil {
		return nil, err
	}
	if output == "" {
		return nil, nil
	}
	return strings.Fields(output), nil
}

func _(pool *mcov1alpha1.MachineConfigPool) {}
func _(nn types.NamespacedName)             {}

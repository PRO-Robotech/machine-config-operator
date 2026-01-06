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
	"fmt"
	"os/exec"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/test/utils"
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
				DebounceSeconds:     5,
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
	cmd := exec.Command("kubectl", "get", "mcp", name, "-o", "jsonpath={.status}")
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}
	_ = output
	return nil, nil
}

func countCordonedNodes(ctx context.Context) (int, error) {
	cmd := exec.Command("kubectl", "get", "nodes",
		"-o", "jsonpath={.items[*].spec.unschedulable}")
	output, err := utils.Run(cmd)
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
	cmd := exec.Command("kubectl", "get", "nodes",
		"-o", fmt.Sprintf("jsonpath={.items[*].metadata.annotations.%s}", strings.ReplaceAll(annotation, "/", "~1")))
	output, err := utils.Run(cmd)
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
	output, err := utils.Run(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func isPoolUpdated(ctx context.Context, poolName string) (bool, error) {
	cmd := exec.Command("kubectl", "get", "mcp", poolName,
		"-o", "jsonpath={.status.machineCount},{.status.updatedMachineCount},{.status.unavailableMachineCount}")
	output, err := utils.Run(cmd)
	if err != nil {
		return false, err
	}

	parts := strings.Split(output, ",")
	if len(parts) != 3 {
		return false, fmt.Errorf("unexpected output format: %s", output)
	}

	return parts[0] == parts[1] && parts[2] == "0", nil
}

func getWorkerNodes() ([]string, error) {
	cmd := exec.Command("kubectl", "get", "nodes",
		"-l", "node-role.kubernetes.io/worker=",
		"-o", "jsonpath={.items[*].metadata.name}")
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}
	return strings.Fields(output), nil
}

func labelNode(nodeName, label string) error {
	cmd := exec.Command("kubectl", "label", "node", nodeName, label, "--overwrite")
	_, err := utils.Run(cmd)
	return err
}

func unlabelNode(nodeName, label string) error {
	cmd := exec.Command("kubectl", "label", "node", nodeName, label+"-")
	_, err := utils.Run(cmd)
	return err
}

func applyYAML(yaml string) error {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(yaml)
	_, err := utils.Run(cmd)
	return err
}

func deleteResource(kind, name string) error {
	cmd := exec.Command("kubectl", "delete", kind, name, "--ignore-not-found")
	_, err := utils.Run(cmd)
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
		time.Sleep(5 * time.Second)
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
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}
	_ = output
	return nil, nil
}

func getMCP(name string) (*mcov1alpha1.MachineConfigPool, error) {
	cmd := exec.Command("kubectl", "get", "mcp", name, "-o", "json")
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}
	_ = output
	return nil, nil
}

func getUpdatingNodeCount(ctx context.Context, poolName string) (int, error) {
	cmd := exec.Command("kubectl", "get", "mcp", poolName,
		"-o", "jsonpath={.status.updatingMachineCount}")
	output, err := utils.Run(cmd)
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
	output, err := utils.Run(cmd)
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
	output, err := utils.Run(cmd)
	if err != nil {
		return 0, err
	}
	var count int
	_, _ = fmt.Sscanf(output, "%d", &count)
	return count, nil
}

func _ (pool *mcov1alpha1.MachineConfigPool) {}
func _ (nn types.NamespacedName) {}

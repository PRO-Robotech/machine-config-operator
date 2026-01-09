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
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"in-cloud.io/machine-config/tests/testutil"
)

var (
	// Optional Environment Variables:
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// These variables are useful if CertManager is already installed, avoiding
	// re-installation and conflicts.
	skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true"
	// isCertManagerAlreadyInstalled will be set true when CertManager CRDs be found on the cluster
	isCertManagerAlreadyInstalled = false

	// skipImageBuild skips building and loading images if already done (for rapid iteration)
	skipImageBuild = os.Getenv("SKIP_IMAGE_BUILD") == "true"

	// skipTeardown skips AfterSuite cleanup to allow debugging a live cluster.
	// Useful with focused runs: make test-e2e-focus ... SKIP_CLEANUP=true E2E_SKIP_TEARDOWN=true
	skipTeardown = os.Getenv("E2E_SKIP_TEARDOWN") == "true"

	// E2E images: controller and agent
	// These are built and loaded to Kind cluster during BeforeSuite
	controllerImage = getEnvOrDefault("E2E_CONTROLLER_IMG", "mco-controller:e2e")
	agentImage      = getEnvOrDefault("E2E_AGENT_IMG", "mco-agent:e2e")
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purpose of being used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally, and installs
// CertManager.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting machine-config integration test suite\n")
	RunSpecs(t, "e2e suite")
}

// getEnvOrDefault returns the environment variable value or the default.
func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

var _ = BeforeSuite(func() {
	if !skipImageBuild {
		By("building controller and agent images for E2E tests")
		cmd := exec.Command("make", "docker-build-e2e",
			fmt.Sprintf("E2E_CONTROLLER_IMG=%s", controllerImage),
			fmt.Sprintf("E2E_AGENT_IMG=%s", agentImage),
		)
		_, err := testutil.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build controller and agent images")

		By("loading controller image to Kind cluster")
		err = testutil.LoadImageToKindClusterWithName(controllerImage)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load controller image into Kind")

		By("loading agent image to Kind cluster")
		err = testutil.LoadImageToKindClusterWithName(agentImage)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load agent image into Kind")
	} else {
		_, _ = fmt.Fprintf(GinkgoWriter, "SKIP_IMAGE_BUILD=true: skipping image build/load\n")
	}

	// The tests-e2e are intended to run on a temporary cluster that is created and destroyed for testing.
	// To prevent errors when tests run in environments with CertManager already installed,
	// we check for its presence before execution.
	// Setup CertManager before the suite if not skipped and if not already installed
	if !skipCertManagerInstall {
		By("checking if cert manager is installed already")
		isCertManagerAlreadyInstalled = testutil.IsCertManagerCRDsInstalled()
		if !isCertManagerAlreadyInstalled {
			_, _ = fmt.Fprintf(GinkgoWriter, "Installing CertManager...\n")
			Expect(testutil.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: CertManager is already installed. Skipping installation...\n")
		}
	}

	By("creating machine-config-system namespace (if needed)")
	cmd := exec.Command("kubectl", "create", "ns", namespace)
	_, _ = testutil.Run(cmd) // ignore if it already exists

	By("labeling namespace for Pod Security admission (agent requires privileged)")
	cmd = exec.Command(
		"kubectl", "label", "--overwrite", "ns", namespace,
		"pod-security.kubernetes.io/enforce=privileged",
	)
	_, err := testutil.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to label namespace with privileged policy")

	By("installing CRDs")
	cmd = exec.Command("make", "install")
	_, err = testutil.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to install CRDs")

	By("labeling kind worker nodes with node-role.kubernetes.io/worker=")
	cmd = exec.Command("kubectl", "get", "nodes", "-o", "name")
	nodesOut, err := testutil.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to list nodes")
	for _, name := range testutil.GetNonEmptyLines(nodesOut) {
		nodeName := strings.TrimPrefix(name, "node/")
		if nodeName == "" || strings.Contains(nodeName, "control-plane") {
			continue
		}
		labelCmd := exec.Command("kubectl", "label", "node", nodeName, "node-role.kubernetes.io/worker=", "--overwrite")
		_, err := testutil.Run(labelCmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to label worker node", "node", nodeName)
	}

	By("deploying controller+agent for E2E")
	cmd = exec.Command(
		"make",
		"deploy-e2e",
		fmt.Sprintf("E2E_CONTROLLER_IMG=%s", controllerImage),
		fmt.Sprintf("E2E_AGENT_IMG=%s", agentImage),
	)
	_, err = testutil.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to deploy controller+agent")
})

var _ = AfterSuite(func() {
	if skipTeardown {
		_, _ = fmt.Fprintf(GinkgoWriter, "E2E_SKIP_TEARDOWN=true: skipping AfterSuite cleanup\n")
		return
	}
	By("undeploying controller+agent")
	cmd := exec.Command("make", "undeploy-e2e")
	_, _ = testutil.Run(cmd)

	By("uninstalling CRDs")
	cmd = exec.Command("make", "uninstall")
	_, _ = testutil.Run(cmd)

	By("removing machine-config-system namespace")
	cmd = exec.Command("kubectl", "delete", "ns", namespace)
	_, _ = testutil.Run(cmd)

	if !skipCertManagerInstall && !isCertManagerAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CertManager...\n")
		testutil.UninstallCertManager()
	}
})

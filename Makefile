# Image URLs to use for building/pushing image targets
CONTROLLER_IMG ?= mco-controller:latest
AGENT_IMG ?= mco-agent:latest
# Legacy IMG variable for compatibility with kubebuilder targets
IMG ?= $(CONTROLLER_IMG)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	"$(CONTROLLER_GEN)" rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= machine-config-test-e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_CLUSTER)'..."; \
			$(KIND) create cluster --name $(KIND_CLUSTER) --config tests/e2e/kind-config.yaml ;; \
	esac

.PHONY: test-envtest
test-envtest: manifests generate setup-envtest ## Run EnvTest tests (controller tests with real API server).
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" \
		go test -tags=envtest ./tests/envtest/... -v -ginkgo.v

.PHONY: test-unit
test-unit: ## Run unit tests with mocks (isolated tests).
	go test -tags=unit ./tests/agent/... -v

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test -timeout 30m -tags=e2e ./tests/e2e/ -v -ginkgo.v
	$(MAKE) cleanup-test-e2e

.PHONY: test-e2e-focus
test-e2e-focus: setup-test-e2e manifests generate fmt vet ## Run a focused subset of e2e tests. Usage: make test-e2e-focus FOCUS='Node Invariants|File Apply' [SKIP_CLEANUP=true]
	@if [ -z "$(FOCUS)" ]; then \
		echo "Error: FOCUS is required. Example:"; \
		echo "  make test-e2e-focus FOCUS='Node Invariants|File Apply'"; \
		exit 1; \
	fi
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test -timeout 30m -tags=e2e ./tests/e2e/ -v -ginkgo.v -ginkgo.focus="$(FOCUS)"
	@if [ "$(SKIP_CLEANUP)" != "true" ]; then $(MAKE) cleanup-test-e2e; fi

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

##@ E2E Docker Images

# E2E specific image names - used for building/loading to Kind
E2E_CONTROLLER_IMG ?= mco-controller:e2e
E2E_AGENT_IMG ?= mco-agent:e2e

.PHONY: docker-build-e2e
docker-build-e2e: ## Build docker images for E2E tests (controller + agent)
	$(MAKE) docker-build-controller CONTROLLER_IMG=$(E2E_CONTROLLER_IMG)
	$(MAKE) docker-build-agent AGENT_IMG=$(E2E_AGENT_IMG)

.PHONY: kind-load-e2e
kind-load-e2e: docker-build-e2e ## Load E2E images to Kind cluster
	$(KIND) load docker-image $(E2E_CONTROLLER_IMG) --name $(KIND_CLUSTER)
	$(KIND) load docker-image $(E2E_AGENT_IMG) --name $(KIND_CLUSTER)

.PHONY: deploy-e2e
deploy-e2e: manifests kustomize ## Deploy controller and agent for E2E tests
	$(KUSTOMIZE) build config/e2e | $(KUBECTL) apply -f -

.PHONY: undeploy-e2e
undeploy-e2e: kustomize ## Undeploy controller and agent for E2E tests
	$(KUSTOMIZE) build config/e2e | $(KUBECTL) delete --ignore-not-found -f -

##@ Scenario Tests (Minikube)

SCENARIO ?=

.PHONY: test-scenario
test-scenario: ## Run a single scenario test. Usage: make test-scenario SCENARIO=001-sysctl-basic
	@if [ -z "$(SCENARIO)" ]; then \
		echo "Error: SCENARIO is required. Usage: make test-scenario SCENARIO=001-sysctl-basic"; \
		echo ""; \
		echo "Available scenarios:"; \
		ls -d tests/scenarios/[0-9][0-9][0-9]-*/ 2>/dev/null | xargs -I{} basename {} | sed 's/^/  /'; \
		exit 1; \
	fi
	./tests/scenarios/run-scenario.sh $(SCENARIO)

.PHONY: test-scenarios
test-scenarios: ## Run all scenario tests in minikube
	./tests/scenarios/run-all.sh

.PHONY: list-scenarios
list-scenarios: ## List available test scenarios
	@echo "Available scenarios:"
	@ls -d tests/scenarios/[0-9][0-9][0-9]-*/ 2>/dev/null | xargs -I{} basename {} | sed 's/^/  /'

.PHONY: cleanup-scenarios
cleanup-scenarios: ## Cleanup all test scenarios
	./tests/scenarios/run-all.sh --cleanup-all

##@ Mocks

MOCKGEN ?= $(LOCALBIN)/mockgen
MOCKGEN_VERSION ?= v0.6.0

.PHONY: mockgen
mockgen: $(MOCKGEN) ## Download mockgen locally if necessary.
$(MOCKGEN): $(LOCALBIN)
	$(call go-install-tool,$(MOCKGEN),go.uber.org/mock/mockgen,$(MOCKGEN_VERSION))

.PHONY: generate-mocks
generate-mocks: mockgen ## Generate mocks from production interfaces.
	@echo "Generating mocks from production code..."
	@mkdir -p tests/mocks
	$(MOCKGEN) -source=internal/agent/files.go \
		-destination=tests/mocks/mock_files.go -package=mocks
	$(MOCKGEN) -source=internal/agent/systemd.go \
		-destination=tests/mocks/mock_systemd.go -package=mocks
	$(MOCKGEN) -source=internal/agent/reboot_decision.go \
		-destination=tests/mocks/mock_rmc_fetcher.go -package=mocks
	$(MOCKGEN) -source=internal/agent/reboot/handler.go \
		-destination=tests/mocks/mock_reboot.go -package=mocks
	@echo "Mocks generated in tests/mocks/"

.PHONY: verify-mocks
verify-mocks: generate-mocks ## Verify mocks are up to date.
	@git diff --exit-code tests/mocks/ || \
		(echo "Mocks are out of date. Run 'make generate-mocks'" && exit 1)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	"$(GOLANGCI_LINT)" run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	"$(GOLANGCI_LINT)" config verify

##@ Build

.PHONY: build
build: build-controller build-agent ## Build controller and agent binaries.

.PHONY: build-controller
build-controller: manifests generate fmt vet ## Build controller binary.
	go build -o bin/controller ./cmd/controller

.PHONY: build-agent
build-agent: fmt vet ## Build agent binary.
	go build -o bin/agent ./cmd/agent

.PHONY: run
run: manifests generate fmt vet ## Run controller from your host.
	go run ./cmd/controller/main.go

.PHONY: run-agent
run-agent: fmt vet ## Run agent from your host (requires root for systemd).
	go run ./cmd/agent/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: docker-build-controller docker-build-agent ## Build docker images for controller and agent.

.PHONY: docker-build-controller
docker-build-controller: ## Build docker image for controller.
	$(CONTAINER_TOOL) build -t ${CONTROLLER_IMG} -f Dockerfile.controller .

.PHONY: docker-build-agent
docker-build-agent: ## Build docker image for agent.
	$(CONTAINER_TOOL) build -t ${AGENT_IMG} -f Dockerfile.agent .

.PHONY: docker-push
docker-push: docker-push-controller docker-push-agent ## Push docker images for controller and agent.

.PHONY: docker-push-controller
docker-push-controller: ## Push docker image for controller.
	$(CONTAINER_TOOL) push ${CONTROLLER_IMG}

.PHONY: docker-push-agent
docker-push-agent: ## Push docker image for agent.
	$(CONTAINER_TOOL) push ${AGENT_IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name machine-config-builder
	$(CONTAINER_TOOL) buildx use machine-config-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm machine-config-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${CONTROLLER_IMG}
	"$(KUSTOMIZE)" build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" apply -f -; else echo "No CRDs to install; skipping."; fi

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -; else echo "No CRDs to delete; skipping."; fi

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller and agent to the K8s cluster specified in ~/.kube/config.
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${CONTROLLER_IMG}
	cd config/agent && "$(KUSTOMIZE)" edit set image agent=${AGENT_IMG}
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || echo "$(LOCALBIN)/golangci-lint")

## Tool Versions
KUSTOMIZE_VERSION ?= v5.7.1
CONTROLLER_TOOLS_VERSION ?= v0.19.0

#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell v='$(call gomodver,sigs.k8s.io/controller-runtime)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_VERSION manually (controller-runtime replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?([0-9]+)\.([0-9]+).*/release-\1.\2/')

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')

GOLANGCI_LINT_VERSION ?= v2.5.0
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@"$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: ## Download golangci-lint locally if necessary (skipped if found in PATH).
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		$(MAKE) $(LOCALBIN)/golangci-lint; \
	fi
$(LOCALBIN)/golangci-lint: $(LOCALBIN)
	$(call go-install-tool,$(LOCALBIN)/golangci-lint,github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef

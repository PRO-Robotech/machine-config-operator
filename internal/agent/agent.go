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

package agent

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/internal/agent/reboot"
	"in-cloud.io/machine-config/pkg/annotations"
	mcoclient "in-cloud.io/machine-config/pkg/client"
)

var agentLog = ctrl.Log.WithName("agent")

// Config holds the configuration for the Agent.
type Config struct {
	// NodeName is the name of the node this agent is running on.
	NodeName string

	// K8sClient is the Kubernetes client for node operations.
	K8sClient kubernetes.Interface

	// MCOClient is the MCO client for RMC operations.
	MCOClient mcoclient.MCOClient

	// HostRoot is the path prefix for file operations (e.g., "/host").
	HostRoot string

	// SystemdConn is the systemd connection. If nil, a real D-Bus connection is used.
	SystemdConn SystemdConnection

	// NoReboot disables actual reboots (uses NoOpExecutor for testing).
	NoReboot bool
}

// Agent manages configuration on a single node.
// It watches the node annotations and applies configurations when needed.
type Agent struct {
	nodeName      string
	k8sClient     kubernetes.Interface
	mcoClient     mcoclient.MCOClient
	applier       *Applier
	writer        *NodeWriter
	rebootHandler *reboot.Handler
	hostRoot      string

	// rmcCache caches RenderedMachineConfigs for diff-based reboot determination.
	rmcCache *RMCCache

	// rebootDeterminer handles diff-based reboot logic.
	rebootDeterminer *RebootDeterminer

	// pendingRebootRevision tracks which revision we've applied and are waiting
	// for reboot. This prevents re-applying the same config on every watch event
	// when the node object in the event is stale.
	pendingRebootRevision string
}

// New creates a new Agent with the given configuration.
// Use NewWithContext for production use when a real systemd connection is needed.
func New(cfg Config) (*Agent, error) {
	return NewWithContext(context.Background(), cfg)
}

// NewWithContext creates a new Agent with the given configuration and context.
// The context is used for creating the systemd D-Bus connection.
func NewWithContext(ctx context.Context, cfg Config) (*Agent, error) {
	if cfg.NodeName == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if cfg.K8sClient == nil {
		return nil, fmt.Errorf("kubernetes client is required")
	}
	if cfg.MCOClient == nil {
		return nil, fmt.Errorf("MCO client is required")
	}

	conn := cfg.SystemdConn
	if conn == nil {
		var err error
		conn, err = NewDBusConnection(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create systemd connection: %w", err)
		}
	}

	writer := NewNodeWriter(cfg.K8sClient, cfg.NodeName)

	var executor reboot.RebootExecutor
	if cfg.NoReboot {
		agentLog.Info("using no-op reboot executor (reboots disabled)")
		executor = &reboot.NoOpExecutor{}
	} else {
		executor = reboot.NewSystemdExecutor(cfg.HostRoot)
	}

	rebootHandler := reboot.NewHandler(cfg.HostRoot, writer, executor)
	rmcCache := NewRMCCache(DefaultRMCCacheTTL)
	agent := &Agent{
		nodeName:      cfg.NodeName,
		k8sClient:     cfg.K8sClient,
		mcoClient:     cfg.MCOClient,
		applier:       NewApplier(cfg.HostRoot, conn),
		writer:        writer,
		rebootHandler: rebootHandler,
		hostRoot:      cfg.HostRoot,
		rmcCache:      rmcCache,
	}

	agent.rebootDeterminer = NewRebootDeterminer(agent)

	return agent, nil
}

// Run starts the agent main loop.
// It watches the node annotations and applies configurations when needed.
func (a *Agent) Run(ctx context.Context) error {
	log := agentLog.WithValues("node", a.nodeName)
	log.Info("starting agent")

	var node *corev1.Node
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		var getErr error
		node, getErr = a.k8sClient.CoreV1().Nodes().Get(ctx, a.nodeName, metav1.GetOptions{})
		if getErr != nil {
			log.V(1).Info("waiting for API server to be ready", "error", getErr.Error())
			return false, nil // retry
		}
		return true, nil // success
	})
	if err != nil {
		log.Error(err, "failed to get node for startup check after retries, continuing anyway")
	} else {
		if err := a.rebootHandler.CheckRebootPendingOnStartup(ctx, node); err != nil {
			log.Error(err, "startup reboot check failed, continuing anyway")
		}
	}

	// Set initial state to idle
	if err := a.writer.SetState(ctx, annotations.StateIdle); err != nil {
		log.Error(err, "failed to set initial state")
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("context cancelled, stopping agent")
			return nil
		default:
		}

		if err := a.watchNode(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Error(err, "watch error, retrying in 5s")
			time.Sleep(5 * time.Second)
		}
	}
}

// Close closes any resources held by the agent.
func (a *Agent) Close() {
	if a.applier != nil {
		a.applier.Close()
	}
}

// watchNode watches the agent's own node using a field selector.
func (a *Agent) watchNode(ctx context.Context) error {
	log := agentLog.WithValues("node", a.nodeName)

	// Create watch with field selector for own node only
	watcher, err := a.k8sClient.CoreV1().Nodes().Watch(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", a.nodeName).String(),
	})
	if err != nil {
		return fmt.Errorf("failed to create node watch: %w", err)
	}
	defer watcher.Stop()

	log.V(1).Info("watching node for annotation changes")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event, ok := <-watcher.ResultChan():
			if !ok {
				// Watch closed, will reconnect
				log.V(1).Info("watch closed, reconnecting")
				return nil
			}

			if event.Type == watch.Error {
				return fmt.Errorf("watch error: %v", event.Object)
			}
			if event.Type != watch.Added && event.Type != watch.Modified {
				continue
			}

			node, ok := event.Object.(*corev1.Node)
			if !ok {
				log.V(1).Info("unexpected object type in watch")
				continue
			}

			if err := a.handleNodeUpdate(ctx, node); err != nil {
				log.Error(err, "failed to handle node update")
				// Continue watching, don't return error
			}
		}
	}
}

// handleNodeUpdate processes a node update and triggers apply if needed.
func (a *Agent) handleNodeUpdate(ctx context.Context, node *corev1.Node) error {
	log := agentLog.WithValues("node", a.nodeName)
	ann := node.Annotations

	if annotations.IsNodePaused(ann) {
		log.V(1).Info("node is paused, skipping")
		return nil
	}

	desired := annotations.GetAnnotation(ann, annotations.DesiredRevision)
	current := annotations.GetAnnotation(ann, annotations.CurrentRevision)

	if desired == "" {
		log.V(1).Info("no desired revision set")
		return nil
	}

	if desired == current {
		log.V(1).Info("already at desired revision", "revision", desired)
		return nil
	}

	rebootPending := a.pendingRebootRevision == desired || annotations.GetBoolAnnotation(ann, annotations.RebootPending)
	if rebootPending {
		log.V(1).Info("reboot pending, checking if interval elapsed", "revision", desired)
		rmc, err := a.fetchRMCWithRetry(ctx, desired)
		if err != nil {
			log.Error(err, "failed to fetch RMC for reboot check")
			return nil // Don't error out, just wait for next event
		}

		if err := a.rebootHandler.HandleReboot(ctx, rmc, node); err != nil {
			log.Error(err, "failed to handle reboot")
		}
		return nil
	}

	log.Info("update needed", "desired", desired, "current", current)

	rmc, err := a.fetchRMCWithRetry(ctx, desired)
	if err != nil {
		a.writer.SetStateWithError(ctx, annotations.StateError, fmt.Sprintf("fetch RMC: %v", err))
		return fmt.Errorf("fetch RMC %s: %w", desired, err)
	}

	return a.applyConfig(ctx, rmc, node)
}

// fetchRMCWithRetry fetches an RMC with exponential backoff retry.
func (a *Agent) fetchRMCWithRetry(ctx context.Context, name string) (*mcov1alpha1.RenderedMachineConfig, error) {
	var rmc *mcov1alpha1.RenderedMachineConfig
	var lastErr error

	backoff := wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   2.0,
		Steps:    5,
		Cap:      5 * time.Minute,
	}

	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		var err error
		rmc, err = a.mcoClient.RenderedMachineConfigs().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			lastErr = err
			if apierrors.IsNotFound(err) {
				agentLog.V(1).Info("RMC not found, retrying", "name", name)
				return false, nil
			}
			return false, err
		}
		return true, nil
	})

	if err != nil {
		if wait.Interrupted(err) && lastErr != nil {
			return nil, lastErr
		}
		return nil, err
	}

	return rmc, nil
}

// FetchRMC fetches an RMC by name, using cache when available.
// This method implements the RMCFetcher interface for use by RebootDeterminer.
func (a *Agent) FetchRMC(ctx context.Context, name string) (*mcov1alpha1.RenderedMachineConfig, error) {
	if cached := a.rmcCache.Get(name); cached != nil {
		agentLog.V(1).Info("RMC cache hit", "name", name)
		return cached, nil
	}

	rmc, err := a.mcoClient.RenderedMachineConfigs().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("RMC %s not found (may have been garbage collected)", name)
		}
		return nil, fmt.Errorf("failed to fetch RMC %s: %w", name, err)
	}

	a.rmcCache.Set(name, rmc)
	agentLog.V(1).Info("RMC fetched and cached", "name", name)

	return rmc, nil
}

// applyConfig applies the configuration from an RMC.
func (a *Agent) applyConfig(ctx context.Context, rmc *mcov1alpha1.RenderedMachineConfig, node *corev1.Node) error {
	log := agentLog.WithValues("node", a.nodeName, "revision", rmc.Name)

	if err := a.writer.SetState(ctx, annotations.StateApplying); err != nil {
		return fmt.Errorf("set applying state: %w", err)
	}

	// Clear any previous error
	if err := a.writer.ClearLastError(ctx); err != nil {
		log.V(1).Info("failed to clear last error", "error", err)
	}

	log.Info("applying configuration")
	result, err := a.applier.ApplySpec(ctx, &rmc.Spec)
	if err != nil {
		log.Error(err, "apply failed")
		a.writer.SetStateWithError(ctx, annotations.StateError, err.Error())
		return err
	}

	log.Info("apply successful",
		"filesApplied", result.FilesApplied,
		"filesSkipped", result.FilesSkipped,
		"unitsApplied", result.UnitsApplied,
		"unitsSkipped", result.UnitsSkipped)

	currentRevision := annotations.GetAnnotation(node.Annotations, annotations.CurrentRevision)
	decision := a.rebootDeterminer.DetermineReboot(ctx, currentRevision, rmc)

	log.Info("reboot decision",
		"required", decision.Required,
		"method", decision.Method,
		"reasons", decision.Reasons)

	originalRequired := rmc.Spec.Reboot.Required
	rmc.Spec.Reboot.Required = decision.Required
	if err := a.rebootHandler.HandleReboot(ctx, rmc, node); err != nil {
		log.Error(err, "failed to handle reboot")
		a.writer.SetStateWithError(ctx, annotations.StateError, fmt.Sprintf("reboot handling: %v", err))
		return err
	}
	rmc.Spec.Reboot.Required = originalRequired
	if decision.Required {
		a.pendingRebootRevision = rmc.Name
		log.Info("reboot required, waiting for reboot to complete")
		return nil
	}

	a.pendingRebootRevision = ""
	if err := a.writer.SetDone(ctx, rmc.Name); err != nil {
		return fmt.Errorf("set done state: %w", err)
	}

	return nil
}

// GetNodeName returns the name of the node this agent manages.
func (a *Agent) GetNodeName() string {
	return a.nodeName
}

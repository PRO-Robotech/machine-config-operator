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

// Package annotations defines the annotation contract between Controller and Agent.
// Controller writes DESIRED state, Agent writes OBSERVED state.
package annotations

const (
	// Prefix for all MCO annotations.
	Prefix = "mco.in-cloud.io/"

	// Controller-written annotations.

	// DesiredRevision is the full RMC name that the node should apply.
	DesiredRevision = Prefix + "desired-revision"

	// Pool is the name of the MachineConfigPool this node belongs to.
	Pool = Prefix + "pool"

	// Agent-written annotations.

	// CurrentRevision is the last successfully applied RMC name.
	CurrentRevision = Prefix + "current-revision"

	// AgentState is the current state of the agent (idle, applying, done, error).
	AgentState = Prefix + "agent-state"

	// LastError contains the error message if AgentState is "error".
	LastError = Prefix + "last-error"

	// RebootPending is "true" if a reboot is needed but blocked by policy.
	RebootPending = Prefix + "reboot-pending"

	// Cordoned is "true" if the node was cordoned by MCO for update.
	Cordoned = Prefix + "cordoned"

	// DrainStartedAt contains the timestamp when drain started.
	DrainStartedAt = Prefix + "drain-started-at"

	// DrainRetryCount contains the number of drain retry attempts.
	DrainRetryCount = Prefix + "drain-retry-count"

	// Control annotations (set by user/operator).

	// Paused is "true" to exclude the node from rollout.
	Paused = Prefix + "paused"

	// ForceReboot is "true" to force reboot ignoring minInterval.
	ForceReboot = Prefix + "force-reboot"

	// DesiredRevisionSetAt records when the controller set desired-revision.
	// Used for apply timeout detection.
	DesiredRevisionSetAt = Prefix + "desired-revision-set-at"
)

// Boolean annotation values.
const (
	// ValueTrue is the string "true" used for boolean annotations.
	ValueTrue = "true"
	// ValueFalse is the string "false" used for boolean annotations.
	ValueFalse = "false"
)

// Agent state values.
const (
	// StateIdle means the agent is waiting for work.
	StateIdle = "idle"

	// StateApplying means the agent is currently applying a configuration.
	StateApplying = "applying"

	// StateDone means the agent successfully applied the configuration.
	StateDone = "done"

	// StateError means the agent encountered an error.
	StateError = "error"
)

// GetAnnotation safely gets an annotation value from a map.
// Returns empty string if annotations is nil or key doesn't exist.
func GetAnnotation(annotations map[string]string, key string) string {
	if annotations == nil {
		return ""
	}
	return annotations[key]
}

// GetBoolAnnotation gets an annotation as a boolean.
// Returns true only if the annotation value is exactly "true".
func GetBoolAnnotation(annotations map[string]string, key string) bool {
	return GetAnnotation(annotations, key) == ValueTrue
}

// SetAnnotation sets an annotation value, creating the map if nil.
// Returns the updated annotations map.
func SetAnnotation(annotations map[string]string, key, value string) map[string]string {
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	return annotations
}

// RemoveAnnotation removes an annotation from the map.
// Returns the updated annotations map (safe to call with nil map).
func RemoveAnnotation(annotations map[string]string, key string) map[string]string {
	if annotations != nil {
		delete(annotations, key)
	}
	return annotations
}

// IsNodePaused checks if a node is paused based on its annotations.
func IsNodePaused(annotations map[string]string) bool {
	return GetBoolAnnotation(annotations, Paused)
}

// NeedsUpdate checks if desired-revision differs from current-revision.
// Returns false if desired-revision is not set.
func NeedsUpdate(annotations map[string]string) bool {
	desired := GetAnnotation(annotations, DesiredRevision)
	if desired == "" {
		return false
	}
	current := GetAnnotation(annotations, CurrentRevision)
	return desired != current
}

// IsUpToDate checks if current-revision matches desired-revision.
func IsUpToDate(annotations map[string]string) bool {
	desired := GetAnnotation(annotations, DesiredRevision)
	if desired == "" {
		return true // No desired revision means nothing to update
	}
	current := GetAnnotation(annotations, CurrentRevision)
	return desired == current
}

// IsReady checks if the node has applied the desired revision and is done.
func IsReady(annotations map[string]string) bool {
	return IsUpToDate(annotations) && GetAnnotation(annotations, AgentState) == StateDone
}

// Package mocks contains generated mock implementations for testing.
//
// IMPORTANT: Do not edit mock_*.go files manually!
// Use `make generate-mocks` to regenerate.
//
// Mocks are generated from production interfaces:
//   - internal/agent/files.go → FileOperations
//   - internal/agent/systemd.go → SystemdConnection
//   - internal/agent/reboot_decision.go → RMCFetcher
//   - internal/agent/reboot/handler.go → NodeAnnotationWriter, RebootExecutor
package mocks

import (
	// Import mock package to ensure it's in go.mod
	_ "go.uber.org/mock/gomock"
)

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

package renderer

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

func TestIsPathForbidden(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		// Forbidden paths
		{"bin directory", "/bin/bash", true},
		{"sbin directory", "/sbin/init", true},
		{"usr bin", "/usr/bin/python", true},
		{"usr sbin", "/usr/sbin/nginx", true},
		{"kubernetes config", "/etc/kubernetes/admin.conf", true},
		{"kubelet data", "/var/lib/kubelet/config.yaml", true},
		{"etcd data", "/var/lib/etcd/member", true},
		{"proc filesystem", "/proc/1/status", true},
		{"sys filesystem", "/sys/class/net", true},
		{"dev filesystem", "/dev/sda1", true},
		{"cni config", "/etc/cni/net.d/10-flannel.conf", true},
		{"systemd lib", "/usr/lib/systemd/system/foo.service", true},
		{"lib systemd", "/lib/systemd/system/bar.service", true},

		// Allowed paths
		{"etc config", "/etc/chrony.conf", false},
		{"etc subdir", "/etc/sysconfig/network", false},
		{"opt directory", "/opt/myapp/config.yaml", false},
		{"var log", "/var/log/messages", false},
		{"home directory", "/home/user/.bashrc", false},
		{"root directory", "/root/.profile", false},
		{"tmp directory", "/tmp/test.txt", false},
		{"usr local", "/usr/local/bin/script.sh", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPathForbidden(tt.path)
			if result != tt.expected {
				t.Errorf("IsPathForbidden(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsUnitForbidden(t *testing.T) {
	tests := []struct {
		name     string
		unit     string
		expected bool
	}{
		// Forbidden units
		{"kubelet", "kubelet.service", true},
		{"containerd", "containerd.service", true},
		{"docker", "docker.service", true},
		{"cri-o", "cri-o.service", true},
		{"etcd", "etcd.service", true},

		// Allowed units
		{"chronyd", "chronyd.service", false},
		{"nginx", "nginx.service", false},
		{"sshd", "sshd.service", false},
		{"custom timer", "backup.timer", false},
		{"custom socket", "myapp.socket", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsUnitForbidden(tt.unit)
			if result != tt.expected {
				t.Errorf("IsUnitForbidden(%q) = %v, want %v", tt.unit, result, tt.expected)
			}
		})
	}
}

func TestHasValidUnitSuffix(t *testing.T) {
	tests := []struct {
		name     string
		unit     string
		expected bool
	}{
		// Valid suffixes
		{"service", "nginx.service", true},
		{"timer", "backup.timer", true},
		{"socket", "myapp.socket", true},
		{"mount", "data.mount", true},
		{"target", "multi-user.target", true},

		// Invalid suffixes
		{"no suffix", "nginx", false},
		{"wrong suffix", "nginx.conf", false},
		{"partial suffix", "nginx.serv", false},
		{"uppercase suffix", "nginx.SERVICE", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasValidUnitSuffix(tt.unit)
			if result != tt.expected {
				t.Errorf("HasValidUnitSuffix(%q) = %v, want %v", tt.unit, result, tt.expected)
			}
		})
	}
}

func TestValidateFilePath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantError bool
		errMsg    string
	}{
		// Valid paths
		{"valid etc path", "/etc/chrony.conf", false, ""},
		{"valid opt path", "/opt/app/config.yaml", false, ""},
		{"valid var path", "/var/log/app.log", false, ""},

		// Invalid paths
		{"empty path", "", true, "cannot be empty"},
		{"relative path", "etc/config", true, "must be absolute"},
		{"path traversal", "/etc/../root/.ssh/id_rsa", true, "cannot contain '..'"},
		{"double slash", "/etc//config", true, "cannot contain '//'"},
		{"forbidden bin", "/bin/sh", true, "forbidden"},
		{"forbidden kubernetes", "/etc/kubernetes/pki/ca.crt", true, "forbidden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilePath(tt.path)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateFilePath(%q) = nil, want error containing %q", tt.path, tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateFilePath(%q) error = %v, want error containing %q", tt.path, err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateFilePath(%q) = %v, want nil", tt.path, err)
				}
			}
		})
	}
}

func TestValidateUnitName(t *testing.T) {
	tests := []struct {
		name      string
		unit      string
		wantError bool
		errMsg    string
	}{
		// Valid units
		{"valid service", "nginx.service", false, ""},
		{"valid timer", "backup.timer", false, ""},
		{"valid socket", "myapp.socket", false, ""},
		{"valid mount", "data.mount", false, ""},
		{"valid target", "custom.target", false, ""},

		// Invalid units
		{"empty name", "", true, "cannot be empty"},
		{"no suffix", "nginx", true, "valid suffix"},
		{"wrong suffix", "nginx.conf", true, "valid suffix"},
		{"forbidden kubelet", "kubelet.service", true, "forbidden"},
		{"forbidden containerd", "containerd.service", true, "forbidden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUnitName(tt.unit)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateUnitName(%q) = nil, want error containing %q", tt.unit, tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateUnitName(%q) error = %v, want error containing %q", tt.unit, err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateUnitName(%q) = %v, want nil", tt.unit, err)
				}
			}
		})
	}
}

func TestValidateFileSpec(t *testing.T) {
	tests := []struct {
		name      string
		spec      mcov1alpha1.FileSpec
		wantError bool
		errMsg    string
	}{
		{
			name: "valid file with content",
			spec: mcov1alpha1.FileSpec{
				Path:    "/etc/test.conf",
				Content: "test content",
				Mode:    420,
				Owner:   "root:root",
				State:   "present",
			},
			wantError: false,
		},
		{
			name: "valid file with default state",
			spec: mcov1alpha1.FileSpec{
				Path:    "/etc/test.conf",
				Content: "test content",
			},
			wantError: false,
		},
		{
			name: "valid absent file",
			spec: mcov1alpha1.FileSpec{
				Path:  "/etc/test.conf",
				State: "absent",
			},
			wantError: false,
		},
		{
			name: "missing content for present",
			spec: mcov1alpha1.FileSpec{
				Path:  "/etc/test.conf",
				State: "present",
			},
			wantError: true,
			errMsg:    "content is required",
		},
		{
			name: "missing content for default state",
			spec: mcov1alpha1.FileSpec{
				Path: "/etc/test.conf",
			},
			wantError: true,
			errMsg:    "content is required",
		},
		{
			name: "forbidden path",
			spec: mcov1alpha1.FileSpec{
				Path:    "/bin/test",
				Content: "test",
			},
			wantError: true,
			errMsg:    "forbidden",
		},
		{
			name: "content too large",
			spec: mcov1alpha1.FileSpec{
				Path:    "/etc/test.conf",
				Content: strings.Repeat("x", MaxFileContentSize+1),
			},
			wantError: true,
			errMsg:    "exceeds maximum size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileSpec(tt.spec)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateFileSpec() = nil, want error containing %q", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateFileSpec() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateFileSpec() = %v, want nil", err)
				}
			}
		})
	}
}

func TestValidateUnitSpec(t *testing.T) {
	tests := []struct {
		name      string
		spec      mcov1alpha1.UnitSpec
		wantError bool
		errMsg    string
	}{
		{
			name: "valid unit",
			spec: mcov1alpha1.UnitSpec{
				Name:  "nginx.service",
				State: "started",
			},
			wantError: false,
		},
		{
			name: "forbidden unit",
			spec: mcov1alpha1.UnitSpec{
				Name: "kubelet.service",
			},
			wantError: true,
			errMsg:    "forbidden",
		},
		{
			name: "invalid suffix",
			spec: mcov1alpha1.UnitSpec{
				Name: "nginx.conf",
			},
			wantError: true,
			errMsg:    "valid suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUnitSpec(tt.spec)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateUnitSpec() = nil, want error containing %q", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateUnitSpec() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateUnitSpec() = %v, want nil", err)
				}
			}
		})
	}
}

func TestValidateMachineConfig(t *testing.T) {
	tests := []struct {
		name      string
		mc        *mcov1alpha1.MachineConfig
		wantError bool
		errMsg    string
	}{
		{
			name:      "nil config",
			mc:        nil,
			wantError: true,
			errMsg:    "cannot be nil",
		},
		{
			name: "valid config with files",
			mc: &mcov1alpha1.MachineConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: mcov1alpha1.MachineConfigSpec{
					Files: []mcov1alpha1.FileSpec{
						{Path: "/etc/test.conf", Content: "test"},
					},
				},
			},
			wantError: false,
		},
		{
			name: "valid config with units",
			mc: &mcov1alpha1.MachineConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: mcov1alpha1.MachineConfigSpec{
					Systemd: mcov1alpha1.SystemdSpec{
						Units: []mcov1alpha1.UnitSpec{
							{Name: "nginx.service", State: "started"},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "invalid file path",
			mc: &mcov1alpha1.MachineConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: mcov1alpha1.MachineConfigSpec{
					Files: []mcov1alpha1.FileSpec{
						{Path: "/bin/bad", Content: "test"},
					},
				},
			},
			wantError: true,
			errMsg:    "files[0]",
		},
		{
			name: "invalid unit",
			mc: &mcov1alpha1.MachineConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: mcov1alpha1.MachineConfigSpec{
					Systemd: mcov1alpha1.SystemdSpec{
						Units: []mcov1alpha1.UnitSpec{
							{Name: "kubelet.service"},
						},
					},
				},
			},
			wantError: true,
			errMsg:    "systemd.units[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMachineConfig(tt.mc)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateMachineConfig() = nil, want error containing %q", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateMachineConfig() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateMachineConfig() = %v, want nil", err)
				}
			}
		})
	}
}

func TestValidateMachineConfigs(t *testing.T) {
	validMC := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "valid"},
		Spec: mcov1alpha1.MachineConfigSpec{
			Files: []mcov1alpha1.FileSpec{
				{Path: "/etc/test.conf", Content: "test"},
			},
		},
	}

	invalidMC := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "invalid"},
		Spec: mcov1alpha1.MachineConfigSpec{
			Files: []mcov1alpha1.FileSpec{
				{Path: "/bin/bad", Content: "test"},
			},
		},
	}

	tests := []struct {
		name      string
		configs   []*mcov1alpha1.MachineConfig
		wantError bool
		errMsg    string
	}{
		{
			name:      "empty list",
			configs:   []*mcov1alpha1.MachineConfig{},
			wantError: false,
		},
		{
			name:      "single valid",
			configs:   []*mcov1alpha1.MachineConfig{validMC},
			wantError: false,
		},
		{
			name:      "multiple valid",
			configs:   []*mcov1alpha1.MachineConfig{validMC, validMC},
			wantError: false,
		},
		{
			name:      "one invalid",
			configs:   []*mcov1alpha1.MachineConfig{invalidMC},
			wantError: true,
			errMsg:    `MachineConfig "invalid"`,
		},
		{
			name:      "valid then invalid",
			configs:   []*mcov1alpha1.MachineConfig{validMC, invalidMC},
			wantError: true,
			errMsg:    `MachineConfig "invalid"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMachineConfigs(tt.configs)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateMachineConfigs() = nil, want error containing %q", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateMachineConfigs() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateMachineConfigs() = %v, want nil", err)
				}
			}
		})
	}
}

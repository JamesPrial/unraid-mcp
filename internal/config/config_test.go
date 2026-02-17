package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testdataDir returns the absolute path to the testdata/config directory.
func testdataDir(t *testing.T) string {
	t.Helper()
	// Navigate from internal/config/ up to project root, then into testdata/config.
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "config"))
	if err != nil {
		t.Fatalf("failed to resolve testdata dir: %v", err)
	}
	return dir
}

// writeTempFile creates a temporary file with the given content and returns its path.
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp file %s: %v", path, err)
	}
	return path
}

func Test_LoadConfig_Cases(t *testing.T) {
	tests := []struct {
		name        string
		setupPath   func(t *testing.T) string
		wantErr     bool
		errContains string
		validate    func(t *testing.T, cfg *Config)
	}{
		{
			name: "valid config loads all fields",
			setupPath: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(testdataDir(t), "valid.yaml")
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg == nil {
					t.Fatal("expected non-nil config")
				}
				// Server
				if cfg.Server.Port != 9090 {
					t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
				}
				if cfg.Server.AuthToken != "test-secret-token" {
					t.Errorf("Server.AuthToken = %q, want %q", cfg.Server.AuthToken, "test-secret-token")
				}
				// Safety - Docker
				wantDockerAllow := []string{"plex", "sonarr"}
				if len(cfg.Safety.Docker.Allowlist) != len(wantDockerAllow) {
					t.Errorf("Safety.Docker.Allowlist = %v, want %v", cfg.Safety.Docker.Allowlist, wantDockerAllow)
				} else {
					for i, v := range wantDockerAllow {
						if cfg.Safety.Docker.Allowlist[i] != v {
							t.Errorf("Safety.Docker.Allowlist[%d] = %q, want %q", i, cfg.Safety.Docker.Allowlist[i], v)
						}
					}
				}
				wantDockerDeny := []string{"unraid-mcp"}
				if len(cfg.Safety.Docker.Denylist) != len(wantDockerDeny) {
					t.Errorf("Safety.Docker.Denylist = %v, want %v", cfg.Safety.Docker.Denylist, wantDockerDeny)
				}
				// Safety - VMs
				if len(cfg.Safety.VMs.Allowlist) != 1 || cfg.Safety.VMs.Allowlist[0] != "windows-vm" {
					t.Errorf("Safety.VMs.Allowlist = %v, want [windows-vm]", cfg.Safety.VMs.Allowlist)
				}
				if len(cfg.Safety.VMs.Denylist) != 1 || cfg.Safety.VMs.Denylist[0] != "macos-vm" {
					t.Errorf("Safety.VMs.Denylist = %v, want [macos-vm]", cfg.Safety.VMs.Denylist)
				}
				// Paths
				if cfg.Paths.Emhttp != "/custom/emhttp" {
					t.Errorf("Paths.Emhttp = %q, want %q", cfg.Paths.Emhttp, "/custom/emhttp")
				}
				if cfg.Paths.Proc != "/custom/proc" {
					t.Errorf("Paths.Proc = %q, want %q", cfg.Paths.Proc, "/custom/proc")
				}
				if cfg.Paths.Sys != "/custom/sys" {
					t.Errorf("Paths.Sys = %q, want %q", cfg.Paths.Sys, "/custom/sys")
				}
				if cfg.Paths.DockerSocket != "/custom/docker.sock" {
					t.Errorf("Paths.DockerSocket = %q, want %q", cfg.Paths.DockerSocket, "/custom/docker.sock")
				}
				if cfg.Paths.LibvirtSocket != "/custom/libvirt-sock" {
					t.Errorf("Paths.LibvirtSocket = %q, want %q", cfg.Paths.LibvirtSocket, "/custom/libvirt-sock")
				}
				// Audit
				if cfg.Audit.Enabled != true {
					t.Errorf("Audit.Enabled = %v, want true", cfg.Audit.Enabled)
				}
				if cfg.Audit.LogPath != "/custom/audit.log" {
					t.Errorf("Audit.LogPath = %q, want %q", cfg.Audit.LogPath, "/custom/audit.log")
				}
				if cfg.Audit.MaxSizeMB != 100 {
					t.Errorf("Audit.MaxSizeMB = %d, want 100", cfg.Audit.MaxSizeMB)
				}
			},
		},
		{
			name: "missing file returns error",
			setupPath: func(t *testing.T) string {
				t.Helper()
				return "/nonexistent/path/config.yaml"
			},
			wantErr:     true,
			errContains: "no such file",
			validate: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg != nil {
					t.Error("expected nil config for missing file")
				}
			},
		},
		{
			name: "invalid YAML returns unmarshal error",
			setupPath: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(testdataDir(t), "invalid.yaml")
			},
			wantErr:     true,
			errContains: "unmarshal",
			validate: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg != nil {
					t.Error("expected nil config for invalid YAML")
				}
			},
		},
		{
			name: "empty file returns config with zero values",
			setupPath: func(t *testing.T) string {
				t.Helper()
				return writeTempFile(t, "empty.yaml", "")
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg == nil {
					t.Fatal("expected non-nil config for empty file")
				}
				if cfg.Server.Port != 0 {
					t.Errorf("Server.Port = %d, want 0 for empty file", cfg.Server.Port)
				}
				if cfg.Server.AuthToken != "" {
					t.Errorf("Server.AuthToken = %q, want empty for empty file", cfg.Server.AuthToken)
				}
				if cfg.Audit.Enabled != false {
					t.Errorf("Audit.Enabled = %v, want false for empty file", cfg.Audit.Enabled)
				}
				if cfg.Paths.Emhttp != "" {
					t.Errorf("Paths.Emhttp = %q, want empty for empty file", cfg.Paths.Emhttp)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setupPath(t)
			cfg, err := LoadConfig(path)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func Test_DefaultConfig_Values(t *testing.T) {
	tests := []struct {
		name     string
		validate func(t *testing.T, cfg *Config)
	}{
		{
			name: "port is 8080",
			validate: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Server.Port != 8080 {
					t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
				}
			},
		},
		{
			name: "audit enabled is true",
			validate: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Audit.Enabled != true {
					t.Errorf("Audit.Enabled = %v, want true", cfg.Audit.Enabled)
				}
			},
		},
		{
			name: "audit log path is /config/audit.log",
			validate: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Audit.LogPath != "/config/audit.log" {
					t.Errorf("Audit.LogPath = %q, want %q", cfg.Audit.LogPath, "/config/audit.log")
				}
			},
		},
		{
			name: "docker socket path",
			validate: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Paths.DockerSocket != "/var/run/docker.sock" {
					t.Errorf("Paths.DockerSocket = %q, want %q", cfg.Paths.DockerSocket, "/var/run/docker.sock")
				}
			},
		},
		{
			name: "libvirt socket path",
			validate: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Paths.LibvirtSocket != "/var/run/libvirt/libvirt-sock" {
					t.Errorf("Paths.LibvirtSocket = %q, want %q", cfg.Paths.LibvirtSocket, "/var/run/libvirt/libvirt-sock")
				}
			},
		},
		{
			name: "emhttp path",
			validate: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Paths.Emhttp != "/host/emhttp" {
					t.Errorf("Paths.Emhttp = %q, want %q", cfg.Paths.Emhttp, "/host/emhttp")
				}
			},
		},
		{
			name: "proc path",
			validate: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Paths.Proc != "/host/proc" {
					t.Errorf("Paths.Proc = %q, want %q", cfg.Paths.Proc, "/host/proc")
				}
			},
		},
		{
			name: "sys path",
			validate: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Paths.Sys != "/host/sys" {
					t.Errorf("Paths.Sys = %q, want %q", cfg.Paths.Sys, "/host/sys")
				}
			},
		},
	}

	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, cfg)
		})
	}
}

func Test_DefaultConfig_ReturnsNewInstance(t *testing.T) {
	cfg1 := DefaultConfig()
	cfg2 := DefaultConfig()

	if cfg1 == cfg2 {
		t.Error("DefaultConfig() should return a new instance each time, got same pointer")
	}
}

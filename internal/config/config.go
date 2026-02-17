// Package config provides configuration loading and defaults for the unraid-mcp server.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ResourceFilter holds allowlist and denylist entries for a resource category.
type ResourceFilter struct {
	Allowlist []string `yaml:"allowlist"`
	Denylist  []string `yaml:"denylist"`
}

// SafetyConfig groups resource filters for Docker containers and VMs.
type SafetyConfig struct {
	Docker ResourceFilter `yaml:"docker"`
	VMs    ResourceFilter `yaml:"vms"`
}

// PathsConfig holds filesystem paths used by the server.
type PathsConfig struct {
	Emhttp        string `yaml:"emhttp"`
	Proc          string `yaml:"proc"`
	Sys           string `yaml:"sys"`
	DockerSocket  string `yaml:"docker_socket"`
	LibvirtSocket string `yaml:"libvirt_socket"`
}

// AuditConfig controls audit logging behaviour.
type AuditConfig struct {
	Enabled   bool   `yaml:"enabled"`
	LogPath   string `yaml:"log_path"`
	MaxSizeMB int    `yaml:"max_size_mb"`
}

// ServerConfig holds network and authentication settings.
type ServerConfig struct {
	Port      int    `yaml:"port"`
	AuthToken string `yaml:"auth_token"`
}

// Config is the top-level configuration structure for the unraid-mcp server.
type Config struct {
	Server ServerConfig `yaml:"server"`
	Safety SafetyConfig `yaml:"safety"`
	Paths  PathsConfig  `yaml:"paths"`
	Audit  AuditConfig  `yaml:"audit"`
}

// LoadConfig reads and parses a YAML configuration file from the given path.
// It returns a pointer to the populated Config and any error encountered.
// On error, nil is returned for the config pointer.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// DefaultConfig returns a new Config populated with sensible default values.
// Each call returns a distinct instance.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8080,
		},
		Paths: PathsConfig{
			Emhttp:        "/host/emhttp",
			Proc:          "/host/proc",
			Sys:           "/host/sys",
			DockerSocket:  "/var/run/docker.sock",
			LibvirtSocket: "/var/run/libvirt/libvirt-sock",
		},
		Audit: AuditConfig{
			Enabled: true,
			LogPath: "/config/audit.log",
		},
	}
}

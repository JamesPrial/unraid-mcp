// Package config provides configuration loading and defaults for the unraid-mcp server.
package config

import (
	"crypto/rand"
	"encoding/hex"
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

// GraphQLConfig holds connection details for the Unraid GraphQL API.
type GraphQLConfig struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
	// Timeout is the HTTP request timeout in seconds.
	Timeout int `yaml:"timeout"`
}

// Config is the top-level configuration structure for the unraid-mcp server.
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Safety  SafetyConfig  `yaml:"safety"`
	Paths   PathsConfig   `yaml:"paths"`
	Audit   AuditConfig   `yaml:"audit"`
	GraphQL GraphQLConfig `yaml:"graphql"`
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
		GraphQL: GraphQLConfig{
			URL:     "http://localhost/graphql",
			Timeout: 30,
		},
	}
}

// ApplyEnvOverrides updates cfg in place with values from environment variables.
// Recognized variables:
//   - UNRAID_MCP_AUTH_TOKEN overrides cfg.Server.AuthToken
//   - UNRAID_GRAPHQL_URL overrides cfg.GraphQL.URL
//   - UNRAID_GRAPHQL_API_KEY overrides cfg.GraphQL.APIKey
func ApplyEnvOverrides(cfg *Config) {
	if token := os.Getenv("UNRAID_MCP_AUTH_TOKEN"); token != "" {
		cfg.Server.AuthToken = token
	}
	if url := os.Getenv("UNRAID_GRAPHQL_URL"); url != "" {
		cfg.GraphQL.URL = url
	}
	if key := os.Getenv("UNRAID_GRAPHQL_API_KEY"); key != "" {
		cfg.GraphQL.APIKey = key
	}
}

// EnsureAuthToken generates a random auth token and sets it on cfg if
// cfg.Server.AuthToken is empty. It returns the token (existing or generated)
// and any error encountered during generation.
func EnsureAuthToken(cfg *Config) (string, error) {
	if cfg.Server.AuthToken != "" {
		return cfg.Server.AuthToken, nil
	}
	token, err := GenerateRandomToken()
	if err != nil {
		return "", fmt.Errorf("generate auth token: %w", err)
	}
	cfg.Server.AuthToken = token
	return token, nil
}

// GenerateRandomToken returns a 32-character hex-encoded cryptographically
// random token string.
func GenerateRandomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand.Read: %w", err)
	}
	return hex.EncodeToString(b), nil
}

package config

import (
	"encoding/hex"
	"os"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// ApplyEnvOverrides
// ---------------------------------------------------------------------------

func Test_ApplyEnvOverrides_Cases(t *testing.T) {
	tests := []struct {
		name         string
		envSet       bool   // whether UNRAID_MCP_AUTH_TOKEN should be present
		envValue     string // value when envSet is true
		initialToken string
		initialPort  int
		initialPaths PathsConfig
		wantToken    string
		wantPort     int
		wantPaths    PathsConfig
	}{
		{
			name:         "token env set on empty config",
			envSet:       true,
			envValue:     "my-token",
			initialToken: "",
			wantToken:    "my-token",
		},
		{
			name:         "token env overrides existing token",
			envSet:       true,
			envValue:     "new",
			initialToken: "old",
			wantToken:    "new",
		},
		{
			name:         "token env not set preserves existing token",
			envSet:       false,
			initialToken: "existing",
			wantToken:    "existing",
		},
		{
			name:         "empty env does not override existing token",
			envSet:       true,
			envValue:     "",
			initialToken: "existing",
			wantToken:    "existing",
		},
		{
			name:         "other fields unchanged when env is set",
			envSet:       true,
			envValue:     "token",
			initialToken: "",
			initialPort:  9090,
			initialPaths: PathsConfig{
				Emhttp:        "/custom/emhttp",
				Proc:          "/custom/proc",
				Sys:           "/custom/sys",
				DockerSocket:  "/custom/docker.sock",
				LibvirtSocket: "/custom/libvirt-sock",
			},
			wantToken: "token",
			wantPort:  9090,
			wantPaths: PathsConfig{
				Emhttp:        "/custom/emhttp",
				Proc:          "/custom/proc",
				Sys:           "/custom/sys",
				DockerSocket:  "/custom/docker.sock",
				LibvirtSocket: "/custom/libvirt-sock",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envSet {
				t.Setenv("UNRAID_MCP_AUTH_TOKEN", tt.envValue)
			} else {
				// Register cleanup via t.Setenv, then immediately remove
				// the variable so os.LookupEnv returns (_, false).
				t.Setenv("UNRAID_MCP_AUTH_TOKEN", "")
				os.Unsetenv("UNRAID_MCP_AUTH_TOKEN")
			}

			cfg := &Config{
				Server: ServerConfig{
					Port:      tt.initialPort,
					AuthToken: tt.initialToken,
				},
				Paths: tt.initialPaths,
			}

			ApplyEnvOverrides(cfg)

			if cfg.Server.AuthToken != tt.wantToken {
				t.Errorf("AuthToken = %q, want %q", cfg.Server.AuthToken, tt.wantToken)
			}
			if tt.wantPort != 0 && cfg.Server.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", cfg.Server.Port, tt.wantPort)
			}
			if tt.wantPaths != (PathsConfig{}) && cfg.Paths != tt.wantPaths {
				t.Errorf("Paths = %+v, want %+v", cfg.Paths, tt.wantPaths)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// EnsureAuthToken
// ---------------------------------------------------------------------------

func Test_EnsureAuthToken_Cases(t *testing.T) {
	t.Run("token already set returns existing token unchanged", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				AuthToken: "pre-set",
			},
		}

		token, err := EnsureAuthToken(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token != "pre-set" {
			t.Errorf("returned token = %q, want %q", token, "pre-set")
		}
		if cfg.Server.AuthToken != "pre-set" {
			t.Errorf("cfg.Server.AuthToken = %q, want %q", cfg.Server.AuthToken, "pre-set")
		}
	})

	t.Run("empty token generates and sets new token", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				AuthToken: "",
			},
		}

		token, err := EnsureAuthToken(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token == "" {
			t.Fatal("returned token is empty, expected a generated value")
		}
		if cfg.Server.AuthToken != token {
			t.Errorf("cfg.Server.AuthToken = %q, want %q (returned token)", cfg.Server.AuthToken, token)
		}
	})

	t.Run("generated token is 32 characters", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				AuthToken: "",
			},
		}

		token, err := EnsureAuthToken(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(token) != 32 {
			t.Errorf("len(token) = %d, want 32", len(token))
		}
	})

	t.Run("generated token is valid hex", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				AuthToken: "",
			},
		}

		token, err := EnsureAuthToken(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		decoded, err := hex.DecodeString(token)
		if err != nil {
			t.Fatalf("token %q is not valid hex: %v", token, err)
		}
		if len(decoded) != 16 {
			t.Errorf("decoded length = %d, want 16 bytes", len(decoded))
		}
	})

	t.Run("two calls produce different tokens", func(t *testing.T) {
		cfg1 := &Config{Server: ServerConfig{AuthToken: ""}}
		cfg2 := &Config{Server: ServerConfig{AuthToken: ""}}

		token1, err := EnsureAuthToken(cfg1)
		if err != nil {
			t.Fatalf("first call error: %v", err)
		}

		token2, err := EnsureAuthToken(cfg2)
		if err != nil {
			t.Fatalf("second call error: %v", err)
		}

		if token1 == token2 {
			t.Errorf("two generated tokens are identical: %q", token1)
		}
	})
}

// ---------------------------------------------------------------------------
// GenerateRandomToken
// ---------------------------------------------------------------------------

func Test_GenerateRandomToken_Cases(t *testing.T) {
	t.Run("returns 32 character string", func(t *testing.T) {
		token, err := GenerateRandomToken()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(token) != 32 {
			t.Errorf("len(token) = %d, want 32", len(token))
		}
	})

	t.Run("output is valid hex encoding 16 bytes", func(t *testing.T) {
		token, err := GenerateRandomToken()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		decoded, err := hex.DecodeString(token)
		if err != nil {
			t.Fatalf("token %q is not valid hex: %v", token, err)
		}
		if len(decoded) != 16 {
			t.Errorf("decoded byte length = %d, want 16", len(decoded))
		}
	})

	t.Run("two calls return different values", func(t *testing.T) {
		token1, err := GenerateRandomToken()
		if err != nil {
			t.Fatalf("first call error: %v", err)
		}

		token2, err := GenerateRandomToken()
		if err != nil {
			t.Fatalf("second call error: %v", err)
		}

		if token1 == token2 {
			t.Errorf("two generated tokens are identical: %q", token1)
		}
	})

	t.Run("concurrent calls all succeed with unique tokens", func(t *testing.T) {
		const goroutines = 100

		var (
			wg     sync.WaitGroup
			mu     sync.Mutex
			tokens = make(map[string]struct{}, goroutines)
			errs   []error
		)

		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				token, err := GenerateRandomToken()
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs = append(errs, err)
					return
				}
				tokens[token] = struct{}{}
			}()
		}
		wg.Wait()

		if len(errs) > 0 {
			t.Fatalf("got %d errors in concurrent calls; first: %v", len(errs), errs[0])
		}

		if len(tokens) != goroutines {
			t.Errorf("expected %d unique tokens, got %d (collisions detected)", goroutines, len(tokens))
		}
	})
}

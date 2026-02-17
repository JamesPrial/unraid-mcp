// Package main is the entry point for the unraid-mcp server.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jamesprial/unraid-mcp/internal/auth"
	"github.com/jamesprial/unraid-mcp/internal/config"
	"github.com/jamesprial/unraid-mcp/internal/docker"
	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/system"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/jamesprial/unraid-mcp/internal/vm"
	"github.com/mark3labs/mcp-go/server"
)

const defaultConfigPath = "/config/config.yaml"

func main() {
	cfg := loadConfig()
	config.ApplyEnvOverrides(cfg)

	tokenBefore := cfg.Server.AuthToken
	token, err := config.EnsureAuthToken(cfg)
	if err != nil {
		log.Printf("warning: could not generate auth token: %v — running without authentication", err)
	} else if tokenBefore == "" {
		log.Printf("generated auth token (set UNRAID_MCP_AUTH_TOKEN to persist): %s", token)
	}

	// Open audit log writer if enabled.
	var auditLogger *safety.AuditLogger
	if cfg.Audit.Enabled {
		f, err := os.OpenFile(cfg.Audit.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			log.Printf("warning: could not open audit log %q: %v — audit logging disabled", cfg.Audit.LogPath, err)
		} else {
			auditLogger = safety.NewAuditLogger(f)
			defer f.Close()
		}
	}

	// Build safety components.
	dockerFilter := safety.NewFilter(
		cfg.Safety.Docker.Allowlist,
		cfg.Safety.Docker.Denylist,
	)
	vmFilter := safety.NewFilter(
		cfg.Safety.VMs.Allowlist,
		cfg.Safety.VMs.Denylist,
	)

	dockerConfirm := safety.NewConfirmationTracker(docker.DestructiveTools)
	vmConfirm := safety.NewConfirmationTracker(vm.DestructiveTools)

	// Build resource managers.
	dockerMgr, err := docker.NewDockerClientManager(cfg.Paths.DockerSocket)
	if err != nil {
		log.Fatalf("failed to create Docker manager: %v", err)
	}

	// VM manager: attempt real libvirt connection; fall back gracefully if
	// the libvirt build tag is absent or the socket is unavailable.
	var vmMgr vm.VMManager
	if rawVMMgr, vmErr := vm.NewLibvirtVMManager(cfg.Paths.LibvirtSocket); vmErr != nil {
		log.Printf("warning: VM manager unavailable (%v) — VM tools will not be registered", vmErr)
	} else {
		vmMgr = rawVMMgr
	}

	systemMon := system.NewFileSystemMonitor(
		cfg.Paths.Proc,
		cfg.Paths.Sys,
		cfg.Paths.Emhttp,
	)

	// Build MCP server.
	mcpServer := server.NewMCPServer(
		"unraid-mcp",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	// Register all tools.
	var registrations []tools.Registration
	registrations = append(registrations, docker.DockerTools(dockerMgr, dockerFilter, dockerConfirm, auditLogger)...)

	if vmMgr != nil {
		registrations = append(registrations, vm.VMTools(vmMgr, vmFilter, vmConfirm, auditLogger)...)
	}

	registrations = append(registrations, system.SystemTools(systemMon, auditLogger)...)

	tools.RegisterAll(mcpServer, registrations)

	// Build Streamable HTTP server and wrap with auth middleware.
	httpHandler := server.NewStreamableHTTPServer(mcpServer)
	authMiddleware := auth.NewAuthMiddleware(cfg.Server.AuthToken)
	wrappedHandler := authMiddleware(httpHandler)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           wrappedHandler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown on SIGINT / SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("unraid-mcp listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-stop
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
	log.Println("server stopped")
}

// loadConfig attempts to read the config file from the path specified by
// UNRAID_MCP_CONFIG_PATH or the default /config/config.yaml. If the file
// cannot be read, DefaultConfig is returned.
func loadConfig() *config.Config {
	path := os.Getenv("UNRAID_MCP_CONFIG_PATH")
	if path == "" {
		path = defaultConfigPath
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		log.Printf("could not load config from %q (%v), using defaults", path, err)
		return config.DefaultConfig()
	}

	log.Printf("loaded config from %q", path)
	return cfg
}



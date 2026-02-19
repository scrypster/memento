// cmd/memento-mcp is the entry point for the Memento MCP (Model Context Protocol)
// server.  It wires the SQLite storage backend through the MemoryEngine so that
// all memories stored via MCP flow through enrichment and decay tracking.
//
// Startup sequence:
//  1. Load configuration from environment variables.
//  2. Open the SQLite database and apply pending migrations.
//  3. Create a MemoryEngine wrapping the store.
//  4. Start the decay-score updater as a background goroutine.
//  5. Create the MCP server, injecting the engine's store.
//  6. Serve JSON-RPC 2.0 requests from stdin, writing responses to stdout.
//
// CRITICAL: ALL logging MUST go to stderr.  Any bytes written to stdout that
// are not valid JSON-RPC 2.0 response frames will corrupt the protocol.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/scrypster/memento/internal/api/mcp"
	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/storage/sqlite"
)

// resolveConnectionsConfig finds the connections.json path, trying multiple locations.
// Returns an empty string if no config file is found.
func resolveConnectionsConfig() string {
	// 1. Explicit env var (highest priority — set by downloaded integration configs)
	if path := os.Getenv("MEMENTO_CONNECTIONS_CONFIG"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		log.Printf("MEMENTO_CONNECTIONS_CONFIG=%s does not exist, continuing search", path)
	}

	// 2. Next to the executable (typical installed layout: memento-mcp + config/ in same dir)
	if execPath, err := os.Executable(); err == nil {
		candidate := fmt.Sprintf("%s/config/connections.json", filepath.Dir(execPath))
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 3. CWD fallback (legacy: running from project root)
	if _, err := os.Stat("config/connections.json"); err == nil {
		return "config/connections.json"
	}

	return ""
}

func main() {
	// Redirect the default logger to stderr so that any incidental log calls
	// (e.g. from imported packages) never pollute the stdout JSON-RPC stream.
	log.SetOutput(os.Stderr)
	log.SetPrefix("memento-mcp: ")
	log.SetFlags(log.LstdFlags)

	// Load configuration from environment variables.
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Ensure the data directory exists.
	if err := os.MkdirAll(cfg.Storage.DataPath, 0o700); err != nil {
		log.Fatalf("failed to create data directory %q: %v", cfg.Storage.DataPath, err)
	}

	// Open the SQLite database.
	dbPath := fmt.Sprintf("%s/memento.db", cfg.Storage.DataPath)
	store, err := sqlite.NewMemoryStore(dbPath)
	if err != nil {
		log.Fatalf("failed to open database at %q: %v", dbPath, err)
	}
	defer store.Close()

	// Set up a root context that is cancelled on SIGINT / SIGTERM.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("received shutdown signal")
		cancel()
	}()

	// Wrap the raw store in the MemoryEngine so that memories stored via MCP
	// flow through the enrichment and decay pipeline.
	memEngine, err := engine.NewMemoryEngine(store, engine.DefaultConfig(), cfg)
	if err != nil {
		log.Fatalf("failed to create memory engine: %v", err)
	}
	if err := memEngine.Start(ctx); err != nil {
		log.Fatalf("failed to start memory engine: %v", err)
	}
	defer func() {
		if err := memEngine.Shutdown(ctx); err != nil {
			log.Printf("engine shutdown error: %v", err)
		}
	}()

	// Load connections config so the MCP server can route connection_id to
	// the right store and so that memory IDs get the correct domain segment.
	//
	// Priority order for finding connections.json:
	//   1. MEMENTO_CONNECTIONS_CONFIG env var (absolute path, set by integration configs)
	//   2. config/connections.json relative to the executable's directory
	//   3. config/connections.json relative to CWD (legacy fallback)
	var connManager *connections.Manager
	connectionsConfigPath := resolveConnectionsConfig()
	if connectionsConfigPath != "" {
		if cm, err := connections.NewManager(connectionsConfigPath); err == nil {
			connManager = cm
			log.Printf("loaded connections config from %s", connectionsConfigPath)
		} else {
			log.Printf("warning: failed to load connections config from %s: %v", connectionsConfigPath, err)
		}
	}
	if connManager == nil {
		log.Printf("using single-store mode with MEMENTO_DATA_PATH=%s", cfg.Storage.DataPath)
		connManager = connections.NewManagerWithStore(store, "default")
	}

	// Read optional default connection from env.
	// MEMENTO_DEFAULT_CONNECTION pins the connection used when no connection_id
	// is passed to any MCP tool call.  Useful for global or per-project defaults.
	defaultConn := os.Getenv("MEMENTO_DEFAULT_CONNECTION")
	if defaultConn != "" {
		log.Printf("default connection: %s", defaultConn)
	}

	// Create the MCP server, injecting the store so that memories
	// stored via MCP are accessible to the enrichment pipeline.
	// Also inject the engine so store_memory and retry_enrichment can queue
	// jobs immediately rather than waiting for the next startup recovery.
	srvOpts := []mcp.ServerOption{
		mcp.WithConfig(cfg),
		mcp.WithConnectionManager(connManager),
		mcp.WithEngine(memEngine),
	}
	if defaultConn != "" {
		srvOpts = append(srvOpts, mcp.WithDefaultConnection(defaultConn))
	}
	srv := mcp.NewServer(store, srvOpts...)

	// Wrap the server in a StdioTransport that reads line-delimited JSON-RPC
	// from stdin and writes responses to stdout.  All logging inside the
	// transport is directed to stderr.
	transport := mcp.NewStdioTransport(srv, os.Stdin, os.Stdout)

	log.Println("ready — serving JSON-RPC 2.0 on stdin/stdout")

	if err := transport.Serve(ctx); err != nil {
		// A non-nil error here is normal (context cancellation) or indicates a
		// fatal stdin/stdout problem.  Either way it is informational only.
		log.Printf("transport stopped: %v", err)
	}
}

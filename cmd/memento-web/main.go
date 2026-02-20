package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/notify"
	"github.com/scrypster/memento/internal/server"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/sqlite"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to connections config file (default: config/connections.json)")
	flag.Parse()

	// If no config path specified, use default if it exists
	if *configPath == "" {
		defaultPath := "config/connections.json"
		if _, err := os.Stat(defaultPath); err == nil {
			*configPath = defaultPath
			log.Printf("Using connections config: %s", defaultPath)
		}
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize storage
	store, err := sqlite.NewMemoryStore(cfg.Storage.DataPath + "/memento.db")
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	// Setup context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize memory engine for enrichment
	engineCfg := engine.DefaultConfig()
	engineCfg.NumWorkers = 1 // Use 1 worker for SQLite to avoid database locking
	memoryEngine, err := engine.NewMemoryEngine(store, engineCfg, cfg)
	if err != nil {
		log.Fatalf("Failed to initialize memory engine: %v", err)
	}

	// Start enrichment workers
	if err := memoryEngine.Start(ctx); err != nil {
		log.Fatalf("Failed to start memory engine: %v", err)
	}

	// Start server (pass memory engine for queue size reporting and optional config path)
	addr, wsHub := server.Start(ctx, cfg, store, memoryEngine, *configPath)
	log.Printf("Memento Web UI running at http://%s", addr)

	// Broadcast a lifecycle event over WebSocket
	broadcastEvent := func(eventType, memoryID string) {
		wsHub.Broadcast(map[string]interface{}{
			"type":     eventType,
			"memoryId": memoryID,
		})
	}

	// Local enrichments (web's own engine)
	memoryEngine.SetOnMemoryCreated(func(memoryID string) {
		broadcastEvent("memory_created", memoryID)
	})
	memoryEngine.SetOnEnrichmentStarted(func(memoryID string) {
		broadcastEvent("enrichment_started", memoryID)
	})
	memoryEngine.SetOnEnrichmentComplete(func(memoryID string) {
		broadcastEvent("enrichment_complete", memoryID)
	})

	// Cross-process events (from memento-mcp via filesystem events)
	eventWatcher := notify.NewEventWatcher(cfg.Storage.DataPath, broadcastEvent)
	if err := eventWatcher.Start(); err != nil {
		log.Printf("WARNING: cross-process notifications disabled: %v", err)
	}
	defer eventWatcher.Stop()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down gracefully...")

	// Shutdown enrichment workers first
	if err := memoryEngine.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down memory engine: %v", err)
	}

	cancel()
	time.Sleep(1 * time.Second) // Give time for connections to close
}

// startServer is a helper that wraps server.Start for testability.
// It accepts a storage.MemoryStore interface so tests can pass in any implementation.
func startServer(ctx context.Context, cfg *config.Config, store storage.MemoryStore) string {
	addr, _ := server.Start(ctx, cfg, store)
	return addr
}

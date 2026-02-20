// Package server provides HTTP server initialization and lifecycle management
// for the Memento Web UI.
package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/connections"
	"github.com/scrypster/memento/internal/services"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/web/handlers"
)

// dbGetter interface for stores that expose their database connection
type dbGetter interface {
	GetDB() *sql.DB
}

// securityHeadersMiddleware adds security headers to all HTTP responses.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// Start initializes and starts the HTTP server.
// Returns the actual address being listened on (useful for testing with port 0)
// and the WebSocketHub for wiring enrichment event broadcasts.
// The queueGetter parameter is optional (may be nil) and is used to expose the
// enrichment queue depth in /api/stats.
// The connectionsConfigPath parameter is optional and points to a connections.json file
// for loading multiple connections; if empty, only the default connection is available.
func Start(ctx context.Context, cfg *config.Config, store storage.MemoryStore, queueGetter ...interface{}) (string, *handlers.WebSocketHub) {
	var qg handlers.QueueSizeGetter
	var connectionsConfigPath string
	var maintenanceEng handlers.MaintenanceEngine

	// Parse variadic arguments
	for _, arg := range queueGetter {
		switch v := arg.(type) {
		case handlers.MaintenanceEngine:
			// MaintenanceEngine is a superset; also satisfies QueueSizeGetter
			maintenanceEng = v
			if qg == nil {
				qg = v
			}
		case handlers.QueueSizeGetter:
			qg = v
		case string:
			connectionsConfigPath = v
		}
	}
	mux := http.NewServeMux()

	// Create WebSocket hub
	wsHub := handlers.NewWebSocketHub()
	go wsHub.Run()

	// Create rate limiter (10 req/sec, burst of 20)
	rateLimiter := handlers.NewRateLimiter(10.0, 20)

	// Build a connections manager
	// If a connections config path is provided, load it; otherwise use default
	var connManager *connections.Manager
	if connectionsConfigPath != "" {
		var err error
		connManager, err = connections.NewManager(connectionsConfigPath)
		if err != nil {
			log.Printf("Warning: failed to load connections config: %v, falling back to default", err)
			connManager = connections.NewManagerWithStore(store, "default")
		}
	} else {
		// Build a single-store connections manager so that the stats and search
		// handlers can resolve connections by name (or use the default).
		connManager = connections.NewManagerWithStore(store, "default")
	}

	// Create API handlers with database connection and connection manager for multi-connection support
	// Try to get database connection from store if available
	var apiHandlers *handlers.APIHandlers
	var db *sql.DB
	if dbStore, ok := store.(dbGetter); ok {
		db = dbStore.GetDB()
	}

	if db != nil {
		apiHandlers = handlers.NewAPIHandlersWithConnManager(store, cfg, db, connManager)
	} else {
		apiHandlers = handlers.NewAPIHandlersWithDB(store, cfg, nil)
		apiHandlers.SetConnectionManager(connManager)
	}

	// Create connection handlers for API routes
	connectionHandlers := handlers.NewConnectionHandlers(connManager)

	// Create stats handler
	statsHandler := handlers.NewStatsHandler(store, connManager, qg)

	// Create queue handler
	queueHandler := handlers.NewQueueHandler(connManager, qg)

	// Create search handler
	searchHandler := handlers.NewSearchHandler(store, connManager)

	// Create activity handler (memory creation time series)
	activityHandler := handlers.NewActivityHandler(store, connManager)

	// Create LLM models handler (model discovery and testing)
	llmModelsHandler := handlers.NewLLMModelHandlers()

	// Create entity handler (entities, relationships, graph API)
	entityHandler := handlers.NewEntityHandler(store, connManager)

	// Create import handler (Obsidian / Markdown import)
	importHandler := handlers.NewImportHandlers(store)

	// Create maintenance handler for per-connection backfill operations
	maintenanceHandler := handlers.NewMaintenanceHandler(connManager, maintenanceEng)

	// Create settings handler for per-connection taxonomy customization
	var settingsHandlers *handlers.SettingsHandlers
	if db != nil {
		settingsService := services.NewSettingsService(db)
		settingsHandlers = handlers.NewSettingsHandlers(settingsService)
	}

	// integrationsHandler is created after basePath is resolved (see below)

	// API routes (require auth in production mode)
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/memories", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			apiHandlers.ListMemories(w, r)
		case http.MethodPost:
			apiHandlers.CreateMemory(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/memories/{id}", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			apiHandlers.GetMemory(w, r)
		case http.MethodPatch:
			apiHandlers.UpdateMemory(w, r)
		case http.MethodDelete:
			apiHandlers.DeleteMemory(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/memories/{id}/retry", maintenanceHandler.RetryEnrichment)
	apiMux.HandleFunc("/api/stats", statsHandler.GetStats)
	apiMux.HandleFunc("/api/activity", activityHandler.GetActivity)
	apiMux.HandleFunc("/api/queue", queueHandler.GetQueue)
	apiMux.HandleFunc("/api/search", searchHandler.Search)
	apiMux.HandleFunc("/api/config/user", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			apiHandlers.GetUserConfig(w, r)
		case http.MethodPost:
			apiHandlers.PostUserConfig(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Import routes (Tier 1.2: Obsidian/Markdown Import)
	apiMux.HandleFunc("/api/import/obsidian", importHandler.PostObsidianImport)
	apiMux.HandleFunc("/api/import/markdown", importHandler.PostMarkdownImport)
	apiMux.HandleFunc("/api/import/status/{job_id}", importHandler.GetImportStatus)

	// Entity and relationship routes
	apiMux.HandleFunc("/api/entities", entityHandler.ListEntities)
	apiMux.HandleFunc("/api/entities/{id}/graph", entityHandler.GetEntityGraph)
	apiMux.HandleFunc("/api/relationships", entityHandler.ListRelationships)

	// Connection management routes
	apiMux.HandleFunc("/api/connections", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			connectionHandlers.ListConnections(w, r)
		case http.MethodPost:
			connectionHandlers.CreateConnection(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/connections/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			connectionHandlers.TestConnection(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/connections/default", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			connectionHandlers.SetDefaultConnection(w, r)
		case http.MethodPut:
			connectionHandlers.UpdateConnection(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/connections/{name}", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			connectionHandlers.UpdateConnection(w, r)
		case http.MethodDelete:
			connectionHandlers.DeleteConnection(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Maintenance routes (per-connection enrichment and embedding backfill)
	apiMux.HandleFunc("/api/connections/{name}/maintenance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			maintenanceHandler.GetStatus(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/connections/{name}/maintenance/backfill", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			maintenanceHandler.RunBackfill(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/connections/{name}/maintenance/unknown-types", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			maintenanceHandler.GetUnknownTypes(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Connection settings routes (per-connection taxonomy customization)
	if settingsHandlers != nil {
		apiMux.HandleFunc("/api/connections/{id}/settings", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				settingsHandlers.GetSettings(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
		apiMux.HandleFunc("/api/connections/{id}/settings/entities", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				settingsHandlers.UpdateEntityTypes(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
		apiMux.HandleFunc("/api/connections/{id}/settings/relationships", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				settingsHandlers.UpdateRelationshipTypes(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
		apiMux.HandleFunc("/api/connections/{id}/settings/memory-types", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				settingsHandlers.UpdateMemoryTypes(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
		apiMux.HandleFunc("/api/connections/{id}/settings/classifications", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				settingsHandlers.UpdateClassifications(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
		apiMux.HandleFunc("/api/connections/{id}/settings/active-category", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				settingsHandlers.UpdateActiveCategory(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
	}

	// LLM model discovery and testing routes
	apiMux.HandleFunc("/api/llm/available-models", llmModelsHandler.GetAvailableModels)
	apiMux.HandleFunc("/api/llm/test-connection", llmModelsHandler.TestConnection)

	// Integration routes
	// TODO: Uncomment once NewIntegrationsHandlers is implemented
	/*
	apiMux.HandleFunc("/api/integrations/systems", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			integrationsHandler.ListSupportedSystems(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/integrations/{system}/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			integrationsHandler.GetIntegrationConfig(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/integrations/test-connection", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			integrationsHandler.TestConnection(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/integrations/install", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			integrationsHandler.InstallConfig(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	*/

	// Determine base path for static assets / templates
	basePath := findBasePath()

	// Debug handler (needs basePath for template serving)
	debugHandler := handlers.NewDebugHandler(store, basePath)
	apiMux.HandleFunc("/api/debug/recall-trace", debugHandler.RecallTrace)

	// Integrations handler — config downloads and connection test
	integrationsHandler := handlers.NewIntegrationsHandler(cfg, basePath)
	apiMux.HandleFunc("/api/integrations/systems", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			integrationsHandler.ListSupportedSystems(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/integrations/{system}/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			integrationsHandler.GetConfig(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/integrations/test-connection", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			integrationsHandler.TestConnection(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Health endpoint — no auth required, used by integrations and monitoring
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","version":"1.0.0"}`))
	})

	// Wrap API routes with auth middleware
	mux.Handle("/api/", handlers.RequireAuth(apiMux, cfg))

	// Debug UI page (served outside the auth-required API prefix)
	mux.HandleFunc("/debug", debugHandler.DebugPage)

	// WebSocket endpoint (no auth required - origin validation handles security)
	mux.Handle("/ws", wsHub)

	// Static files
	fs := http.FileServer(http.Dir(basePath + "/web/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Index page
	indexPath := basePath + "/web/templates/index.html"
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, indexPath)
	})

	// Import page
	importPath := basePath + "/web/templates/import.html"
	mux.HandleFunc("/import", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, importPath)
	})

	// Integrations page
	integrationsPath := basePath + "/web/templates/integrations.html"
	mux.HandleFunc("/integrations", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, integrationsPath)
	})

	// Wrap entire server with rate limiting, then security headers
	handler := handlers.RateLimitMiddleware(mux, rateLimiter)
	handler = securityHeadersMiddleware(handler)

	// Create server with security timeouts
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}

	actualAddr := listener.Addr().String()

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
		wsHub.Stop()
	}()

	return actualAddr, wsHub
}

// findBasePath returns the base path for the project.
// When running from cmd/memento-web, we need to go up two directories.
// When running tests, we may already be in the project root.
func findBasePath() string {
	// Try current directory first (for when running from project root)
	if _, err := os.Stat("web/templates/index.html"); err == nil {
		return "."
	}

	// Try parent directory (for when running from cmd/)
	if _, err := os.Stat("../web/templates/index.html"); err == nil {
		return ".."
	}

	// Try two levels up (for when running from cmd/memento-web/)
	if _, err := os.Stat("../../web/templates/index.html"); err == nil {
		return "../.."
	}

	// Default to current directory
	return "."
}

package connections

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/internal/storage/postgres"
	"github.com/scrypster/memento/internal/storage/sqlite"
)

// sanitizeDSN replaces the password in a DSN string with [REDACTED] for safe logging.
// Handles both postgres://user:pass@host/db and user=x password=y host=z formats.
func sanitizeDSN(dsn string) string {
	// Handle URL format: postgres://user:password@host/db
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err == nil && u.User != nil {
			if _, hasPassword := u.User.Password(); hasPassword {
				u.User = url.UserPassword(u.User.Username(), "[REDACTED]")
				return u.String()
			}
		}
	}
	// Handle key=value format: password=xxx or sslpassword=xxx
	re := regexp.MustCompile(`(password\s*=\s*)\S+`)
	return re.ReplaceAllString(dsn, "${1}[REDACTED]")
}

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	Type     string `json:"type"`               // sqlite, postgresql
	Path     string `json:"path,omitempty"`     // For SQLite
	Host     string `json:"host,omitempty"`     // For PostgreSQL
	Port     int    `json:"port,omitempty"`     // For PostgreSQL
	Username string `json:"username,omitempty"` // For PostgreSQL
	Password string `json:"password,omitempty"` // For PostgreSQL
	Database string `json:"database,omitempty"` // For PostgreSQL
	SSLMode  string `json:"sslmode,omitempty"`  // For PostgreSQL
}

// LLMConfig holds LLM configuration per connection
type LLMConfig struct {
	Provider       string `json:"provider"`                        // ollama, openai, anthropic
	Model          string `json:"model"`                           // Model name
	APIKey         string `json:"api_key,omitempty"`               // For cloud providers
	BaseURL        string `json:"base_url,omitempty"`              // Custom base URL (Ollama/custom endpoints)
	EmbeddingModel string `json:"embedding_model,omitempty"`       // Model name for embeddings
}

// Connection represents a workspace/project connection configuration
type Connection struct {
	Name             string          `json:"name"`
	DisplayName      string          `json:"display_name"`
	Description      string          `json:"description"`
	Enabled          bool            `json:"enabled"`
	CreatedAt        string          `json:"created_at"`
	Database         DatabaseConfig  `json:"database"`
	LLM              LLMConfig       `json:"llm"`
	CategoryTemplate string          `json:"category_template,omitempty"`
	Categories       []string        `json:"categories,omitempty"`
}

// ConnectionsConfig holds the connections configuration
type ConnectionsConfig struct {
	DefaultConnection string       `json:"default_connection"`
	Connections       []Connection `json:"connections"`
	Settings          struct {
		AutoCreateDefault bool `json:"auto_create_default"`
		MaxConnections    int  `json:"max_connections"`
		AllowUserCreate   bool `json:"allow_user_create"`
	} `json:"settings"`
}

// Manager manages multiple database connections
type Manager struct {
	config      *ConnectionsConfig
	stores      map[string]storage.MemoryStore
	storesLock  sync.RWMutex
	configPath  string
	baseDir     string // Directory used to resolve relative paths in the config
	ownedStores map[string]bool // Track which stores are owned vs borrowed
}

// NewManagerWithStore creates a Manager that wraps a single pre-existing store.
// The store is registered under the given connectionName and set as the default.
// This is used when the server is started with a store that was opened by the
// caller (e.g. in cmd/memento-web) rather than via a connections config file.
// The store is marked as "borrowed" and will NOT be closed by the manager.
func NewManagerWithStore(store storage.MemoryStore, connectionName string) *Manager {
	manager := &Manager{
		stores: map[string]storage.MemoryStore{
			connectionName: store,
		},
		ownedStores: map[string]bool{
			connectionName: false, // Borrowed from caller, don't close
		},
		config: &ConnectionsConfig{
			DefaultConnection: connectionName,
			Connections: []Connection{
				{
					Name:    connectionName,
					Enabled: true,
				},
			},
		},
	}
	return manager
}

// NewManager creates a new connection manager.
// configPath should be an absolute path so that relative database paths inside
// the config file can be resolved correctly regardless of the working directory.
func NewManager(configPath string) (*Manager, error) {
	// Resolve to absolute so baseDir is always correct.
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		absPath = configPath
	}

	manager := &Manager{
		stores:      make(map[string]storage.MemoryStore),
		ownedStores: make(map[string]bool),
		configPath:  absPath,
		// Relative paths inside connections.json are resolved from the directory
		// that *contains* the config file (e.g. the project root when connections.json
		// lives at <root>/config/connections.json and paths are like "../data/...").
		// We use the directory of the config file itself; callers should ensure
		// database paths in the config are relative to that directory or absolute.
		baseDir: filepath.Dir(absPath),
	}

	if err := manager.LoadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load connections config: %w", err)
	}

	return manager, nil
}

// LoadConfig loads the connections configuration from file
func (m *Manager) LoadConfig() error {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config ConnectionsConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	m.config = &config
	return nil
}

// SaveConfig saves the connections configuration to file
// For single-store managers (no config path), this is a no-op since there's no file to save
func (m *Manager) SaveConfig() error {
	// If no config path is set (single-store mode), don't try to save
	if m.configPath == "" {
		return nil
	}

	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(m.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetStore returns the MemoryStore for a given connection name
func (m *Manager) GetStore(connectionName string) (storage.MemoryStore, error) {
	// Use default if empty
	if connectionName == "" {
		connectionName = m.config.DefaultConnection
	}

	// Check cache first
	m.storesLock.RLock()
	if store, exists := m.stores[connectionName]; exists {
		m.storesLock.RUnlock()
		return store, nil
	}
	m.storesLock.RUnlock()

	// Find connection config
	var conn *Connection
	for i := range m.config.Connections {
		if m.config.Connections[i].Name == connectionName {
			conn = &m.config.Connections[i]
			break
		}
	}

	if conn == nil {
		return nil, fmt.Errorf("connection '%s' not found", connectionName)
	}

	if !conn.Enabled {
		return nil, fmt.Errorf("connection '%s' is disabled", connectionName)
	}

	// Create new store based on database type
	var store storage.MemoryStore
	var err error

	switch conn.Database.Type {
	case "sqlite":
		dbPath := conn.Database.Path
		// Resolve relative paths against the directory containing the config file
		// so that memento-mcp works correctly when invoked from any working directory.
		if !filepath.IsAbs(dbPath) && m.baseDir != "" {
			dbPath = filepath.Join(m.baseDir, dbPath)
		}
		store, err = sqlite.NewMemoryStore(dbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create SQLite store for '%s': %w", connectionName, err)
		}
	case "postgresql":
		// Set default port if not specified
		port := conn.Database.Port
		if port == 0 {
			port = 5432
		}
		// Set default sslmode if not specified
		sslmode := conn.Database.SSLMode
		if sslmode == "" {
			sslmode = "disable"
		}
		dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			conn.Database.Username,
			conn.Database.Password,
			conn.Database.Host,
			port,
			conn.Database.Database,
			sslmode,
		)
		store, err = postgres.NewMemoryStore(dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to create PostgreSQL store for '%s' (DSN: %s): %w", connectionName, sanitizeDSN(dsn), err)
		}
	default:
		return nil, fmt.Errorf("unsupported database type '%s' for connection '%s'", conn.Database.Type, connectionName)
	}

	// Cache it and mark as owned by this manager
	m.storesLock.Lock()
	m.stores[connectionName] = store
	m.ownedStores[connectionName] = true
	m.storesLock.Unlock()

	return store, nil
}

// ListConnections returns all configured connections
func (m *Manager) ListConnections() []Connection {
	return m.config.Connections
}

// GetDefaultConnection returns the default connection name
func (m *Manager) GetDefaultConnection() string {
	return m.config.DefaultConnection
}

// AddConnection adds a new connection to the configuration
func (m *Manager) AddConnection(ctx context.Context, conn Connection) error {
	// Validate
	if conn.Name == "" {
		return fmt.Errorf("connection name is required")
	}

	// Check if exists
	for _, existing := range m.config.Connections {
		if existing.Name == conn.Name {
			return fmt.Errorf("connection '%s' already exists", conn.Name)
		}
	}

	// Check max connections
	if len(m.config.Connections) >= m.config.Settings.MaxConnections {
		return fmt.Errorf("maximum connections limit (%d) reached", m.config.Settings.MaxConnections)
	}

	// Add to config
	m.config.Connections = append(m.config.Connections, conn)

	// Save config
	return m.SaveConfig()
}

// UpdateConnection updates an existing connection's configuration
func (m *Manager) UpdateConnection(ctx context.Context, name string, updatedConn Connection) error {
	// Validate
	if name == "" {
		return fmt.Errorf("connection name is required")
	}

	// Find and update connection
	found := false
	for i := range m.config.Connections {
		if m.config.Connections[i].Name == name {
			// Preserve name (can't change) and created_at
			updatedConn.Name = name
			updatedConn.CreatedAt = m.config.Connections[i].CreatedAt
			m.config.Connections[i] = updatedConn
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("connection '%s' not found", name)
	}

	// Invalidate cached store (will be recreated with new config)
	// Only close if we own it (not borrowed from external caller)
	m.storesLock.Lock()
	if store, exists := m.stores[name]; exists {
		if m.ownedStores[name] {
			_ = store.Close()
		}
		delete(m.stores, name)
		delete(m.ownedStores, name)
	}
	m.storesLock.Unlock()

	// Save config
	return m.SaveConfig()
}

// DeleteConnection removes a connection from the configuration
func (m *Manager) DeleteConnection(ctx context.Context, name string) error {
	// Can't delete the default connection
	if name == m.config.DefaultConnection {
		return fmt.Errorf("cannot delete the default connection")
	}

	// Find and remove connection
	found := false
	newConnections := make([]Connection, 0, len(m.config.Connections))
	for _, conn := range m.config.Connections {
		if conn.Name == name {
			found = true
			// Close the store if it's cached and we own it
			m.storesLock.Lock()
			if store, exists := m.stores[name]; exists {
				if m.ownedStores[name] {
					_ = store.Close()
				}
				delete(m.stores, name)
				delete(m.ownedStores, name)
			}
			m.storesLock.Unlock()
			continue
		}
		newConnections = append(newConnections, conn)
	}

	if !found {
		return fmt.Errorf("connection '%s' not found", name)
	}

	m.config.Connections = newConnections
	return m.SaveConfig()
}

// SetDefaultConnection sets the default connection
func (m *Manager) SetDefaultConnection(ctx context.Context, name string) error {
	// Verify connection exists
	found := false
	for _, conn := range m.config.Connections {
		if conn.Name == name {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("connection '%s' not found", name)
	}

	m.config.DefaultConnection = name
	return m.SaveConfig()
}

// TestConnection tests a connection configuration without saving it
func (m *Manager) TestConnection(ctx context.Context, conn Connection) error {
	switch conn.Database.Type {
	case "sqlite":
		// Test SQLite connection
		store, err := sqlite.NewMemoryStore(conn.Database.Path)
		if err != nil {
			return fmt.Errorf("failed to connect to SQLite: %w", err)
		}
		defer func() { _ = store.Close() }()

		// Try a simple query to verify the connection works
		_, err = store.List(ctx, storage.ListOptions{Page: 1, Limit: 1})
		if err != nil {
			return fmt.Errorf("failed to query database: %w", err)
		}

	case "postgresql":
		// Set default port if not specified
		port := conn.Database.Port
		if port == 0 {
			port = 5432
		}
		// Set default sslmode if not specified
		sslmode := conn.Database.SSLMode
		if sslmode == "" {
			sslmode = "disable"
		}

		// Test PostgreSQL connection
		dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			conn.Database.Username,
			conn.Database.Password,
			conn.Database.Host,
			port,
			conn.Database.Database,
			sslmode,
		)
		store, err := postgres.NewMemoryStore(dsn)
		if err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL (DSN: %s): %w", sanitizeDSN(dsn), err)
		}
		defer func() { _ = store.Close() }()

		// Try a simple query to verify the connection works
		_, err = store.List(ctx, storage.ListOptions{Page: 1, Limit: 1})
		if err != nil {
			return fmt.Errorf("failed to query database: %w", err)
		}

	default:
		return fmt.Errorf("unsupported database type: %s", conn.Database.Type)
	}

	return nil
}

// Close closes all open connections
// Only closes stores that are owned by this manager (not borrowed from external callers)
func (m *Manager) Close() error {
	m.storesLock.Lock()
	defer m.storesLock.Unlock()

	var lastErr error
	for name, store := range m.stores {
		// Only close stores we own (not borrowed from external caller)
		if m.ownedStores[name] {
			if err := store.Close(); err != nil {
				lastErr = fmt.Errorf("failed to close connection '%s': %w", name, err)
			}
		}
	}

	return lastErr
}

// Package config provides configuration management for Memento.
// It loads settings from environment variables with the MEMENTO_ prefix
// and provides sensible defaults for all configuration options.
//
// User settings (e.g., user_name) are persisted to the settings table in
// the database. LoadConfigFromDB reads from the database first and falls back
// to environment variables. SaveConfig writes user settings to the database.
package config

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
)

// Config holds all configuration settings for the Memento application.
type Config struct {
	Server   ServerConfig
	Storage  StorageConfig
	LLM      LLMConfig
	Security SecurityConfig
	Backup   BackupConfig
	Features FeaturesConfig
	User     UserConfig
}

// ServerConfig contains HTTP server configuration.
type ServerConfig struct {
	Port int    // Server port (default: 6363)
	Host string // Server host (default: 0.0.0.0)
}

// StorageConfig contains database and storage configuration.
type StorageConfig struct {
	StorageEngine string // Storage engine type: sqlite, postgres, etc. (default: sqlite)
	DataPath      string // Path to data directory (default: ./data)
}

// LLMConfig contains LLM provider configuration.
type LLMConfig struct {
	LLMProvider          string // LLM provider: ollama, openai, anthropic (default: ollama)
	OllamaURL            string // Ollama API URL (default: http://localhost:11434)
	OllamaModel          string // Ollama model name for extraction (default: qwen2.5:7b)
	OllamaEmbeddingModel string // Ollama model name for embeddings (default: nomic-embed-text)
	OpenAIAPIKey         string // OpenAI API key
	OpenAIModel          string // OpenAI model name (default: gpt-4)
	AnthropicAPIKey      string // Anthropic API key
	AnthropicModel       string // Anthropic model name (default: claude-3-5-sonnet-20241022)
}

// SecurityConfig contains security and authentication settings.
type SecurityConfig struct {
	SecurityMode string // Security mode: development, production (default: development)
	APIToken     string // API authentication token
}

// BackupConfig contains backup configuration.
type BackupConfig struct {
	BackupEnabled          bool   // Enable automatic backups (default: false)
	BackupInterval         string // Backup interval duration (default: 24h)
	BackupPath             string // Path to backup directory (default: ./backups)
	BackupVerify           bool   // Verify backups after creation (default: true)
	BackupRetentionHourly  int    // Number of hourly backups to keep (default: 24)
	BackupRetentionDaily   int    // Number of daily backups to keep (default: 7)
	BackupRetentionWeekly  int    // Number of weekly backups to keep (default: 4)
	BackupRetentionMonthly int    // Number of monthly backups to keep (default: 12)
}

// FeaturesConfig contains feature flags.
type FeaturesConfig struct {
	EnableWebUI bool // Enable web UI (default: true)
	EnableMCP   bool // Enable MCP server (default: true)
	EnableREST  bool // Enable REST API (default: true)
}

// UserConfig contains user-specific settings that persist across restarts.
// These settings are stored in the settings table in the database.
type UserConfig struct {
	// UserName is the display name for the user.
	// Env var: MEMENTO_USER_NAME
	// Database key: user_name
	UserName string
}

// LoadConfig loads configuration from environment variables with sensible defaults.
// All environment variables use the MEMENTO_ prefix.
// User settings (UserConfig) are loaded from environment variables only.
// Use LoadConfigFromDB to also read persisted user settings from the database.
func LoadConfig() (*Config, error) {
	cfg := buildBaseConfig()
	return cfg, nil
}

// LoadConfigFromDB loads configuration from both environment variables and the
// database. The database value takes precedence over the environment variable
// for user settings. Falls back to environment variable when no DB entry exists.
//
// Returns an error if db is nil.
func LoadConfigFromDB(db *sql.DB) (*Config, error) {
	if db == nil {
		return nil, errors.New("config: database connection is required")
	}

	cfg := buildBaseConfig()

	// Load user_name from settings table (DB takes precedence over env var)
	userName, err := getSetting(db, "user_name")
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("config: failed to load user_name from database: %w", err)
	}

	if userName != "" {
		// DB value overrides env var
		cfg.User.UserName = userName
	}
	// If no DB value, cfg.User.UserName already has the env var value from buildBaseConfig()

	return cfg, nil
}

// SaveConfig persists user configuration settings to the settings table in the
// database. Uses upsert semantics: inserts if not present, updates if already
// stored. This ensures user settings survive application restarts.
//
// Returns an error if db is nil.
func (c *Config) SaveConfig(db *sql.DB) error {
	if db == nil {
		return errors.New("config: database connection is required")
	}

	if err := setSetting(db, "user_name", c.User.UserName); err != nil {
		return fmt.Errorf("config: failed to save user_name: %w", err)
	}

	return nil
}

// getSetting retrieves a single setting value by key from the settings table.
// Returns an empty string and sql.ErrNoRows if the key does not exist.
func getSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

// setSetting writes a key-value pair to the settings table using upsert semantics.
func setSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(`
		INSERT INTO settings (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return err
}

// buildBaseConfig constructs a Config with values from environment variables
// and defaults. This is the shared base for both LoadConfig and LoadConfigFromDB.
func buildBaseConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: getEnvInt("MEMENTO_PORT", 6363),
			Host: getEnv("MEMENTO_HOST", "127.0.0.1"),
		},
		Storage: StorageConfig{
			StorageEngine: getEnv("MEMENTO_STORAGE_ENGINE", "sqlite"),
			DataPath:      getEnv("MEMENTO_DATA_PATH", "./data"),
		},
		LLM: LLMConfig{
			LLMProvider:          getEnv("MEMENTO_LLM_PROVIDER", "ollama"),
			OllamaURL:            getEnv("MEMENTO_OLLAMA_URL", "http://localhost:11434"),
			OllamaModel:          getEnv("MEMENTO_OLLAMA_MODEL", "qwen2.5:7b"),
			OllamaEmbeddingModel: getEnv("MEMENTO_EMBEDDING_MODEL", "nomic-embed-text"),
			OpenAIAPIKey:         getEnv("MEMENTO_OPENAI_API_KEY", ""),
			OpenAIModel:          getEnv("MEMENTO_OPENAI_MODEL", "gpt-4"),
			AnthropicAPIKey:      getEnv("MEMENTO_ANTHROPIC_API_KEY", ""),
			AnthropicModel:       getEnv("MEMENTO_ANTHROPIC_MODEL", "claude-3-5-sonnet-20241022"),
		},
		Security: SecurityConfig{
			SecurityMode: getEnv("MEMENTO_SECURITY_MODE", "development"),
			APIToken:     getEnv("MEMENTO_API_TOKEN", ""),
		},
		Backup: BackupConfig{
			BackupEnabled:          getEnvBool("MEMENTO_BACKUP_ENABLED", false),
			BackupInterval:         getEnv("MEMENTO_BACKUP_INTERVAL", "24h"),
			BackupPath:             getEnv("MEMENTO_BACKUP_PATH", "./backups"),
			BackupVerify:           getEnvBool("MEMENTO_BACKUP_VERIFY", true),
			BackupRetentionHourly:  getEnvInt("MEMENTO_BACKUP_RETENTION_HOURLY", 24),
			BackupRetentionDaily:   getEnvInt("MEMENTO_BACKUP_RETENTION_DAILY", 7),
			BackupRetentionWeekly:  getEnvInt("MEMENTO_BACKUP_RETENTION_WEEKLY", 4),
			BackupRetentionMonthly: getEnvInt("MEMENTO_BACKUP_RETENTION_MONTHLY", 12),
		},
		Features: FeaturesConfig{
			EnableWebUI: getEnvBool("MEMENTO_ENABLE_WEB_UI", true),
			EnableMCP:   getEnvBool("MEMENTO_ENABLE_MCP", true),
			EnableREST:  getEnvBool("MEMENTO_ENABLE_REST", true),
		},
		User: UserConfig{
			UserName: getEnv("MEMENTO_USER_NAME", ""),
		},
	}
}

// getEnv retrieves a string environment variable or returns a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt retrieves an integer environment variable or returns a default value.
// If the environment variable exists but cannot be parsed as an integer,
// it returns the default value.
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvBool retrieves a boolean environment variable or returns a default value.
// It recognizes "true", "1", "yes" as true and "false", "0", "no" as false (case-insensitive).
// If the environment variable exists but cannot be parsed as a boolean,
// it returns the default value.
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		switch value {
		case "true", "1", "yes", "True", "TRUE", "Yes", "YES":
			return true
		case "false", "0", "no", "False", "FALSE", "No", "NO":
			return false
		}
	}
	return defaultValue
}

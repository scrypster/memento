package config_test

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/scrypster/memento/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_DefaultHostIsLocalhost(t *testing.T) {
	_ = os.Unsetenv("MEMENTO_HOST")
	cfg, err := config.LoadConfig()
	assert.NoError(t, err)
	assert.Equal(t, "127.0.0.1", cfg.Server.Host,
		"Default host must be 127.0.0.1 for security")
}

func TestLoadConfig_CanOverrideHost(t *testing.T) {
	t.Setenv("MEMENTO_HOST", "0.0.0.0")
	cfg, err := config.LoadConfig()
	assert.NoError(t, err)
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
}

// TestUserConfig_DefaultValues verifies UserConfig has sensible defaults
// when no environment variables or database entries are set.
func TestUserConfig_DefaultValues(t *testing.T) {
	_ = os.Unsetenv("MEMENTO_USER_NAME")

	cfg, err := config.LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, "", cfg.User.UserName,
		"Default UserName must be empty string when not configured")
}

// TestUserConfig_EnvVarFallback verifies that MEMENTO_USER_NAME env var
// sets the user name when no database value exists.
func TestUserConfig_EnvVarFallback(t *testing.T) {
	t.Setenv("MEMENTO_USER_NAME", "alice")

	cfg, err := config.LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, "alice", cfg.User.UserName)
}

// TestSaveConfig_PersistsUserName verifies that SaveConfig writes the user
// name to the settings table and can be read back.
func TestSaveConfig_PersistsUserName(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	// Create a config with a user name
	cfg := &config.Config{}
	cfg.User.UserName = "bob"

	// Save to database
	err := cfg.SaveConfig(db)
	require.NoError(t, err, "SaveConfig must not return an error")

	// Read back directly from DB
	var value string
	err = db.QueryRow("SELECT value FROM settings WHERE key = 'user_name'").Scan(&value)
	require.NoError(t, err, "user_name must be stored in settings table")
	assert.Equal(t, "bob", value, "stored user_name must match saved value")
}

// TestLoadConfigFromDB_ReadsUserName verifies that LoadConfigFromDB reads
// the user_name from the settings table.
func TestLoadConfigFromDB_ReadsUserName(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	// Insert user_name directly into settings table
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('user_name', 'charlie')`)
	require.NoError(t, err)

	// Load config from DB (no env override)
	_ = os.Unsetenv("MEMENTO_USER_NAME")
	cfg, err := config.LoadConfigFromDB(db)
	require.NoError(t, err, "LoadConfigFromDB must not return an error")

	assert.Equal(t, "charlie", cfg.User.UserName,
		"UserName must be read from settings table")
}

// TestLoadConfigFromDB_DBOverridesEnvVar verifies that the database value
// takes precedence over the environment variable.
func TestLoadConfigFromDB_DBOverridesEnvVar(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	// Set env var
	t.Setenv("MEMENTO_USER_NAME", "env-user")

	// Insert different value in DB
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('user_name', 'db-user')`)
	require.NoError(t, err)

	cfg, err := config.LoadConfigFromDB(db)
	require.NoError(t, err)

	assert.Equal(t, "db-user", cfg.User.UserName,
		"Database value must take precedence over environment variable")
}

// TestLoadConfigFromDB_FallsBackToEnvVar verifies that when no database entry
// exists, LoadConfigFromDB falls back to the environment variable.
func TestLoadConfigFromDB_FallsBackToEnvVar(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	t.Setenv("MEMENTO_USER_NAME", "fallback-user")

	// No DB entry - should fall back to env var
	cfg, err := config.LoadConfigFromDB(db)
	require.NoError(t, err)

	assert.Equal(t, "fallback-user", cfg.User.UserName,
		"Must fall back to env var when no DB entry exists")
}

// TestSaveAndLoad_RoundTrip verifies that SaveConfig and LoadConfigFromDB
// work together for a complete round-trip.
func TestSaveAndLoad_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	_ = os.Unsetenv("MEMENTO_USER_NAME")

	// Save a config
	original := &config.Config{}
	original.User.UserName = "round-trip-user"
	err := original.SaveConfig(db)
	require.NoError(t, err, "SaveConfig must succeed")

	// Load it back
	loaded, err := config.LoadConfigFromDB(db)
	require.NoError(t, err, "LoadConfigFromDB must succeed after SaveConfig")

	assert.Equal(t, original.User.UserName, loaded.User.UserName,
		"Loaded config must match saved config")
}

// TestSaveConfig_UpdatesExistingEntry verifies that saving the same key twice
// updates the value (upsert semantics).
func TestSaveConfig_UpdatesExistingEntry(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	cfg := &config.Config{}

	// Save first value
	cfg.User.UserName = "first"
	err := cfg.SaveConfig(db)
	require.NoError(t, err)

	// Save second value (should upsert)
	cfg.User.UserName = "second"
	err = cfg.SaveConfig(db)
	require.NoError(t, err)

	// Verify only one row and it has the updated value
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM settings WHERE key = 'user_name'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Must have exactly one row for user_name")

	var value string
	err = db.QueryRow("SELECT value FROM settings WHERE key = 'user_name'").Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, "second", value, "Value must be updated to latest")
}

// TestLoadConfigFromDB_NilDB verifies that passing nil db returns an error.
func TestLoadConfigFromDB_NilDB(t *testing.T) {
	_, err := config.LoadConfigFromDB(nil)
	assert.Error(t, err, "LoadConfigFromDB with nil db must return an error")
}

// TestSaveConfig_NilDB verifies that SaveConfig with nil db returns an error.
func TestSaveConfig_NilDB(t *testing.T) {
	cfg := &config.Config{}
	cfg.User.UserName = "test"
	err := cfg.SaveConfig(nil)
	assert.Error(t, err, "SaveConfig with nil db must return an error")
}

// openTestDB creates an in-memory SQLite database with the settings schema.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err, "Failed to open in-memory SQLite database")

	// Create the settings table (matches migration 000003)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err, "Failed to create settings table")

	return db
}

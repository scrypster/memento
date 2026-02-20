package tests

import (
	"os"
	"testing"

	"github.com/scrypster/memento/internal/config"
)

// TestDefaultConfig verifies that LoadConfig returns sensible defaults
// when no environment variables are set.
func TestDefaultConfig(t *testing.T) {
	// Clear any MEMENTO_ environment variables
	clearMementoEnv(t)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Server defaults
	if cfg.Server.Port != 6363 {
		t.Errorf("expected default Port=6363, got %d", cfg.Server.Port)
	}
	// Default host is 127.0.0.1 (localhost only) for security.
	// Use MEMENTO_HOST=0.0.0.0 to bind to all interfaces.
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected default Host=127.0.0.1, got %s", cfg.Server.Host)
	}

	// Storage defaults
	if cfg.Storage.StorageEngine != "sqlite" {
		t.Errorf("expected default StorageEngine=sqlite, got %s", cfg.Storage.StorageEngine)
	}
	if cfg.Storage.DataPath != "./data" {
		t.Errorf("expected default DataPath=./data, got %s", cfg.Storage.DataPath)
	}

	// LLM defaults
	if cfg.LLM.LLMProvider != "ollama" {
		t.Errorf("expected default LLMProvider=ollama, got %s", cfg.LLM.LLMProvider)
	}
	if cfg.LLM.OllamaURL != "http://localhost:11434" {
		t.Errorf("expected default OllamaURL=http://localhost:11434, got %s", cfg.LLM.OllamaURL)
	}
	if cfg.LLM.OllamaModel != "qwen2.5:7b" {
		t.Errorf("expected default OllamaModel=qwen2.5:7b, got %s", cfg.LLM.OllamaModel)
	}
	if cfg.LLM.OpenAIModel != "gpt-4" {
		t.Errorf("expected default OpenAIModel=gpt-4, got %s", cfg.LLM.OpenAIModel)
	}
	if cfg.LLM.AnthropicModel != "claude-3-5-sonnet-20241022" {
		t.Errorf("expected default AnthropicModel=claude-3-5-sonnet-20241022, got %s", cfg.LLM.AnthropicModel)
	}

	// Security defaults
	if cfg.Security.SecurityMode != "development" {
		t.Errorf("expected default SecurityMode=development, got %s", cfg.Security.SecurityMode)
	}
	if cfg.Security.APIToken != "" {
		t.Errorf("expected default APIToken to be empty, got %s", cfg.Security.APIToken)
	}

	// Backup defaults
	if cfg.Backup.BackupEnabled != false {
		t.Errorf("expected default BackupEnabled=false, got %v", cfg.Backup.BackupEnabled)
	}
	if cfg.Backup.BackupInterval != "24h" {
		t.Errorf("expected default BackupInterval=24h, got %s", cfg.Backup.BackupInterval)
	}
	if cfg.Backup.BackupPath != "./backups" {
		t.Errorf("expected default BackupPath=./backups, got %s", cfg.Backup.BackupPath)
	}

	// Features defaults
	if cfg.Features.EnableWebUI != true {
		t.Errorf("expected default EnableWebUI=true, got %v", cfg.Features.EnableWebUI)
	}
	if cfg.Features.EnableMCP != true {
		t.Errorf("expected default EnableMCP=true, got %v", cfg.Features.EnableMCP)
	}
	if cfg.Features.EnableREST != true {
		t.Errorf("expected default EnableREST=true, got %v", cfg.Features.EnableREST)
	}
}

// TestEnvOverrides verifies that environment variables properly override defaults.
func TestEnvOverrides(t *testing.T) {
	// Set environment variables
	testEnv := map[string]string{
		"MEMENTO_PORT":             "8080",
		"MEMENTO_HOST":             "127.0.0.1",
		"MEMENTO_STORAGE_ENGINE":   "postgres",
		"MEMENTO_DATA_PATH":        "/custom/data",
		"MEMENTO_LLM_PROVIDER":     "openai",
		"MEMENTO_OLLAMA_URL":       "http://custom:11434",
		"MEMENTO_OLLAMA_MODEL":     "llama3",
		"MEMENTO_OPENAI_API_KEY":   "sk-test123",
		"MEMENTO_OPENAI_MODEL":     "gpt-4-turbo",
		"MEMENTO_ANTHROPIC_API_KEY": "sk-ant-test456",
		"MEMENTO_ANTHROPIC_MODEL":  "claude-opus-4",
		"MEMENTO_SECURITY_MODE":    "production",
		"MEMENTO_API_TOKEN":        "secret-token",
		"MEMENTO_BACKUP_ENABLED":   "true",
		"MEMENTO_BACKUP_INTERVAL":  "12h",
		"MEMENTO_BACKUP_PATH":      "/custom/backups",
		"MEMENTO_ENABLE_WEB_UI":    "false",
		"MEMENTO_ENABLE_MCP":       "false",
		"MEMENTO_ENABLE_REST":      "false",
	}

	// Set all test environment variables
	for key, value := range testEnv {
		if err := os.Setenv(key, value); err != nil {
			t.Fatalf("failed to set %s: %v", key, err)
		}
	}
	// Clean up after test
	defer clearMementoEnv(t)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Verify Server overrides
	if cfg.Server.Port != 8080 {
		t.Errorf("expected Port=8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected Host=127.0.0.1, got %s", cfg.Server.Host)
	}

	// Verify Storage overrides
	if cfg.Storage.StorageEngine != "postgres" {
		t.Errorf("expected StorageEngine=postgres, got %s", cfg.Storage.StorageEngine)
	}
	if cfg.Storage.DataPath != "/custom/data" {
		t.Errorf("expected DataPath=/custom/data, got %s", cfg.Storage.DataPath)
	}

	// Verify LLM overrides
	if cfg.LLM.LLMProvider != "openai" {
		t.Errorf("expected LLMProvider=openai, got %s", cfg.LLM.LLMProvider)
	}
	if cfg.LLM.OllamaURL != "http://custom:11434" {
		t.Errorf("expected OllamaURL=http://custom:11434, got %s", cfg.LLM.OllamaURL)
	}
	if cfg.LLM.OllamaModel != "llama3" {
		t.Errorf("expected OllamaModel=llama3, got %s", cfg.LLM.OllamaModel)
	}
	if cfg.LLM.OpenAIAPIKey != "sk-test123" {
		t.Errorf("expected OpenAIAPIKey=sk-test123, got %s", cfg.LLM.OpenAIAPIKey)
	}
	if cfg.LLM.OpenAIModel != "gpt-4-turbo" {
		t.Errorf("expected OpenAIModel=gpt-4-turbo, got %s", cfg.LLM.OpenAIModel)
	}
	if cfg.LLM.AnthropicAPIKey != "sk-ant-test456" {
		t.Errorf("expected AnthropicAPIKey=sk-ant-test456, got %s", cfg.LLM.AnthropicAPIKey)
	}
	if cfg.LLM.AnthropicModel != "claude-opus-4" {
		t.Errorf("expected AnthropicModel=claude-opus-4, got %s", cfg.LLM.AnthropicModel)
	}

	// Verify Security overrides
	if cfg.Security.SecurityMode != "production" {
		t.Errorf("expected SecurityMode=production, got %s", cfg.Security.SecurityMode)
	}
	if cfg.Security.APIToken != "secret-token" {
		t.Errorf("expected APIToken=secret-token, got %s", cfg.Security.APIToken)
	}

	// Verify Backup overrides
	if cfg.Backup.BackupEnabled != true {
		t.Errorf("expected BackupEnabled=true, got %v", cfg.Backup.BackupEnabled)
	}
	if cfg.Backup.BackupInterval != "12h" {
		t.Errorf("expected BackupInterval=12h, got %s", cfg.Backup.BackupInterval)
	}
	if cfg.Backup.BackupPath != "/custom/backups" {
		t.Errorf("expected BackupPath=/custom/backups, got %s", cfg.Backup.BackupPath)
	}

	// Verify Features overrides
	if cfg.Features.EnableWebUI != false {
		t.Errorf("expected EnableWebUI=false, got %v", cfg.Features.EnableWebUI)
	}
	if cfg.Features.EnableMCP != false {
		t.Errorf("expected EnableMCP=false, got %v", cfg.Features.EnableMCP)
	}
	if cfg.Features.EnableREST != false {
		t.Errorf("expected EnableREST=false, got %v", cfg.Features.EnableREST)
	}
}

// clearMementoEnv removes all MEMENTO_ environment variables for clean testing.
func clearMementoEnv(t *testing.T) {
	t.Helper()

	vars := []string{
		"MEMENTO_PORT",
		"MEMENTO_HOST",
		"MEMENTO_STORAGE_ENGINE",
		"MEMENTO_DATA_PATH",
		"MEMENTO_LLM_PROVIDER",
		"MEMENTO_OLLAMA_URL",
		"MEMENTO_OLLAMA_MODEL",
		"MEMENTO_OPENAI_API_KEY",
		"MEMENTO_OPENAI_MODEL",
		"MEMENTO_ANTHROPIC_API_KEY",
		"MEMENTO_ANTHROPIC_MODEL",
		"MEMENTO_SECURITY_MODE",
		"MEMENTO_API_TOKEN",
		"MEMENTO_BACKUP_ENABLED",
		"MEMENTO_BACKUP_INTERVAL",
		"MEMENTO_BACKUP_PATH",
		"MEMENTO_ENABLE_WEB_UI",
		"MEMENTO_ENABLE_MCP",
		"MEMENTO_ENABLE_REST",
	}

	for _, v := range vars {
		_ = os.Unsetenv(v)
	}
}

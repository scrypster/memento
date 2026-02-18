package handlers

import (
	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/pkg/types"
)

// ErrorResponse is the standard error response format for the API.
type ErrorResponse struct {
	Error   string                 `json:"error"`
	Code    string                 `json:"code"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// ConfigResponse is the response format for GET /api/config.
// API keys are masked for security.
type ConfigResponse struct {
	LLM        LLMConfigResponse `json:"llm"`
	Enrichment EnrichmentConfig  `json:"enrichment"`
	Search     SearchConfig      `json:"search"`
	Backup     BackupConfig      `json:"backup"`
}

// LLMConfigResponse contains LLM configuration with masked API keys.
type LLMConfigResponse struct {
	Provider        string `json:"provider"`
	OllamaURL       string `json:"ollama_url"`
	OllamaModel     string `json:"ollama_model"`
	OpenAIAPIKey    string `json:"openai_api_key"`    // Masked
	OpenAIModel     string `json:"openai_model"`
	AnthropicAPIKey string `json:"anthropic_api_key"` // Masked
	AnthropicModel  string `json:"anthropic_model"`
}

// EnrichmentConfig contains enrichment settings.
type EnrichmentConfig struct {
	AutoEnrich bool `json:"auto_enrich"`
	MaxRetries int  `json:"max_retries"`
}

// SearchConfig contains search settings.
type SearchConfig struct {
	ResultsPerPage     int     `json:"results_per_page"`
	MinConfidenceScore float64 `json:"min_confidence_score"`
}

// BackupConfig contains backup settings.
type BackupConfig struct {
	Enabled   bool   `json:"enabled"`
	Schedule  string `json:"schedule"`
	Retention string `json:"retention"`
}

// SearchResponse is the response format for GET /api/search.
type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Total   int            `json:"total"`
	Page    int            `json:"page"`
	Pages   int            `json:"pages"`
	Query   string         `json:"query"`
}

// SearchResult is a single search result with score.
type SearchResult struct {
	Memory types.Memory `json:"memory"`
	Score  float64      `json:"score"`
}

// StatsResponse is the response format for GET /api/stats.
type StatsResponse struct {
	Memories      int `json:"memories"`
	Entities      int `json:"entities"`
	Relationships int `json:"relationships"`
	QueueSize     int `json:"queue_size"`
}

// ImportRequest is the request format for POST /api/import (JSON body).
type ImportRequest struct {
	Memories []ImportMemory `json:"memories"`
}

// ImportMemory is a single memory to import.
type ImportMemory struct {
	Content  string                 `json:"content"`
	Source   string                 `json:"source"`
	Domain   string                 `json:"domain,omitempty"`
	Tags     []string               `json:"tags,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ImportResponse is the response format for POST /api/import.
type ImportResponse struct {
	ImportID string `json:"import_id"`
	Total    int    `json:"total"`
	Message  string `json:"message"`
}

// MaskAPIKey masks an API key for safe display.
// Shows first 7 chars and last 4 chars, hides the middle.
func MaskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) < 8 {
		return "***"
	}
	return key[:7] + "..." + key[len(key)-4:]
}

// ToConfigResponse converts a config.Config to ConfigResponse with masked keys.
func ToConfigResponse(cfg *config.Config) ConfigResponse {
	return ConfigResponse{
		LLM: LLMConfigResponse{
			Provider:        cfg.LLM.LLMProvider,
			OllamaURL:       cfg.LLM.OllamaURL,
			OllamaModel:     cfg.LLM.OllamaModel,
			OpenAIAPIKey:    MaskAPIKey(cfg.LLM.OpenAIAPIKey),
			OpenAIModel:     cfg.LLM.OpenAIModel,
			AnthropicAPIKey: MaskAPIKey(cfg.LLM.AnthropicAPIKey),
			AnthropicModel:  cfg.LLM.AnthropicModel,
		},
		Enrichment: EnrichmentConfig{
			AutoEnrich: true, // TODO: read from config
			MaxRetries: 3,    // TODO: read from config
		},
		Search: SearchConfig{
			ResultsPerPage:     20,
			MinConfidenceScore: 0.0,
		},
		Backup: BackupConfig{
			Enabled:   cfg.Backup.BackupEnabled,
			Schedule:  cfg.Backup.BackupInterval,
			Retention: "hourly(24),daily(7),weekly(4),monthly(12)",
		},
	}
}

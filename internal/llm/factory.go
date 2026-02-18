package llm

import (
	"fmt"

	"github.com/scrypster/memento/internal/connections"
)

// NewTextGenerator creates the appropriate TextGenerator based on connection LLM config.
func NewTextGenerator(cfg connections.LLMConfig) (TextGenerator, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIClient(OpenAIConfig{APIKey: cfg.APIKey, Model: cfg.Model, BaseURL: cfg.BaseURL}), nil
	case "anthropic":
		return NewAnthropicClient(AnthropicConfig{APIKey: cfg.APIKey, Model: cfg.Model}), nil
	case "ollama", "":
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		model := cfg.Model
		if model == "" {
			model = "qwen2.5:7b"
		}
		return NewOllamaClient(OllamaConfig{BaseURL: baseURL, Model: model}), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q", cfg.Provider)
	}
}

// NewEmbeddingGenerator creates the appropriate EmbeddingGenerator.
// Returns (nil, nil) for providers that don't support embeddings (Anthropic).
func NewEmbeddingGenerator(cfg connections.LLMConfig, embeddingModel string) (EmbeddingGenerator, error) {
	switch cfg.Provider {
	case "openai":
		model := embeddingModel
		if model == "" {
			model = "text-embedding-3-small"
		}
		return NewOpenAIEmbeddingClient(OpenAIEmbeddingConfig{APIKey: cfg.APIKey, Model: model, BaseURL: cfg.BaseURL}), nil
	case "ollama", "":
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		model := embeddingModel
		if model == "" {
			model = "nomic-embed-text"
		}
		return NewOllamaClient(OllamaConfig{BaseURL: baseURL, Model: model}), nil
	default:
		// Anthropic and others don't support embeddings
		return nil, nil
	}
}

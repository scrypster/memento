package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LLMModelHandlers handles LLM model discovery and testing
type LLMModelHandlers struct {
	client *http.Client
}

// NewLLMModelHandlers creates a new LLMModelHandlers instance
func NewLLMModelHandlers() *LLMModelHandlers {
	return &LLMModelHandlers{
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// AvailableModelsResponse is the response for available models endpoint
type AvailableModelsResponse struct {
	Models []string `json:"models"`
	Error  string   `json:"error,omitempty"`
}

// TestLLMConnectionRequest is the request body for test LLM connection
type TestLLMConnectionRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
}

// TestLLMConnectionResponse is the response for test LLM connection
type TestLLMConnectionResponse struct {
	Success         bool     `json:"success"`
	AvailableModels []string `json:"available_models,omitempty"`
	Error           string   `json:"error,omitempty"`
}

// GetAvailableModels handles GET /api/llm/available-models
func (h *LLMModelHandlers) GetAvailableModels(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	apiKey := r.URL.Query().Get("api_key")
	baseURL := r.URL.Query().Get("base_url")

	if provider == "" {
		respondError(w, http.StatusBadRequest, "provider is required", nil)
		return
	}

	// Context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var models []string
	var err error

	switch provider {
	case "ollama":
		models, err = h.getOllamaModels(ctx, baseURL)
	case "openai":
		models, err = h.getOpenAIModels(ctx, apiKey)
	case "anthropic":
		models, err = h.getAnthropicModels(ctx, apiKey)
	default:
		respondError(w, http.StatusBadRequest, "unsupported provider", nil)
		return
	}

	resp := AvailableModelsResponse{
		Models: models,
	}
	if err != nil {
		resp.Error = err.Error()
	}

	respondJSON(w, http.StatusOK, resp)
}

// TestConnection handles POST /api/llm/test-connection
// Tests LLM provider connectivity with the given configuration
func (h *LLMModelHandlers) TestConnection(w http.ResponseWriter, r *http.Request) {
	var req TestLLMConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request", err)
		return
	}

	// Validate required fields based on provider
	if req.Provider == "" {
		respondJSON(w, http.StatusOK, TestLLMConnectionResponse{
			Success: false,
			Error:   "provider is required",
		})
		return
	}

	switch req.Provider {
	case "openai", "anthropic":
		if req.APIKey == "" {
			respondJSON(w, http.StatusOK, TestLLMConnectionResponse{
				Success: false,
				Error:   fmt.Sprintf("API key is required for %s", req.Provider),
			})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp := TestLLMConnectionResponse{}

	switch req.Provider {
	case "ollama":
		if err := h.testOllamaConnection(ctx, req.BaseURL); err == nil {
			resp.Success = true
			models, _ := h.getOllamaModels(ctx, req.BaseURL)
			resp.AvailableModels = models
		} else {
			resp.Error = err.Error()
		}
	case "openai":
		if err := h.testOpenAIConnection(ctx, req.APIKey); err == nil {
			resp.Success = true
			models, _ := h.getOpenAIModels(ctx, req.APIKey)
			resp.AvailableModels = models
		} else {
			resp.Error = err.Error()
		}
	case "anthropic":
		if err := h.testAnthropicConnection(ctx, req.APIKey); err == nil {
			resp.Success = true
			models, _ := h.getAnthropicModels(ctx, req.APIKey)
			resp.AvailableModels = models
		} else {
			resp.Error = err.Error()
		}
	default:
		resp.Error = fmt.Sprintf("unsupported provider: %q", req.Provider)
	}

	respondJSON(w, http.StatusOK, resp)
}

// getOllamaModels fetches available models from Ollama
func (h *LLMModelHandlers) getOllamaModels(ctx context.Context, baseURL string) ([]string, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ollama: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	var models []string
	for _, m := range result.Models {
		if m.Name != "" {
			models = append(models, m.Name)
		}
	}

	return models, nil
}

// testOllamaConnection tests Ollama connection
func (h *LLMModelHandlers) testOllamaConnection(ctx context.Context, baseURL string) error {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Ollama at %s: %w", baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	return nil
}

// getOpenAIModels fetches available models from OpenAI
func (h *LLMModelHandlers) getOpenAIModels(ctx context.Context, apiKey string) ([]string, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key required for OpenAI")
	}

	// For now, return known OpenAI models
	// In production, you'd call the models API
	return []string{
		"gpt-4-turbo",
		"gpt-4",
		"gpt-3.5-turbo",
	}, nil
}

// testOpenAIConnection tests OpenAI connection
func (h *LLMModelHandlers) testOpenAIConnection(ctx context.Context, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("API key required")
	}

	// Make a simple request to OpenAI API to test key validity
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.openai.com/v1/models", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to OpenAI: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid API key")
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenAI returned status %d", resp.StatusCode)
	}

	return nil
}

// getAnthropicModels fetches available models from Anthropic
func (h *LLMModelHandlers) getAnthropicModels(ctx context.Context, apiKey string) ([]string, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key required for Anthropic")
	}

	// For now, return known Anthropic models
	// In production, you'd call the models API if available
	return []string{
		"claude-opus-4-1",
		"claude-sonnet-4-20250514",
		"claude-haiku-3",
	}, nil
}

// testAnthropicConnection tests Anthropic connection
func (h *LLMModelHandlers) testAnthropicConnection(ctx context.Context, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("API key required")
	}

	// Make a simple API call to test key validity
	payload := map[string]interface{}{
		"model":      "claude-opus-4-1",
		"max_tokens": 10,
		"messages": []map[string]string{
			{"role": "user", "content": "test"},
		},
	}

	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Anthropic: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid API key")
	}

	// 4xx errors other than 401 might be expected (rate limit, etc)
	// but connection is valid
	if resp.StatusCode >= 500 {
		return fmt.Errorf("anthropic service error: status %d", resp.StatusCode)
	}

	return nil
}

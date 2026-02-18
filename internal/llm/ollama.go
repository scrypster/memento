package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaClient handles communication with Ollama API for local LLM inference.
// It wraps all HTTP calls with circuit breaker protection to prevent cascading failures.
type OllamaClient struct {
	baseURL        string
	client         *http.Client
	circuitBreaker *CircuitBreaker
	model          string
	timeout        time.Duration
}

// OllamaConfig holds Ollama client configuration.
type OllamaConfig struct {
	// BaseURL is the base URL for the Ollama API (default: http://localhost:11434)
	BaseURL string

	// Model is the model name to use for completions and embeddings (default: phi3:mini)
	Model string

	// Timeout is the request timeout duration (default: 5s)
	Timeout time.Duration
}

// generateRequest represents the request body for /api/generate endpoint
type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// generateResponse represents the response from /api/generate endpoint
type generateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// embedRequest represents the request body for /api/embed endpoint
type embedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// embedResponse represents the response from /api/embed endpoint
// The embeddings field is a 2D array; we always use the first (and only) embedding.
type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// tagsResponse represents the response from /api/tags endpoint
type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// NewOllamaClient creates a new Ollama client with the given configuration.
// If configuration values are not provided, the following defaults are used:
//   - BaseURL: http://localhost:11434
//   - Model: phi3:mini
//   - Timeout: 5 seconds
func NewOllamaClient(config OllamaConfig) *OllamaClient {
	// Apply defaults
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434"
	}
	if config.Model == "" {
		config.Model = "phi3:mini"
	}
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}

	return &OllamaClient{
		baseURL: config.BaseURL,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		circuitBreaker: NewCircuitBreaker(),
		model:          config.Model,
		timeout:        config.Timeout,
	}
}

// Complete sends a completion request to Ollama and returns the response text.
// The request is wrapped with circuit breaker protection and has a 5-second timeout.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - prompt: The text prompt to send to the LLM
//
// Returns:
//   - The completion response text
//   - An error if the request fails or the circuit breaker is open
func (c *OllamaClient) Complete(ctx context.Context, prompt string) (string, error) {
	// Wrap HTTP call with circuit breaker
	result, err := c.circuitBreaker.Execute(ctx, func() (interface{}, error) {
		return c.complete(ctx, prompt)
	})

	if err != nil {
		if errors.Is(err, ErrCircuitOpen) {
			return "", fmt.Errorf("ollama circuit breaker open: %w", err)
		}
		return "", err
	}

	return result.(string), nil
}

// complete is the internal implementation of Complete without circuit breaker wrapping
func (c *OllamaClient) complete(ctx context.Context, prompt string) (string, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Build request body
	reqBody := generateRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false, // We don't support streaming
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/generate", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var respData generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return respData.Response, nil
}

// Embed generates embeddings for the given text using the configured model.
// The request is wrapped with circuit breaker protection and has a 5-second timeout.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - text: The text to generate embeddings for
//
// Returns:
//   - A slice of float32 values representing the embedding vector
//   - An error if the request fails or the circuit breaker is open
func (c *OllamaClient) Embed(ctx context.Context, text string) ([]float32, error) {
	// Wrap HTTP call with circuit breaker
	result, err := c.circuitBreaker.Execute(ctx, func() (interface{}, error) {
		return c.embed(ctx, text)
	})

	if err != nil {
		if errors.Is(err, ErrCircuitOpen) {
			return nil, fmt.Errorf("ollama circuit breaker open: %w", err)
		}
		return nil, err
	}

	return result.([]float32), nil
}

// embed is the internal implementation of Embed without circuit breaker wrapping
func (c *OllamaClient) embed(ctx context.Context, text string) ([]float32, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Build request body
	reqBody := embedRequest{
		Model: c.model,
		Input: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/embed", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var respData embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(respData.Embeddings) == 0 || len(respData.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("ollama returned empty embedding vector")
	}

	return respData.Embeddings[0], nil
}

// HealthCheck verifies that Ollama is reachable by checking the /api/version endpoint.
// This does not use circuit breaker protection since it's a health check itself.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns:
//   - An error if Ollama is unreachable or returns an error
func (c *OllamaClient) HealthCheck(ctx context.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/version", nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health check returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetModel returns the configured model name.
func (c *OllamaClient) GetModel() string {
	return c.model
}

// Compile-time assertions that OllamaClient satisfies both LLM interfaces.
var _ TextGenerator = (*OllamaClient)(nil)
var _ EmbeddingGenerator = (*OllamaClient)(nil)

// ListModels returns a list of available models from Ollama.
// This is useful for discovering what models are installed and can be used.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns:
//   - A slice of model names
//   - An error if the request fails
func (c *OllamaClient) ListModels(ctx context.Context) ([]string, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var respData tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract model names
	models := make([]string, len(respData.Models))
	for i, model := range respData.Models {
		models[i] = model.Name
	}

	return models, nil
}

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

// OpenAIConfig holds configuration for the OpenAI client.
type OpenAIConfig struct {
	APIKey  string
	Model   string        // default: gpt-4o-mini
	BaseURL string        // default: https://api.openai.com
	Timeout time.Duration // default: 60s
}

// OpenAIClient implements TextGenerator using the OpenAI chat completions API.
type OpenAIClient struct {
	cfg            OpenAIConfig
	client         *http.Client
	circuitBreaker *CircuitBreaker
}

// NewOpenAIClient creates a new OpenAI client with the given configuration.
func NewOpenAIClient(cfg OpenAIConfig) *OpenAIClient {
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &OpenAIClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		circuitBreaker: NewCircuitBreaker(),
	}
}

// openAIChatRequest is the request body for POST /v1/chat/completions.
type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature float64             `json:"temperature"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatResponse is the response body from POST /v1/chat/completions.
type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Complete sends a single-turn completion to OpenAI and returns the response text.
func (c *OpenAIClient) Complete(ctx context.Context, prompt string) (string, error) {
	result, err := c.circuitBreaker.Execute(ctx, func() (interface{}, error) {
		return c.complete(ctx, prompt)
	})
	if err != nil {
		if errors.Is(err, ErrCircuitOpen) {
			return "", fmt.Errorf("openai circuit breaker open: %w", err)
		}
		return "", err
	}
	return result.(string), nil
}

func (c *OpenAIClient) complete(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	reqBody := openAIChatRequest{
		Model: c.cfg.Model,
		Messages: []openAIChatMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: 0,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai returned status %d: %s", resp.StatusCode, string(body))
	}

	var respData openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(respData.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}

	return respData.Choices[0].Message.Content, nil
}

// GetModel returns the configured model name.
func (c *OpenAIClient) GetModel() string {
	return c.cfg.Model
}

// Compile-time assertion.
var _ TextGenerator = (*OpenAIClient)(nil)

// OpenAIEmbeddingConfig holds configuration for the OpenAI embedding client.
type OpenAIEmbeddingConfig struct {
	APIKey  string
	Model   string        // default: text-embedding-3-small
	BaseURL string        // default: https://api.openai.com
	Timeout time.Duration // default: 30s
}

// OpenAIEmbeddingClient implements EmbeddingGenerator using the OpenAI embeddings API.
type OpenAIEmbeddingClient struct {
	cfg            OpenAIEmbeddingConfig
	client         *http.Client
	circuitBreaker *CircuitBreaker
}

// NewOpenAIEmbeddingClient creates a new OpenAI embedding client.
func NewOpenAIEmbeddingClient(cfg OpenAIEmbeddingConfig) *OpenAIEmbeddingClient {
	if cfg.Model == "" {
		cfg.Model = "text-embedding-3-small"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &OpenAIEmbeddingClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		circuitBreaker: NewCircuitBreaker(),
	}
}

// openAIEmbeddingRequest is the request body for POST /v1/embeddings.
type openAIEmbeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// openAIEmbeddingResponse is the response body from POST /v1/embeddings.
type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// Embed generates an embedding vector for the given text.
func (c *OpenAIEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	result, err := c.circuitBreaker.Execute(ctx, func() (interface{}, error) {
		return c.embed(ctx, text)
	})
	if err != nil {
		if errors.Is(err, ErrCircuitOpen) {
			return nil, fmt.Errorf("openai embedding circuit breaker open: %w", err)
		}
		return nil, err
	}
	return result.([]float32), nil
}

func (c *OpenAIEmbeddingClient) embed(ctx context.Context, text string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	reqBody := openAIEmbeddingRequest{
		Model: c.cfg.Model,
		Input: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL+"/v1/embeddings", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai returned status %d: %s", resp.StatusCode, string(body))
	}

	var respData openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(respData.Data) == 0 || len(respData.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("openai returned empty embedding")
	}

	// Convert []float64 to []float32
	raw := respData.Data[0].Embedding
	vec := make([]float32, len(raw))
	for i, v := range raw {
		vec[i] = float32(v)
	}
	return vec, nil
}

// GetModel returns the configured model name.
func (c *OpenAIEmbeddingClient) GetModel() string {
	return c.cfg.Model
}

// Compile-time assertion.
var _ EmbeddingGenerator = (*OpenAIEmbeddingClient)(nil)

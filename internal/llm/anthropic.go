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

// AnthropicConfig holds configuration for the Anthropic client.
type AnthropicConfig struct {
	APIKey  string
	Model   string        // default: claude-haiku-4-5-20251001
	Timeout time.Duration // default: 60s
}

// AnthropicClient implements TextGenerator using the Anthropic Messages API.
type AnthropicClient struct {
	cfg            AnthropicConfig
	client         *http.Client
	circuitBreaker *CircuitBreaker
}

// NewAnthropicClient creates a new Anthropic client with the given configuration.
func NewAnthropicClient(cfg AnthropicConfig) *AnthropicClient {
	if cfg.Model == "" {
		cfg.Model = "claude-haiku-4-5-20251001"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &AnthropicClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		circuitBreaker: NewCircuitBreaker(),
	}
}

// anthropicMessagesRequest is the request body for POST /v1/messages.
type anthropicMessagesRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicMessagesResponse is the response body from POST /v1/messages.
type anthropicMessagesResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

// Complete sends a single-turn completion to Anthropic and returns the response text.
func (c *AnthropicClient) Complete(ctx context.Context, prompt string) (string, error) {
	result, err := c.circuitBreaker.Execute(ctx, func() (interface{}, error) {
		return c.complete(ctx, prompt)
	})
	if err != nil {
		if errors.Is(err, ErrCircuitOpen) {
			return "", fmt.Errorf("anthropic circuit breaker open: %w", err)
		}
		return "", err
	}
	return result.(string), nil
}

func (c *AnthropicClient) complete(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	reqBody := anthropicMessagesRequest{
		Model:     c.cfg.Model,
		MaxTokens: 4096,
		Messages: []anthropicMessage{
			{Role: "user", Content: prompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic returned status %d: %s", resp.StatusCode, string(body))
	}

	var respData anthropicMessagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(respData.Content) == 0 {
		return "", fmt.Errorf("anthropic returned empty content")
	}

	return respData.Content[0].Text, nil
}

// GetModel returns the configured model name.
func (c *AnthropicClient) GetModel() string {
	return c.cfg.Model
}

// Compile-time assertion.
var _ TextGenerator = (*AnthropicClient)(nil)

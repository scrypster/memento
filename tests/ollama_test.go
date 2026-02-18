package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/llm"
)

// mockOllamaServer creates a test HTTP server that simulates Ollama API endpoints
func mockOllamaServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"response": "Test response from Ollama",
				"done":     true,
			})
		case "/api/embed":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"embeddings": [][]float32{{0.1, 0.2, 0.3, 0.4, 0.5}},
			})
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"models": []map[string]string{
					{"name": "phi3:mini"},
					{"name": "llama3:8b"},
				},
			})
		case "/api/version":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"version": "0.1.0",
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

// mockSlowOllamaServer creates a test server that responds slowly to test timeouts
func mockSlowOllamaServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(6 * time.Second) // Longer than the 5s timeout
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"response": "Too late",
			"done":     true,
		})
	}))
}

// mockFailingOllamaServer creates a test server that always returns errors
func mockFailingOllamaServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
	}))
}

// mockInvalidJSONServer creates a test server that returns malformed JSON
func mockInvalidJSONServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{invalid json"))
	}))
}

// TestOllamaClient_Complete verifies that Complete sends a prompt and returns a response
func TestOllamaClient_Complete(t *testing.T) {
	server := mockOllamaServer()
	defer server.Close()

	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	response, err := client.Complete(ctx, "What is the capital of France?")
	if err != nil {
		t.Fatalf("Expected successful completion, got error: %v", err)
	}

	if response != "Test response from Ollama" {
		t.Fatalf("Expected 'Test response from Ollama', got: %s", response)
	}
}

// TestOllamaClient_Embed verifies that Embed generates embeddings
func TestOllamaClient_Embed(t *testing.T) {
	server := mockOllamaServer()
	defer server.Close()

	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	embeddings, err := client.Embed(ctx, "Hello world")
	if err != nil {
		t.Fatalf("Expected successful embedding, got error: %v", err)
	}

	if len(embeddings) != 5 {
		t.Fatalf("Expected 5 embedding values, got: %d", len(embeddings))
	}

	expected := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	for i, val := range embeddings {
		if val != expected[i] {
			t.Fatalf("Expected embedding[%d] = %f, got: %f", i, expected[i], val)
		}
	}
}

// TestOllamaClient_HealthCheck verifies that HealthCheck succeeds when Ollama is reachable
func TestOllamaClient_HealthCheck(t *testing.T) {
	server := mockOllamaServer()
	defer server.Close()

	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("Expected successful health check, got error: %v", err)
	}
}

// TestOllamaClient_HealthCheckFailure verifies that HealthCheck fails when Ollama is unreachable
func TestOllamaClient_HealthCheckFailure(t *testing.T) {
	config := llm.OllamaConfig{
		BaseURL: "http://localhost:99999", // Invalid port
		Model:   "phi3:mini",
		Timeout: 1 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.HealthCheck(ctx)
	if err == nil {
		t.Fatal("Expected health check to fail with unreachable server")
	}
}

// TestOllamaClient_Timeout verifies that requests timeout after 5 seconds
func TestOllamaClient_Timeout(t *testing.T) {
	server := mockSlowOllamaServer()
	defer server.Close()

	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	start := time.Now()
	_, err := client.Complete(ctx, "Test prompt")
	duration := time.Since(start)

	if err == nil {
		t.Fatal("Expected timeout error, got success")
	}

	// Should timeout around 5 seconds (allow some margin)
	if duration < 4*time.Second || duration > 7*time.Second {
		t.Fatalf("Expected timeout around 5s, took: %v", duration)
	}

	// Error should be a context deadline exceeded
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Expected context.DeadlineExceeded, got: %v", err)
	}
}

// TestOllamaClient_CircuitBreaker verifies that circuit breaker protection works
func TestOllamaClient_CircuitBreaker(t *testing.T) {
	server := mockFailingOllamaServer()
	defer server.Close()

	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// Trigger 3 consecutive failures to open the circuit
	for i := 0; i < 3; i++ {
		_, err := client.Complete(ctx, "Test prompt")
		if err == nil {
			t.Fatalf("Expected error on attempt %d", i+1)
		}
	}

	// Circuit should now be open, next request should be rejected immediately
	start := time.Now()
	_, err := client.Complete(ctx, "Test prompt")
	duration := time.Since(start)

	if err == nil {
		t.Fatal("Expected circuit breaker to reject request")
	}

	// Should be rejected immediately (< 100ms)
	if duration > 100*time.Millisecond {
		t.Fatalf("Expected immediate rejection, took: %v", duration)
	}

	if !errors.Is(err, llm.ErrCircuitOpen) {
		t.Fatalf("Expected ErrCircuitOpen, got: %v", err)
	}
}

// TestOllamaClient_ListModels verifies that ListModels returns available models
func TestOllamaClient_ListModels(t *testing.T) {
	server := mockOllamaServer()
	defer server.Close()

	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	models, err := client.ListModels(ctx)
	if err != nil {
		t.Fatalf("Expected successful model listing, got error: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("Expected 2 models, got: %d", len(models))
	}

	expectedModels := []string{"phi3:mini", "llama3:8b"}
	for i, model := range models {
		if model != expectedModels[i] {
			t.Fatalf("Expected model %s, got: %s", expectedModels[i], model)
		}
	}
}

// TestOllamaClient_InvalidResponse verifies error handling for malformed JSON
func TestOllamaClient_InvalidResponse(t *testing.T) {
	server := mockInvalidJSONServer()
	defer server.Close()

	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	_, err := client.Complete(ctx, "Test prompt")
	if err == nil {
		t.Fatal("Expected error for invalid JSON response")
	}
}

// TestOllamaClient_ContextCancellation verifies that context cancellation is respected
func TestOllamaClient_ContextCancellation(t *testing.T) {
	server := mockOllamaServer()
	defer server.Close()

	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Complete(ctx, "Test prompt")
	if err == nil {
		t.Fatal("Expected error due to cancelled context")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Expected context.Canceled, got: %v", err)
	}
}

// TestOllamaClient_DefaultConfig verifies that default configuration values are set correctly
func TestOllamaClient_DefaultConfig(t *testing.T) {
	server := mockOllamaServer()
	defer server.Close()

	// Create client with minimal config (defaults should be applied)
	config := llm.OllamaConfig{
		BaseURL: server.URL,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// Should work with defaults
	_, err := client.Complete(ctx, "Test prompt")
	if err != nil {
		t.Fatalf("Expected successful completion with default config, got error: %v", err)
	}
}

// TestOllamaClient_EmptyPrompt verifies handling of empty prompts
func TestOllamaClient_EmptyPrompt(t *testing.T) {
	server := mockOllamaServer()
	defer server.Close()

	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// Should still work with empty prompt (server handles it)
	response, err := client.Complete(ctx, "")
	if err != nil {
		t.Fatalf("Expected completion with empty prompt, got error: %v", err)
	}

	// Response should still be valid
	if response == "" {
		t.Fatal("Expected non-empty response even with empty prompt")
	}
}

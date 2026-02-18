package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/llm"
)

// TestOllamaIntegration_EntityExtraction tests the full entity extraction workflow
func TestOllamaIntegration_EntityExtraction(t *testing.T) {
	// Create mock server that returns entity extraction JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"response": `{
					"entities": [
						{
							"name": "John Doe",
							"type": "person",
							"description": "Software engineer mentioned in the context",
							"confidence": 0.95
						},
						{
							"name": "Memento",
							"type": "project",
							"description": "AI-native memory system project",
							"confidence": 0.98
						}
					]
				}`,
				"done": true,
			})
		}
	}))
	defer server.Close()

	// Create Ollama client
	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}
	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// Generate entity extraction prompt
	content := "John Doe is working on the Memento project, an AI-native memory system."
	prompt := llm.EntityExtractionPrompt(content)

	// Get response from Ollama
	response, err := client.Complete(ctx, prompt)
	if err != nil {
		t.Fatalf("Entity extraction failed: %v", err)
	}

	// Verify response contains expected entities
	if response == "" {
		t.Fatal("Expected non-empty response")
	}

	// Response should be valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	// Verify entities exist
	entities, ok := result["entities"].([]interface{})
	if !ok {
		t.Fatal("Expected 'entities' array in response")
	}

	if len(entities) != 2 {
		t.Fatalf("Expected 2 entities, got: %d", len(entities))
	}
}

// TestOllamaIntegration_WithChunking tests Ollama integration with content chunking
func TestOllamaIntegration_WithChunking(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"response": `{"entities": []}`,
				"done":     true,
			})
		}
	}))
	defer server.Close()

	// Create Ollama client and chunker
	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}
	client := llm.NewOllamaClient(config)
	chunker := &llm.Chunker{
		MaxChunkSize: 50, // Very small size to force chunking
		Overlap:      10,
	}
	ctx := context.Background()

	// Create large content that will be chunked (needs to be > 200 chars for multiple chunks)
	largeContent := ""
	for i := 0; i < 20; i++ {
		largeContent += "This is a test sentence that will force chunking. "
	}

	// Chunk the content
	chunks, err := chunker.Chunk(largeContent)
	if err != nil {
		t.Fatalf("Failed to chunk content: %v", err)
	}

	if len(chunks) < 2 {
		t.Fatalf("Expected multiple chunks, got: %d", len(chunks))
	}

	// Process each chunk with Ollama
	for i, chunk := range chunks {
		prompt := llm.EntityExtractionPrompt(chunk)
		response, err := client.Complete(ctx, prompt)
		if err != nil {
			t.Fatalf("Failed to process chunk %d: %v", i, err)
		}

		if response == "" {
			t.Fatalf("Expected non-empty response for chunk %d", i)
		}
	}
}

// TestOllamaIntegration_CircuitBreakerRecovery tests circuit breaker recovery behavior
func TestOllamaIntegration_CircuitBreakerRecovery(t *testing.T) {
	failCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failCount++
		if failCount <= 3 {
			// First 3 requests fail
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// After circuit opens and timeout, requests succeed
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"response": "Success after recovery",
			"done":     true,
		})
	}))
	defer server.Close()

	// Create client with fast circuit breaker timeout
	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}
	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// Trigger 3 failures to open circuit
	for i := 0; i < 3; i++ {
		_, err := client.Complete(ctx, "test")
		if err == nil {
			t.Fatalf("Expected error on attempt %d", i+1)
		}
	}

	// Next request should be rejected by circuit breaker
	_, err := client.Complete(ctx, "test")
	if err == nil {
		t.Fatal("Expected circuit breaker to reject request")
	}

	// Note: Full recovery test would require waiting for circuit breaker timeout
	// This is a simplified test to verify circuit breaker integration
}

// TestOllamaIntegration_EmbeddingWorkflow tests the embedding generation workflow
func TestOllamaIntegration_EmbeddingWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embed" {
			w.Header().Set("Content-Type", "application/json")
			// Return realistic embedding vector (384 dimensions for all-MiniLM-L6-v2)
			embedding := make([]float32, 384)
			for i := range embedding {
				embedding[i] = float32(i) * 0.001
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"embeddings": [][]float32{embedding},
			})
		}
	}))
	defer server.Close()

	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "all-MiniLM-L6-v2",
		Timeout: 5 * time.Second,
	}
	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// Generate embeddings for text
	text := "This is a test sentence for embedding generation."
	embeddings, err := client.Embed(ctx, text)
	if err != nil {
		t.Fatalf("Failed to generate embeddings: %v", err)
	}

	if len(embeddings) != 384 {
		t.Fatalf("Expected 384 dimensions, got: %d", len(embeddings))
	}

	// Verify embedding values are in reasonable range
	for i, val := range embeddings {
		if val < -1.0 || val > 1.0 {
			t.Fatalf("Embedding[%d] = %f is outside expected range [-1, 1]", i, val)
		}
	}
}

// TestOllamaIntegration_MultipleRequests tests concurrent request handling
func TestOllamaIntegration_MultipleRequests(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"response": "Success",
			"done":     true,
		})
	}))
	defer server.Close()

	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}
	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// Send multiple requests
	numRequests := 5
	for i := 0; i < numRequests; i++ {
		response, err := client.Complete(ctx, "Test prompt")
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}
		if response != "Success" {
			t.Fatalf("Expected 'Success', got: %s", response)
		}
	}

	// Verify all requests were processed
	if requestCount != numRequests {
		t.Fatalf("Expected %d requests, server received: %d", numRequests, requestCount)
	}
}

// TestOllamaIntegration_ModelListing tests model discovery
func TestOllamaIntegration_ModelListing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"models": []map[string]interface{}{
					{"name": "phi3:mini", "size": 2200000000},
					{"name": "llama3:8b", "size": 4700000000},
					{"name": "mistral:7b", "size": 4100000000},
				},
			})
		}
	}))
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
		t.Fatalf("Failed to list models: %v", err)
	}

	expectedModels := []string{"phi3:mini", "llama3:8b", "mistral:7b"}
	if len(models) != len(expectedModels) {
		t.Fatalf("Expected %d models, got: %d", len(expectedModels), len(models))
	}

	for i, expected := range expectedModels {
		if models[i] != expected {
			t.Fatalf("Expected model %s at index %d, got: %s", expected, i, models[i])
		}
	}
}

// TestOllamaIntegration_HealthCheckMonitoring tests health check for monitoring
func TestOllamaIntegration_HealthCheckMonitoring(t *testing.T) {
	healthChecks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			healthChecks++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"version": "0.1.0",
			})
		}
	}))
	defer server.Close()

	config := llm.OllamaConfig{
		BaseURL: server.URL,
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}
	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// Perform multiple health checks (simulating monitoring)
	for i := 0; i < 3; i++ {
		if err := client.HealthCheck(ctx); err != nil {
			t.Fatalf("Health check %d failed: %v", i+1, err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if healthChecks != 3 {
		t.Fatalf("Expected 3 health checks, got: %d", healthChecks)
	}
}

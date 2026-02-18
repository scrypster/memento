package llm_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/scrypster/memento/internal/llm"
)

// ExampleOllamaClient_Complete demonstrates how to use the OllamaClient for text completion.
func ExampleOllamaClient_Complete() {
	// Create Ollama client with custom configuration
	config := llm.OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// Send a completion request
	response, err := client.Complete(ctx, "What is the capital of France?")
	if err != nil {
		log.Fatalf("Failed to complete: %v", err)
	}

	fmt.Println(response)
}

// ExampleOllamaClient_Embed demonstrates how to generate embeddings.
func ExampleOllamaClient_Embed() {
	config := llm.OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// Generate embeddings for text
	embeddings, err := client.Embed(ctx, "Hello, world!")
	if err != nil {
		log.Fatalf("Failed to generate embeddings: %v", err)
	}

	fmt.Printf("Generated %d-dimensional embedding vector\n", len(embeddings))
}

// ExampleOllamaClient_HealthCheck demonstrates how to check if Ollama is reachable.
func ExampleOllamaClient_HealthCheck() {
	config := llm.OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// Check if Ollama is running and reachable
	if err := client.HealthCheck(ctx); err != nil {
		log.Fatalf("Ollama health check failed: %v", err)
	}

	fmt.Println("Ollama is healthy")
}

// ExampleOllamaClient_ListModels demonstrates how to list available models.
func ExampleOllamaClient_ListModels() {
	config := llm.OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// List all available models
	models, err := client.ListModels(ctx)
	if err != nil {
		log.Fatalf("Failed to list models: %v", err)
	}

	fmt.Printf("Available models: %v\n", models)
}

// ExampleOllamaClient_withEntityExtraction demonstrates integration with entity extraction prompts.
func ExampleOllamaClient_withEntityExtraction() {
	config := llm.OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// Use entity extraction prompt
	content := "John Doe works on the Memento project at Anthropic."
	prompt := llm.EntityExtractionPrompt(content)

	response, err := client.Complete(ctx, prompt)
	if err != nil {
		log.Fatalf("Failed to extract entities: %v", err)
	}

	fmt.Println(response)
	// Expected: JSON response with extracted entities
}

// ExampleOllamaClient_withCircuitBreaker demonstrates circuit breaker protection.
func ExampleOllamaClient_withCircuitBreaker() {
	config := llm.OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	ctx := context.Background()

	// The circuit breaker automatically protects against cascading failures
	// After 3 consecutive failures, the circuit opens and rejects requests
	for i := 0; i < 5; i++ {
		_, err := client.Complete(ctx, "Test prompt")
		if err != nil {
			if err == llm.ErrCircuitOpen {
				fmt.Println("Circuit breaker is open, request rejected")
				break
			}
			fmt.Printf("Request failed: %v\n", err)
		}
	}
}

// ExampleOllamaClient_withChunking demonstrates combining Ollama with the chunker for large content.
func ExampleOllamaClient_withChunking() {
	config := llm.OllamaConfig{
		BaseURL: "http://localhost:11434",
		Model:   "phi3:mini",
		Timeout: 5 * time.Second,
	}

	client := llm.NewOllamaClient(config)
	chunker := &llm.Chunker{
		MaxChunkSize: 3000,
		Overlap:      200,
	}
	ctx := context.Background()

	// Process large content in chunks
	largeContent := "..." // Large text content
	chunks, err := chunker.Chunk(largeContent)
	if err != nil {
		log.Fatalf("Failed to chunk content: %v", err)
	}

	// Process each chunk with entity extraction
	for i, chunk := range chunks {
		prompt := llm.EntityExtractionPrompt(chunk)
		response, err := client.Complete(ctx, prompt)
		if err != nil {
			log.Printf("Failed to process chunk %d: %v", i, err)
			continue
		}
		fmt.Printf("Chunk %d: %s\n", i, response)
	}
}

// ExampleOllamaClient_defaultConfig demonstrates using default configuration.
func ExampleOllamaClient_defaultConfig() {
	// Create client with defaults (localhost:11434, phi3:mini, 5s timeout)
	client := llm.NewOllamaClient(llm.OllamaConfig{})
	ctx := context.Background()

	// Health check with defaults
	if err := client.HealthCheck(ctx); err != nil {
		log.Fatalf("Health check failed: %v", err)
	}

	fmt.Println("Using default configuration")
}

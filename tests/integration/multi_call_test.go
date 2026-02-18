// +build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/llm"
)

// TestMultiCallStrategies tests different multi-call extraction strategies
func TestMultiCallStrategies(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Setup
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen2.5:7b"
	}

	baseURL := os.Getenv("OLLAMA_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	config := llm.OllamaConfig{
		BaseURL: baseURL,
		Model:   model,
		Timeout: 30 * time.Second,
	}

	client := llm.NewOllamaClient(config)

	t.Logf("Testing with model: %s\n", model)
	t.Logf("Base URL: %s\n", baseURL)

	// Run multi-call tests
	for _, test := range llm.MultiCallTests {
		t.Run(test.Name, func(t *testing.T) {
			t.Logf("\nTesting: %s", test.Name)
			t.Logf("Description: %s", test.Description)

			result := llm.RunMultiCallTest(test, client)
			formatted := llm.FormatTestResults(result)
			t.Log(formatted)

			if !result.Success {
				t.Logf("⚠️  Some calls failed parsing, but multi-call completed")
			}
		})
	}
}

// TestMultiCallWithDifferentModels tests strategies across models
func TestMultiCallWithDifferentModels(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	models := []string{"qwen2.5:7b", "phi3:mini"}
	baseURL := os.Getenv("OLLAMA_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	for _, model := range models {
		t.Run("Model_"+model, func(t *testing.T) {
			config := llm.OllamaConfig{
				BaseURL: baseURL,
				Model:   model,
				Timeout: 30 * time.Second,
			}

			client := llm.NewOllamaClient(config)

			t.Logf("\n=== Testing Multi-Call with %s ===\n", model)

			// Test the two-call split strategy
			twoCallTest := llm.MultiCallTest{
				Name:        "Two-Call Split (Entities + Relationships)",
				Description: "Call 1: Extract entities, Call 2: Extract relationships",
				Calls: []llm.PromptCall{
					{
						Name:    "V1_Entities",
						Purpose: "entity_extraction",
						PromptFunc: func(content string) string {
							return llm.EntityExtractionPrompt(content)
						},
					},
					{
						Name:    "V1_Relationships",
						Purpose: "relationship_extraction",
						PromptFunc: func(content string) string {
							return llm.RelationshipExtractionPrompt(content, []llm.Entity{})
						},
					},
				},
				TestContent: llm.TestPayloads["complex"],
			}

			result := llm.RunMultiCallTest(twoCallTest, client)
			t.Log(llm.FormatTestResults(result))

			// Report summary
			t.Logf("\nSummary for %s:", model)
			passCount := 0
			for _, res := range result.Results {
				if res.ParsedOK {
					passCount++
					t.Logf("  ✅ %s: %.2fs", res.CallName, res.Duration.Seconds())
				} else {
					t.Logf("  ❌ %s: %s", res.CallName, res.ErrorMsg)
				}
			}
			t.Logf("  Total: %d/%d calls succeeded", passCount, len(result.Results))
		})
	}
}

// TestMultiCallPerformance compares single-call vs multi-call performance
func TestMultiCallPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen2.5:7b"
	}

	baseURL := os.Getenv("OLLAMA_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	config := llm.OllamaConfig{
		BaseURL: baseURL,
		Model:   model,
		Timeout: 30 * time.Second,
	}

	client := llm.NewOllamaClient(config)

	t.Logf("Performance Comparison: %s\n", model)

	// Single call (entity extraction only)
	testContent := llm.TestPayloads["complex"]
	prompt := llm.EntityExtractionPrompt(testContent)

	start := time.Now()
	_, err := client.Complete(ctx, prompt)
	singleCallTime := time.Since(start)

	t.Logf("\n1. Single Call (Entity Extraction):")
	t.Logf("   Time: %.2fs", singleCallTime.Seconds())
	t.Logf("   Status: %s", func() string {
		if err != nil {
			return "❌ Error: " + err.Error()
		}
		return "✅ Success"
	}())

	// Two calls (entities + relationships)
	t.Logf("\n2. Two Calls (Entities + Relationships):")
	start = time.Now()
	_, err1 := client.Complete(ctx, llm.EntityExtractionPrompt(testContent))
	time.Sleep(500 * time.Millisecond) // Brief pause between calls
	_, err2 := client.Complete(ctx, llm.RelationshipExtractionPrompt(testContent, []llm.Entity{}))
	multiCallTime := time.Since(start)

	t.Logf("   Total Time: %.2fs", multiCallTime.Seconds())
	t.Logf("   Call 1 Status: %s", func() string {
		if err1 != nil {
			return "❌"
		}
		return "✅"
	}())
	t.Logf("   Call 2 Status: %s", func() string {
		if err2 != nil {
			return "❌"
		}
		return "✅"
	}())

	// Compare
	overhead := multiCallTime - singleCallTime
	t.Logf("\n📊 Comparison:")
	t.Logf("   Single Call:  %.2fs", singleCallTime.Seconds())
	t.Logf("   Multi Call:   %.2fs", multiCallTime.Seconds())
	t.Logf("   Overhead:     %.2fs (%.0f%%)", overhead.Seconds(), (float64(overhead)/float64(singleCallTime))*100)
	t.Logf("\n   Conclusion: Multi-call adds ~%.2fs overhead for focused extraction", overhead.Seconds())
}

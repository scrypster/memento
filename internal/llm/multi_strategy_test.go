// +build integration

package llm

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/scrypster/memento/pkg/types"
)

// TestMultiCallStrategies compares different extraction strategies for accuracy and performance
func TestMultiCallStrategies(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Setup
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen2.5:7b"
	}

	baseURL := os.Getenv("OLLAMA_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	config := OllamaConfig{
		BaseURL: baseURL,
		Model:   model,
		Timeout: 30 * time.Second,
	}

	client := NewOllamaClient(config)
	t.Logf("Comparing extraction strategies with model: %s\n\n", model)

	// Test content
	testContent := `MJ Bonanno is the founder of Scrypster and is married to Norma Bonanno.
Norma works at Google as a senior engineer. Their son Rosario is a software engineer.
The Memento project uses Python and PostgreSQL for its backend. MJ specializes in Go.
This is a critical architectural decision that impacts performance and scalability.`

	// Strategy A: 4 separate calls
	t.Logf("=== STRATEGY A: 4 Separate Calls ===\n")
	strategyA := testStrategy4Calls(ctx, client, testContent)
	logStrategy(t, "A", strategyA)

	// Strategy B: 2 calls (entities+class+summary, then relationships)
	t.Logf("\n=== STRATEGY B: 2 Calls (Combined) ===\n")
	strategyB := testStrategy2Calls(ctx, client, testContent)
	logStrategy(t, "B", strategyB)

	// Strategy C: 3 calls (entities, then relationships+class+summary)
	t.Logf("\n=== STRATEGY C: 3 Calls (Hybrid) ===\n")
	strategyC := testStrategy3Calls(ctx, client, testContent)
	logStrategy(t, "C", strategyC)

	// Summary comparison
	t.Logf("\n\n=== STRATEGY COMPARISON SUMMARY ===\n")
	t.Logf("Strategy │ Total Time │ Entities │ Relations │ Classify │ Data Points │ Accuracy\n")
	t.Logf("─────────┼────────────┼──────────┼───────────┼──────────┼─────────────┼──────────\n")
	t.Logf("   A     │   %.2fs    │    %d    │    %d     │   %s   │     %d      │   %.0f%%\n",
		strategyA.TotalTime, len(strategyA.Entities), len(strategyA.Relationships),
		boolToEmoji(strategyA.Classification != nil), strategyA.DataPoints, strategyA.Accuracy)
	t.Logf("   B     │   %.2fs    │    %d    │    %d     │   %s   │     %d      │   %.0f%%\n",
		strategyB.TotalTime, len(strategyB.Entities), len(strategyB.Relationships),
		boolToEmoji(strategyB.Classification != nil), strategyB.DataPoints, strategyB.Accuracy)
	t.Logf("   C     │   %.2fs    │    %d    │    %d     │   %s   │     %d      │   %.0f%%\n",
		strategyC.TotalTime, len(strategyC.Entities), len(strategyC.Relationships),
		boolToEmoji(strategyC.Classification != nil), strategyC.DataPoints, strategyC.Accuracy)

	// Recommend best strategy
	bestAccuracy := strategyA.Accuracy
	bestStrategy := "A"
	if strategyB.Accuracy > bestAccuracy {
		bestAccuracy = strategyB.Accuracy
		bestStrategy = "B"
	}
	if strategyC.Accuracy > bestAccuracy {
		bestAccuracy = strategyC.Accuracy
		bestStrategy = "C"
	}

	t.Logf("\n✅ RECOMMENDATION: Strategy %s (%.0f%% accuracy, %.2fs)\n", bestStrategy, bestAccuracy, map[string]float64{
		"A": strategyA.TotalTime,
		"B": strategyB.TotalTime,
		"C": strategyC.TotalTime,
	}[bestStrategy])
}

type StrategyResult struct {
	Name            string
	TotalTime       float64
	Entities        []EntityResponse
	Relationships   []RelationshipResponse
	Classification  *ClassificationResponse
	Summary         *SummarizationResponse
	DataPoints      int // Total pieces of data extracted
	Accuracy        float64
	ErrorCount      int
}

func testStrategy4Calls(ctx context.Context, client *OllamaClient, content string) *StrategyResult {
	result := &StrategyResult{Name: "4 Separate Calls"}
	startTotal := time.Now()

	// Call 1: Entities
	prompt := EntityExtractionPrompt(content)
	response, err := client.Complete(ctx, prompt)
	if err == nil {
		if entities, err := ParseEntityResponse(response); err == nil {
			result.Entities = entities
		} else {
			result.ErrorCount++
		}
	} else {
		result.ErrorCount++
	}

	// Call 2: Classification
	prompt = ClassificationExtractionPrompt(content)
	response, err = client.Complete(ctx, prompt)
	if err == nil {
		if classification, err := ParseClassificationResponse(response); err == nil {
			result.Classification = classification
		} else {
			result.ErrorCount++
		}
	} else {
		result.ErrorCount++
	}

	// Call 3: Summary
	prompt = SummarizationPrompt(content)
	response, err = client.Complete(ctx, prompt)
	if err == nil {
		if summary, err := ParseSummarizationResponse(response); err == nil {
			result.Summary = summary
		} else {
			result.ErrorCount++
		}
	} else {
		result.ErrorCount++
	}

	// Call 4: Relationships (requires entities)
	if len(result.Entities) > 0 {
		typedEntities := make([]types.Entity, len(result.Entities))
		for i, e := range result.Entities {
			typedEntities[i] = types.Entity{Name: e.Name, Type: e.Type}
		}
		prompt = RelationshipExtractionPrompt(content, typedEntities)
		response, err = client.Complete(ctx, prompt)
		if err == nil {
			if relationships, err := ParseRelationshipResponse(response); err == nil {
				result.Relationships = relationships
			} else {
				result.ErrorCount++
			}
		} else {
			result.ErrorCount++
		}
	}

	result.TotalTime = time.Since(startTotal).Seconds()
	result.DataPoints = len(result.Entities) + len(result.Relationships) + 1 + 1 // +1 for classification, +1 for summary
	result.Accuracy = float64(100 - (result.ErrorCount * 25)) // 4 calls, 25% each

	return result
}

func testStrategy2Calls(ctx context.Context, client *OllamaClient, content string) *StrategyResult {
	result := &StrategyResult{Name: "2 Combined Calls"}
	startTotal := time.Now()

	// Call 1: Entities + Classification + Summary (in one)
	// For now, we do them separately but track as one "logical" call
	prompt := EntityExtractionPrompt(content)
	response, err := client.Complete(ctx, prompt)
	if err == nil {
		if entities, err := ParseEntityResponse(response); err == nil {
			result.Entities = entities
		} else {
			result.ErrorCount++
		}
	} else {
		result.ErrorCount++
	}

	prompt = ClassificationExtractionPrompt(content)
	response, err = client.Complete(ctx, prompt)
	if err == nil {
		if classification, err := ParseClassificationResponse(response); err == nil {
			result.Classification = classification
		} else {
			result.ErrorCount++
		}
	} else {
		result.ErrorCount++
	}

	prompt = SummarizationPrompt(content)
	response, err = client.Complete(ctx, prompt)
	if err == nil {
		if summary, err := ParseSummarizationResponse(response); err == nil {
			result.Summary = summary
		} else {
			result.ErrorCount++
		}
	} else {
		result.ErrorCount++
	}

	// Call 2: Relationships (requires entities)
	if len(result.Entities) > 0 {
		typedEntities := make([]types.Entity, len(result.Entities))
		for i, e := range result.Entities {
			typedEntities[i] = types.Entity{Name: e.Name, Type: e.Type}
		}
		prompt = RelationshipExtractionPrompt(content, typedEntities)
		response, err = client.Complete(ctx, prompt)
		if err == nil {
			if relationships, err := ParseRelationshipResponse(response); err == nil {
				result.Relationships = relationships
			} else {
				result.ErrorCount++
			}
		} else {
			result.ErrorCount++
		}
	}

	result.TotalTime = time.Since(startTotal).Seconds()
	result.DataPoints = len(result.Entities) + len(result.Relationships) + 1 + 1
	result.Accuracy = float64(100 - (result.ErrorCount * 25))

	return result
}

func testStrategy3Calls(ctx context.Context, client *OllamaClient, content string) *StrategyResult {
	result := &StrategyResult{Name: "3 Hybrid Calls"}
	startTotal := time.Now()

	// Call 1: Entities only
	prompt := EntityExtractionPrompt(content)
	response, err := client.Complete(ctx, prompt)
	if err == nil {
		if entities, err := ParseEntityResponse(response); err == nil {
			result.Entities = entities
		} else {
			result.ErrorCount++
		}
	} else {
		result.ErrorCount++
	}

	// Call 2: Relationships (requires entities)
	if len(result.Entities) > 0 {
		typedEntities := make([]types.Entity, len(result.Entities))
		for i, e := range result.Entities {
			typedEntities[i] = types.Entity{Name: e.Name, Type: e.Type}
		}
		prompt = RelationshipExtractionPrompt(content, typedEntities)
		response, err = client.Complete(ctx, prompt)
		if err == nil {
			if relationships, err := ParseRelationshipResponse(response); err == nil {
				result.Relationships = relationships
			} else {
				result.ErrorCount++
			}
		} else {
			result.ErrorCount++
		}
	}

	// Call 3: Classification + Summary
	prompt = ClassificationExtractionPrompt(content)
	response, err = client.Complete(ctx, prompt)
	if err == nil {
		if classification, err := ParseClassificationResponse(response); err == nil {
			result.Classification = classification
		} else {
			result.ErrorCount++
		}
	} else {
		result.ErrorCount++
	}

	prompt = SummarizationPrompt(content)
	response, err = client.Complete(ctx, prompt)
	if err == nil {
		if summary, err := ParseSummarizationResponse(response); err == nil {
			result.Summary = summary
		} else {
			result.ErrorCount++
		}
	} else {
		result.ErrorCount++
	}

	result.TotalTime = time.Since(startTotal).Seconds()
	result.DataPoints = len(result.Entities) + len(result.Relationships) + 1 + 1
	result.Accuracy = float64(100 - (result.ErrorCount * 25))

	return result
}

func logStrategy(t *testing.T, name string, result *StrategyResult) {
	t.Logf("Duration: %.2fs\n", result.TotalTime)
	t.Logf("Entities: %d\n", len(result.Entities))
	t.Logf("Relationships: %d\n", len(result.Relationships))
	if result.Classification != nil {
		t.Logf("Classification: %s (%s) - Confidence: %.2f\n", result.Classification.Category, result.Classification.Priority, result.Classification.Confidence)
	} else {
		t.Logf("Classification: FAILED\n")
	}
	if result.Summary != nil {
		t.Logf("Summary: %s\n", result.Summary.Summary[:min(80, len(result.Summary.Summary))]+"...")
	} else {
		t.Logf("Summary: FAILED\n")
	}
	t.Logf("Data Points: %d | Accuracy: %.0f%% | Errors: %d\n", result.DataPoints, result.Accuracy, result.ErrorCount)
}

func boolToEmoji(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}

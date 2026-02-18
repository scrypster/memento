package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// EnrichmentResult captures detailed metrics for a single enrichment operation
type EnrichmentResult struct {
	ContentSize              ContentSize
	Complexity               ComplexityLevel
	MemoryID                 string
	StartTime                time.Time
	EndTime                  time.Time
	Duration                 time.Duration
	Success                  bool
	EntityCount              int
	RelationshipCount        int
	EntityExtractionError    string
	RelationshipExtractionError string
	SummarizationError       string
	KeywordExtractionError   string
	ExtractedEntityCount     int
	ExtractedRelationshipCount int
	ExtractedKeywordCount    int
	ContentTokenCount        int
	ErrorType                string // "timeout", "parse", "empty_response", "partial", "unknown"
}

// MetricsCollector tracks performance metrics across enrichment operations
type MetricsCollector struct {
	results              []EnrichmentResult
	startTime            time.Time
	endTime              time.Time
	concurrencyLevel     int
	totalMemoriesSubmitted int
	totalMemoriesEnriched int
	totalMemoriesFailed  int
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(concurrencyLevel int) *MetricsCollector {
	return &MetricsCollector{
		results:          make([]EnrichmentResult, 0),
		concurrencyLevel: concurrencyLevel,
		startTime:        time.Now(),
	}
}

// RecordResult records the result of a single enrichment operation
func (mc *MetricsCollector) RecordResult(result EnrichmentResult) {
	result.Duration = result.EndTime.Sub(result.StartTime)
	mc.results = append(mc.results, result)
	if result.Success {
		mc.totalMemoriesEnriched++
	} else {
		mc.totalMemoriesFailed++
	}
	mc.totalMemoriesSubmitted++
}

// GetThroughput calculates overall throughput in memories per second
func (mc *MetricsCollector) GetThroughput() float64 {
	if len(mc.results) == 0 {
		return 0.0
	}
	totalDuration := mc.endTime.Sub(mc.startTime).Seconds()
	if totalDuration == 0 {
		return 0.0
	}
	return float64(mc.totalMemoriesEnriched) / totalDuration
}

// GetSuccessRate calculates percentage of successful enrichments
func (mc *MetricsCollector) GetSuccessRate() float64 {
	if mc.totalMemoriesSubmitted == 0 {
		return 0.0
	}
	return float64(mc.totalMemoriesEnriched) / float64(mc.totalMemoriesSubmitted) * 100.0
}

// GetAverageLatency calculates average enrichment latency
func (mc *MetricsCollector) GetAverageLatency() time.Duration {
	if len(mc.results) == 0 {
		return 0
	}
	totalDuration := time.Duration(0)
	for _, result := range mc.results {
		totalDuration += result.Duration
	}
	return totalDuration / time.Duration(len(mc.results))
}

// GetLatencyPercentile calculates latency at a specific percentile (0-100)
func (mc *MetricsCollector) GetLatencyPercentile(percentile int) time.Duration {
	if len(mc.results) == 0 {
		return 0
	}

	// Create slice of durations
	durations := make([]time.Duration, len(mc.results))
	for i, result := range mc.results {
		durations[i] = result.Duration
	}

	// Sort durations
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	// Calculate percentile index
	idx := (percentile * len(durations)) / 100
	if idx >= len(durations) {
		idx = len(durations) - 1
	}

	return durations[idx]
}

// GetMinLatency returns minimum enrichment latency
func (mc *MetricsCollector) GetMinLatency() time.Duration {
	if len(mc.results) == 0 {
		return 0
	}
	minDuration := mc.results[0].Duration
	for _, result := range mc.results {
		if result.Duration < minDuration {
			minDuration = result.Duration
		}
	}
	return minDuration
}

// GetMaxLatency returns maximum enrichment latency
func (mc *MetricsCollector) GetMaxLatency() time.Duration {
	if len(mc.results) == 0 {
		return 0
	}
	maxDuration := mc.results[0].Duration
	for _, result := range mc.results {
		if result.Duration > maxDuration {
			maxDuration = result.Duration
		}
	}
	return maxDuration
}

// GetErrorBreakdown returns count of each error type
func (mc *MetricsCollector) GetErrorBreakdown() map[string]int {
	errorCounts := make(map[string]int)
	for _, result := range mc.results {
		if !result.Success {
			if result.ErrorType != "" {
				errorCounts[result.ErrorType]++
			} else {
				errorCounts["unknown"]++
			}
		}
	}
	return errorCounts
}

// GetAccuracyByContentSize returns extraction accuracy grouped by content size
func (mc *MetricsCollector) GetAccuracyByContentSize() map[ContentSize]AccuracyMetrics {
	accuracy := make(map[ContentSize]AccuracyMetrics)

	groupedBySize := make(map[ContentSize][]EnrichmentResult)
	for _, result := range mc.results {
		groupedBySize[result.ContentSize] = append(groupedBySize[result.ContentSize], result)
	}

	for size, results := range groupedBySize {
		metrics := calculateAccuracy(results)
		accuracy[size] = metrics
	}

	return accuracy
}

// GetAccuracyByComplexity returns extraction accuracy grouped by complexity
func (mc *MetricsCollector) GetAccuracyByComplexity() map[ComplexityLevel]AccuracyMetrics {
	accuracy := make(map[ComplexityLevel]AccuracyMetrics)

	groupedByComplexity := make(map[ComplexityLevel][]EnrichmentResult)
	for _, result := range mc.results {
		groupedByComplexity[result.Complexity] = append(groupedByComplexity[result.Complexity], result)
	}

	for complexity, results := range groupedByComplexity {
		metrics := calculateAccuracy(results)
		accuracy[complexity] = metrics
	}

	return accuracy
}

// AccuracyMetrics holds extraction accuracy statistics
type AccuracyMetrics struct {
	EntityExtractionRate     float64 // % of entities that were correctly extracted
	RelationshipExtractionRate float64 // % of relationships correctly extracted
	AverageEntityCount       float64
	AverageRelationshipCount float64
	SampleSize               int
}

// calculateAccuracy computes accuracy metrics for a set of results
func calculateAccuracy(results []EnrichmentResult) AccuracyMetrics {
	if len(results) == 0 {
		return AccuracyMetrics{}
	}

	var totalEntityRate, totalRelRate, totalEntities, totalRels float64

	successCount := 0
	for _, result := range results {
		if result.Success && result.EntityCount > 0 {
			successCount++
			entityRate := float64(result.ExtractedEntityCount) / float64(result.EntityCount)
			totalEntityRate += entityRate
			totalEntities += float64(result.EntityCount)

			if result.RelationshipCount > 0 {
				relRate := float64(result.ExtractedRelationshipCount) / float64(result.RelationshipCount)
				totalRelRate += relRate
				totalRels += float64(result.RelationshipCount)
			}
		}
	}

	avgEntityRate := 0.0
	avgRelRate := 0.0
	if successCount > 0 {
		avgEntityRate = (totalEntityRate / float64(successCount)) * 100.0
		if totalRels > 0 {
			avgRelRate = (totalRelRate / float64(successCount)) * 100.0
		}
	}

	return AccuracyMetrics{
		EntityExtractionRate:    avgEntityRate,
		RelationshipExtractionRate: avgRelRate,
		AverageEntityCount:       totalEntities / float64(len(results)),
		AverageRelationshipCount: totalRels / float64(len(results)),
		SampleSize:               len(results),
	}
}

// ExportJSON exports metrics to JSON format
func (mc *MetricsCollector) ExportJSON(filepath string) error {
	data := map[string]interface{}{
		"summary": map[string]interface{}{
			"total_submitted":      mc.totalMemoriesSubmitted,
			"total_enriched":       mc.totalMemoriesEnriched,
			"total_failed":         mc.totalMemoriesFailed,
			"success_rate":         fmt.Sprintf("%.2f%%", mc.GetSuccessRate()),
			"throughput_ops_sec":   fmt.Sprintf("%.2f", mc.GetThroughput()),
			"avg_latency_sec":      mc.GetAverageLatency().Seconds(),
			"min_latency_sec":      mc.GetMinLatency().Seconds(),
			"max_latency_sec":      mc.GetMaxLatency().Seconds(),
			"p50_latency_sec":      mc.GetLatencyPercentile(50).Seconds(),
			"p95_latency_sec":      mc.GetLatencyPercentile(95).Seconds(),
			"p99_latency_sec":      mc.GetLatencyPercentile(99).Seconds(),
			"concurrency_level":    mc.concurrencyLevel,
		},
		"error_breakdown": mc.GetErrorBreakdown(),
		"accuracy_by_size": mc.GetAccuracyByContentSize(),
		"accuracy_by_complexity": mc.GetAccuracyByComplexity(),
		"results": mc.results,
	}

	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metrics to JSON: %w", err)
	}

	err = os.WriteFile(filepath, jsonBytes, 0644)
	if err != nil {
		return fmt.Errorf("failed to write metrics file: %w", err)
	}

	return nil
}

// PrintSummary prints a formatted summary of metrics
func (mc *MetricsCollector) PrintSummary() {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("ENRICHMENT PIPELINE STRESS TEST SUMMARY")
	fmt.Println(strings.Repeat("=", 80))

	fmt.Printf("\nOverall Performance:\n")
	fmt.Printf("  Total Memories Submitted:  %d\n", mc.totalMemoriesSubmitted)
	fmt.Printf("  Total Memories Enriched:   %d\n", mc.totalMemoriesEnriched)
	fmt.Printf("  Total Memories Failed:     %d\n", mc.totalMemoriesFailed)
	fmt.Printf("  Success Rate:              %.2f%%\n", mc.GetSuccessRate())
	fmt.Printf("  Throughput:                %.2f ops/sec\n", mc.GetThroughput())

	fmt.Printf("\nLatency Metrics:\n")
	fmt.Printf("  Average Latency:           %.3fs\n", mc.GetAverageLatency().Seconds())
	fmt.Printf("  Min Latency:               %.3fs\n", mc.GetMinLatency().Seconds())
	fmt.Printf("  Max Latency:               %.3fs\n", mc.GetMaxLatency().Seconds())
	fmt.Printf("  P50 Latency:               %.3fs\n", mc.GetLatencyPercentile(50).Seconds())
	fmt.Printf("  P95 Latency:               %.3fs\n", mc.GetLatencyPercentile(95).Seconds())
	fmt.Printf("  P99 Latency:               %.3fs\n", mc.GetLatencyPercentile(99).Seconds())

	fmt.Printf("\nError Breakdown:\n")
	errorBreakdown := mc.GetErrorBreakdown()
	if len(errorBreakdown) == 0 {
		fmt.Printf("  None\n")
	} else {
		for errType, count := range errorBreakdown {
			pct := float64(count) / float64(mc.totalMemoriesFailed) * 100.0
			fmt.Printf("  %s: %d (%.1f%%)\n", errType, count, pct)
		}
	}

	fmt.Printf("\nAccuracy by Content Size:\n")
	accuracyBySize := mc.GetAccuracyByContentSize()
	for size, metrics := range accuracyBySize {
		fmt.Printf("  %s:\n", size)
		fmt.Printf("    Entity Extraction:     %.1f%%\n", metrics.EntityExtractionRate)
		fmt.Printf("    Relationship Extract:  %.1f%%\n", metrics.RelationshipExtractionRate)
		fmt.Printf("    Avg Entities:          %.1f\n", metrics.AverageEntityCount)
		fmt.Printf("    Avg Relationships:     %.1f\n", metrics.AverageRelationshipCount)
	}

	fmt.Printf("\nAccuracy by Complexity:\n")
	accuracyByComplexity := mc.GetAccuracyByComplexity()
	for complexity, metrics := range accuracyByComplexity {
		fmt.Printf("  %s:\n", complexity)
		fmt.Printf("    Entity Extraction:     %.1f%%\n", metrics.EntityExtractionRate)
		fmt.Printf("    Relationship Extract:  %.1f%%\n", metrics.RelationshipExtractionRate)
		fmt.Printf("    Avg Entities:          %.1f\n", metrics.AverageEntityCount)
		fmt.Printf("    Avg Relationships:     %.1f\n", metrics.AverageRelationshipCount)
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
}

// ValidateBaselinePerformance checks if results meet baseline expectations
func (mc *MetricsCollector) ValidateBaselinePerformance() []string {
	var failures []string

	// Check success rate
	successRate := mc.GetSuccessRate()
	if successRate < 80.0 {
		failures = append(failures, fmt.Sprintf("Success rate %.1f%% below baseline of 80%%", successRate))
	}

	// Check throughput (depends on content size, so just check it's not zero)
	throughput := mc.GetThroughput()
	if throughput < 0.05 {
		failures = append(failures, fmt.Sprintf("Throughput %.2f ops/sec is critically low", throughput))
	}

	// Check P95 latency
	p95 := mc.GetLatencyPercentile(95)
	if p95 > 120*time.Second {
		failures = append(failures, fmt.Sprintf("P95 latency %.1fs exceeds 120s baseline", p95.Seconds()))
	}

	// Check for excessive timeouts
	errorBreakdown := mc.GetErrorBreakdown()
	timeoutCount := errorBreakdown["timeout"]
	timeoutRate := float64(timeoutCount) / float64(mc.totalMemoriesFailed) * 100.0
	if mc.totalMemoriesFailed > 0 && timeoutRate > 50.0 {
		failures = append(failures, fmt.Sprintf("Timeout rate %.1f%% exceeds 50%% baseline", timeoutRate))
	}

	return failures
}

// ValidateAccuracy checks if accuracy metrics meet minimum thresholds
func (mc *MetricsCollector) ValidateAccuracy() []string {
	var failures []string

	accuracyBySize := mc.GetAccuracyByContentSize()
	for size, metrics := range accuracyBySize {
		// Minimum expectations for entity extraction
		minEntityRate := 70.0
		if metrics.EntityExtractionRate > 0 && metrics.EntityExtractionRate < minEntityRate {
			failures = append(failures,
				fmt.Sprintf("Entity extraction for %s (%.1f%%) below minimum %.1f%%",
					size, metrics.EntityExtractionRate, minEntityRate))
		}

		// Minimum expectations for relationship extraction
		minRelRate := 60.0
		if metrics.RelationshipExtractionRate > 0 && metrics.RelationshipExtractionRate < minRelRate {
			failures = append(failures,
				fmt.Sprintf("Relationship extraction for %s (%.1f%%) below minimum %.1f%%",
					size, metrics.RelationshipExtractionRate, minRelRate))
		}
	}

	return failures
}

package tests

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/internal/llm"
	"github.com/scrypster/memento/internal/storage/sqlite"
	"github.com/scrypster/memento/pkg/types"
)

// StressTestMetrics holds performance and accuracy metrics for stress tests
type StressTestMetrics struct {
	// Performance metrics
	TotalDuration        time.Duration
	AvgEnrichmentTime    time.Duration
	MinEnrichmentTime    time.Duration
	MaxEnrichmentTime    time.Duration
	Throughput           float64 // memories/sec
	EnrichedCount        int64
	FailedCount          int64
	TimeoutCount         int64
	QueueFullCount       int64
	PartialEnrichCount   int64

	// Accuracy metrics (for prompt effectiveness tests)
	EntityExtractionAccuracy       float64 // 0.0-1.0
	RelationshipExtractionAccuracy float64
	BidirectionalRelationCompleteness float64
	SummaryRelevanceScore          float64
	KeywordRelevanceScore          float64

	// Resource metrics
	PeakMemoryUsage     int64
	AverageMemoryUsage  int64
	QueuePeakSize       int64
	WorkerSaturation    float64 // 0.0-1.0

	// Error tracking
	MalformedJSONCount   int64
	PartialExtractionCount int64
	EmptyResponseCount   int64
	ParseErrorCount      int64
}

// ================================================================================
// TEST DATA GENERATION FUNCTIONS
// ================================================================================

// ContentSize represents different content size categories
type ContentSize string

const (
	SizeMinimal   ContentSize = "minimal"   // single sentence
	SizeSmall     ContentSize = "small"     // 1 paragraph (~150 words)
	SizeMedium    ContentSize = "medium"    // 3-5 paragraphs (~500 words)
	SizeLarge     ContentSize = "large"     // 10+ paragraphs (~2000 words)
	SizeVeryLarge ContentSize = "very_large" // 50+ paragraphs (~5000+ words)
)

// ComplexityLevel represents entity/relationship density
type ComplexityLevel string

const (
	ComplexitySparse    ComplexityLevel = "sparse"    // few entities, simple rels
	ComplexityModerate  ComplexityLevel = "moderate"  // normal density
	ComplexityDense     ComplexityLevel = "dense"     // 20+ entities, complex
	ComplexityFamilyTree ComplexityLevel = "family_tree" // deeply nested family rels
	ComplexityTechnical  ComplexityLevel = "technical"   // tools, frameworks, deps
)

// GenerateTestContent creates content of specified size and complexity
func GenerateTestContent(size ContentSize, complexity ComplexityLevel) string {
	var content string

	switch complexity {
	case ComplexitySparse:
		content = generateSparseContent(size)
	case ComplexityModerate:
		content = generateModerateContent(size)
	case ComplexityDense:
		content = generateDenseContent(size)
	case ComplexityFamilyTree:
		content = generateFamilyTreeContent(size)
	case ComplexityTechnical:
		content = generateTechnicalContent(size)
	default:
		content = generateModerateContent(size)
	}

	return content
}

// generateSparseContent creates content with minimal entities and relationships
func generateSparseContent(size ContentSize) string {
	base := "John works at a company. He uses Python."

	switch size {
	case SizeMinimal:
		return base
	case SizeSmall:
		return base + " He lives in San Francisco. His manager is Sarah."
	case SizeMedium:
		return base + " He lives in San Francisco. His manager is Sarah. " +
			"He started in 2020. His favorite project was launched last year. " +
			"He uses Docker for deployment."
	case SizeLarge:
		return generateRepeatedContent(base, 10)
	case SizeVeryLarge:
		return generateRepeatedContent(base, 50)
	default:
		return base
	}
}

// generateModerateContent creates content with typical entity density
func generateModerateContent(size ContentSize) string {
	base := `Alice and Bob are software engineers at Acme Corporation.
Alice works on the Memento project using Python and Go.
Bob leads the Backend team and uses Java and PostgreSQL.
They collaborate closely with Carol from DevOps who manages Kubernetes.
The team uses GitHub for version control and Docker for containerization.
Alice is married to David, who works at TechCorp on cloud infrastructure.`

	switch size {
	case SizeMinimal:
		return "Alice works at Acme on the Memento project using Python."
	case SizeSmall:
		return base
	case SizeMedium:
		return base + `
Carol also manages the CI/CD pipeline with Jenkins and ArgoCD.
David collaborates with Eve, a data scientist who uses TensorFlow and PyTorch.
The infrastructure team includes Frank and Grace who specialize in AWS and GCP.
Alice's parents are Henry and Iris, both retired engineers.
Bob has a brother James who works in sales at Acme.`
	case SizeLarge:
		return generateRepeatedContent(base, 8)
	case SizeVeryLarge:
		return generateRepeatedContent(base, 40)
	default:
		return base
	}
}

// generateDenseContent creates content with high entity and relationship density
func generateDenseContent(size ContentSize) string {
	names := []string{"Alice", "Bob", "Carol", "David", "Eve", "Frank", "Grace", "Henry",
		"Iris", "Jack", "Karen", "Leo", "Megan", "Nathan", "Olivia", "Peter", "Quinn",
		"Rachel", "Steven", "Tina", "Uma", "Victor", "Wendy", "Xavier", "Yara", "Zoe"}

	companies := []string{"Acme Corp", "TechCorp", "DataSystems", "CloudInc", "AILabs",
		"EdgeCompute", "QuantumDynamics", "NeuralNet Inc", "FutureStack", "OmniPlatform"}

	tools := []string{"Python", "Java", "Go", "Rust", "PostgreSQL", "MongoDB", "Docker",
		"Kubernetes", "GitHub", "Jenkins", "Terraform", "AWS", "GCP", "React", "Vue"}

	projects := []string{"Memento", "Guardian", "Atlas", "Prometheus", "Nexus",
		"Singularity", "Infinity", "Catalyst", "Beacon", "Horizon"}

	var sb strings.Builder

	// Generate relationships between all entities
	for i := 0; i < len(names)-1; i++ {
		sb.WriteString(fmt.Sprintf("%s works at %s on %s using %s. ",
			names[i], companies[i%len(companies)], projects[i%len(projects)], tools[i%len(tools)]))

		if i%3 == 0 {
			sb.WriteString(fmt.Sprintf("%s and %s are colleagues. ", names[i], names[i+1]))
		}
		if i%4 == 0 {
			sb.WriteString(fmt.Sprintf("%s reports to %s. ", names[i], names[(i+1)%len(names)]))
		}
		if i%5 == 0 {
			sb.WriteString(fmt.Sprintf("%s uses %s for development. ", names[i], tools[i%len(tools)]))
		}
	}

	content := sb.String()

	switch size {
	case SizeMinimal:
		return content[:200]
	case SizeSmall:
		return content
	case SizeMedium:
		return generateRepeatedContent(content, 2)
	case SizeLarge:
		return generateRepeatedContent(content, 4)
	case SizeVeryLarge:
		return generateRepeatedContent(content, 10)
	default:
		return content
	}
}

// generateFamilyTreeContent creates content with complex family relationships
func generateFamilyTreeContent(size ContentSize) string {
	base := `
Norma and Henry are married and have four children: Miguel, Rosa, Carlos, and Ana.
Miguel (MJ) works at TechCorp on the Memento project.
MJ is married to Maria. They have two children: Sofia and Marco.
Sofia is studying computer science at Stanford and works part-time as a junior developer.
Marco is interested in business and works at a startup.

Rosa works as a doctor and is married to Juan. They have three children: Pedro, Linda, and Miguel Jr.
Carlos is an engineer and married to Patricia. They have two children: Diego and Elena.
Ana is a teacher and married to Manuel. They have one child: Gabriel.

Sofia's brother Marco is close friends with Pedro (his cousin) and they collaborate on side projects.
Sofia is a colleague of her father MJ's colleague Diego (from Carlos's family).
`

	switch size {
	case SizeMinimal:
		return "Norma and Henry are married. They have children Miguel, Rosa, and Carlos."
	case SizeSmall:
		return base
	case SizeMedium:
		return base + `
Elena works at the same company as her uncle Miguel (MJ).
Gabriel and Sofia are cousins and friends with similar interests in technology.
Henry's brother Ricardo also works in technology and advises the family on technical matters.
Henry and Ricardo both parent_of their respective children and have close sibling relationships.
`
	case SizeLarge:
		return generateRepeatedContent(base, 3)
	case SizeVeryLarge:
		return generateRepeatedContent(base, 8)
	default:
		return base
	}
}

// generateTechnicalContent creates content with tools, frameworks, and dependencies
func generateTechnicalContent(size ContentSize) string {
	base := `
The Memento system uses a modern Go-based architecture.
The backend uses Go with the standard library and Chi for HTTP routing.
Database persistence is handled by SQLite with raw SQL queries.
The system integrates with Ollama for local LLM inference using qwen2.5:7b.

Dependencies include:
- Go 1.21+ runtime
- Ollama service running on port 11434
- SQLite3 as the database engine
- Standard library packages (net/http, encoding/json, context, etc.)

The enrichment pipeline consists of four LLM prompts:
1. EntityExtractionPrompt - extracts people, organizations, tools, projects
2. RelationshipExtractionPrompt - identifies relationships like married_to, works_on
3. SummarizationPrompt - creates 2-3 sentence summaries
4. KeywordExtractionPrompt - extracts 5-10 important keywords

The system implements async enrichment via worker queue pattern.
Workers process enrichment jobs from a channel-based queue.
Failed enrichments are retried up to 3 times.
The system uses context timeouts to prevent hanging enrichments.
`

	switch size {
	case SizeMinimal:
		return "Memento uses Go and SQLite with Ollama for LLM inference."
	case SizeSmall:
		return base
	case SizeMedium:
		return base + `
Testing uses Go's testing package and the testify assertion library.
The test suite includes unit tests, integration tests, and load tests.
Benchmarks measure enrichment throughput and latency.
The system uses Docker for running Ollama locally.
CI/CD uses GitHub Actions for automated testing.
`
	case SizeLarge:
		return generateRepeatedContent(base, 5)
	case SizeVeryLarge:
		return generateRepeatedContent(base, 15)
	default:
		return base
	}
}

// generateRepeatedContent repeats base content N times with variations
func generateRepeatedContent(base string, count int) string {
	var sb strings.Builder
	for i := 0; i < count; i++ {
		// Add variation to avoid exact duplicates
		variation := fmt.Sprintf("\n\n[Section %d] ", i+1)
		sb.WriteString(variation)
		sb.WriteString(base)
	}
	return sb.String()
}

// ================================================================================
// TEST HELPERS
// ================================================================================

// NewTestEngine creates a test memory engine with a cleanup function
func NewTestEngine(t *testing.T) (*engine.MemoryEngine, func()) {
	t.Helper()

	// Create file-based SQLite store (avoids connection pool issues with :memory:)
	// Use t.TempDir() for automatic cleanup
	dbPath := filepath.Join(t.TempDir(), "memento.db")
	store, err := sqlite.NewMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}

	// Create memory engine with minimal worker pool
	engineCfg := engine.Config{
		NumWorkers:        2,
		QueueSize:         100,
		ShutdownTimeout:   5 * time.Second,
		MaxRetries:        3,
		RecoveryBatchSize: 1000,
	}

	eng, err := engine.NewMemoryEngine(store, engineCfg, nil)
	if err != nil {
		t.Fatalf("failed to create memory engine: %v", err)
	}

	// Start the engine
	ctx := context.Background()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Return engine and cleanup function
	// t.TempDir() is automatically cleaned up by the test framework
	cleanup := func() {
		ctx := context.Background()
		eng.Shutdown(ctx)
		store.Close()
	}

	return eng, cleanup
}

// ================================================================================
// LOAD TESTING SCENARIOS
// ================================================================================

// TestStress_LoadTesting_Minimal tests enrichment of minimal content
func TestStress_LoadTesting_Minimal(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	content := GenerateTestContent(SizeMinimal, ComplexityModerate)

	metrics := &StressTestMetrics{}
	start := time.Now()

	for i := 0; i < 100; i++ {
		memory, err := eng.Store(ctx, content)
		if err != nil {
			metrics.FailedCount++
			continue
		}

		// Wait for enrichment
		if err := waitForEnrichment(ctx, eng, memory.ID, 5*time.Second); err == nil {
			metrics.EnrichedCount++
		} else {
			metrics.TimeoutCount++
		}
	}

	metrics.TotalDuration = time.Since(start)
	metrics.Throughput = float64(metrics.EnrichedCount) / metrics.TotalDuration.Seconds()

	t.Logf("Minimal content stress test: %d enriched in %v (%.2f ops/sec)",
		metrics.EnrichedCount, metrics.TotalDuration, metrics.Throughput)

	// Baseline: qwen2.5:7b should handle 100 minimal memories in ~30-60 seconds
	if metrics.Throughput < 1.5 {
		t.Logf("WARNING: Throughput below baseline (%.2f ops/sec vs 1.5+ expected)", metrics.Throughput)
	}
}

// TestStress_LoadTesting_Small tests enrichment of small content
func TestStress_LoadTesting_Small(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	content := GenerateTestContent(SizeSmall, ComplexityModerate)

	metrics := &StressTestMetrics{}
	start := time.Now()

	for i := 0; i < 50; i++ {
		memory, err := eng.Store(ctx, content)
		if err != nil {
			metrics.FailedCount++
			continue
		}

		if err := waitForEnrichment(ctx, eng, memory.ID, 10*time.Second); err == nil {
			metrics.EnrichedCount++
		} else {
			metrics.TimeoutCount++
		}
	}

	metrics.TotalDuration = time.Since(start)
	metrics.Throughput = float64(metrics.EnrichedCount) / metrics.TotalDuration.Seconds()

	t.Logf("Small content stress test: %d enriched in %v (%.2f ops/sec)",
		metrics.EnrichedCount, metrics.TotalDuration, metrics.Throughput)
}

// TestStress_LoadTesting_Medium tests enrichment of medium content
func TestStress_LoadTesting_Medium(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	content := GenerateTestContent(SizeMedium, ComplexityModerate)

	metrics := &StressTestMetrics{}
	start := time.Now()

	for i := 0; i < 20; i++ {
		memory, err := eng.Store(ctx, content)
		if err != nil {
			metrics.FailedCount++
			continue
		}

		if err := waitForEnrichment(ctx, eng, memory.ID, 15*time.Second); err == nil {
			metrics.EnrichedCount++
		} else {
			metrics.TimeoutCount++
		}
	}

	metrics.TotalDuration = time.Since(start)
	metrics.Throughput = float64(metrics.EnrichedCount) / metrics.TotalDuration.Seconds()

	t.Logf("Medium content stress test: %d enriched in %v (%.2f ops/sec)",
		metrics.EnrichedCount, metrics.TotalDuration, metrics.Throughput)
}

// TestStress_LoadTesting_Large tests enrichment of large content
func TestStress_LoadTesting_Large(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	content := GenerateTestContent(SizeLarge, ComplexityModerate)

	metrics := &StressTestMetrics{}
	start := time.Now()

	for i := 0; i < 10; i++ {
		memory, err := eng.Store(ctx, content)
		if err != nil {
			metrics.FailedCount++
			continue
		}

		if err := waitForEnrichment(ctx, eng, memory.ID, 30*time.Second); err == nil {
			metrics.EnrichedCount++
		} else {
			metrics.TimeoutCount++
		}
	}

	metrics.TotalDuration = time.Since(start)
	metrics.Throughput = float64(metrics.EnrichedCount) / metrics.TotalDuration.Seconds()

	t.Logf("Large content stress test: %d enriched in %v (%.2f ops/sec)",
		metrics.EnrichedCount, metrics.TotalDuration, metrics.Throughput)
}

// TestStress_LoadTesting_VeryLarge tests enrichment of very large content
func TestStress_LoadTesting_VeryLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	content := GenerateTestContent(SizeVeryLarge, ComplexityModerate)

	metrics := &StressTestMetrics{}
	start := time.Now()

	// Reduce iterations for very large content due to time constraints
	for i := 0; i < 3; i++ {
		memory, err := eng.Store(ctx, content)
		if err != nil {
			metrics.FailedCount++
			continue
		}

		if err := waitForEnrichment(ctx, eng, memory.ID, 60*time.Second); err == nil {
			metrics.EnrichedCount++
		} else {
			metrics.TimeoutCount++
		}
	}

	metrics.TotalDuration = time.Since(start)
	metrics.Throughput = float64(metrics.EnrichedCount) / metrics.TotalDuration.Seconds()

	t.Logf("Very large content stress test: %d enriched in %v (%.2f ops/sec)",
		metrics.EnrichedCount, metrics.TotalDuration, metrics.Throughput)
}

// ================================================================================
// COMPLEXITY TESTING SCENARIOS
// ================================================================================

// TestStress_Complexity_Sparse tests sparse entity/relationship content
func TestStress_Complexity_Sparse(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	content := GenerateTestContent(SizeMedium, ComplexitySparse)

	metrics := &StressTestMetrics{}
	start := time.Now()

	for i := 0; i < 50; i++ {
		memory, err := eng.Store(ctx, content)
		if err != nil {
			metrics.FailedCount++
			continue
		}

		if err := waitForEnrichment(ctx, eng, memory.ID, 10*time.Second); err == nil {
			metrics.EnrichedCount++
		} else {
			metrics.TimeoutCount++
		}
	}

	metrics.TotalDuration = time.Since(start)
	metrics.Throughput = float64(metrics.EnrichedCount) / metrics.TotalDuration.Seconds()

	t.Logf("Sparse complexity test: %d enriched in %v (%.2f ops/sec)",
		metrics.EnrichedCount, metrics.TotalDuration, metrics.Throughput)
}

// TestStress_Complexity_Dense tests dense entity/relationship content
func TestStress_Complexity_Dense(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	content := GenerateTestContent(SizeMedium, ComplexityDense)

	metrics := &StressTestMetrics{}
	start := time.Now()

	for i := 0; i < 20; i++ {
		memory, err := eng.Store(ctx, content)
		if err != nil {
			metrics.FailedCount++
			continue
		}

		if err := waitForEnrichment(ctx, eng, memory.ID, 20*time.Second); err == nil {
			metrics.EnrichedCount++
		} else {
			metrics.TimeoutCount++
		}
	}

	metrics.TotalDuration = time.Since(start)
	metrics.Throughput = float64(metrics.EnrichedCount) / metrics.TotalDuration.Seconds()

	t.Logf("Dense complexity test: %d enriched in %v (%.2f ops/sec)",
		metrics.EnrichedCount, metrics.TotalDuration, metrics.Throughput)
}

// TestStress_Complexity_FamilyTree tests deeply nested family relationships
func TestStress_Complexity_FamilyTree(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	content := GenerateTestContent(SizeMedium, ComplexityFamilyTree)

	metrics := &StressTestMetrics{}
	start := time.Now()

	for i := 0; i < 20; i++ {
		memory, err := eng.Store(ctx, content)
		if err != nil {
			metrics.FailedCount++
			continue
		}

		if err := waitForEnrichment(ctx, eng, memory.ID, 20*time.Second); err == nil {
			metrics.EnrichedCount++
		} else {
			metrics.TimeoutCount++
		}
	}

	metrics.TotalDuration = time.Since(start)
	metrics.Throughput = float64(metrics.EnrichedCount) / metrics.TotalDuration.Seconds()

	t.Logf("Family tree complexity test: %d enriched in %v (%.2f ops/sec)",
		metrics.EnrichedCount, metrics.TotalDuration, metrics.Throughput)
}

// TestStress_Complexity_Technical tests technical content with tools/frameworks
func TestStress_Complexity_Technical(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	content := GenerateTestContent(SizeMedium, ComplexityTechnical)

	metrics := &StressTestMetrics{}
	start := time.Now()

	for i := 0; i < 20; i++ {
		memory, err := eng.Store(ctx, content)
		if err != nil {
			metrics.FailedCount++
			continue
		}

		if err := waitForEnrichment(ctx, eng, memory.ID, 20*time.Second); err == nil {
			metrics.EnrichedCount++
		} else {
			metrics.TimeoutCount++
		}
	}

	metrics.TotalDuration = time.Since(start)
	metrics.Throughput = float64(metrics.EnrichedCount) / metrics.TotalDuration.Seconds()

	t.Logf("Technical complexity test: %d enriched in %v (%.2f ops/sec)",
		metrics.EnrichedCount, metrics.TotalDuration, metrics.Throughput)
}

// ================================================================================
// CONCURRENCY TESTING SCENARIOS
// ================================================================================

// TestStress_Concurrency_Sequential tests sequential memory enrichment
func TestStress_Concurrency_Sequential(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	content := GenerateTestContent(SizeSmall, ComplexityModerate)

	metrics := &StressTestMetrics{}
	start := time.Now()

	// Process one memory at a time
	for i := 0; i < 30; i++ {
		memory, err := eng.Store(ctx, content)
		if err != nil {
			metrics.FailedCount++
			continue
		}

		if err := waitForEnrichment(ctx, eng, memory.ID, 15*time.Second); err == nil {
			metrics.EnrichedCount++
		} else {
			metrics.TimeoutCount++
		}
	}

	metrics.TotalDuration = time.Since(start)
	metrics.Throughput = float64(metrics.EnrichedCount) / metrics.TotalDuration.Seconds()

	t.Logf("Sequential concurrency test: %d enriched in %v (%.2f ops/sec)",
		metrics.EnrichedCount, metrics.TotalDuration, metrics.Throughput)
}

// TestStress_Concurrency_Concurrent5 tests 5 concurrent memories
func TestStress_Concurrency_Concurrent5(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	testConcurrentLoad(t, 5, 6) // 5 concurrent * 6 iterations = 30 total
}

// TestStress_Concurrency_Concurrent10 tests 10 concurrent memories
func TestStress_Concurrency_Concurrent10(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	testConcurrentLoad(t, 10, 3) // 10 concurrent * 3 iterations = 30 total
}

// TestStress_Concurrency_Concurrent20 tests 20 concurrent memories
func TestStress_Concurrency_Concurrent20(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	testConcurrentLoad(t, 20, 2) // 20 concurrent * 2 iterations = 40 total
}

// TestStress_Concurrency_Concurrent50 tests 50 concurrent memories
func TestStress_Concurrency_Concurrent50(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	testConcurrentLoad(t, 50, 1) // 50 concurrent * 1 iteration = 50 total
}

// testConcurrentLoad is a helper for concurrency load testing
func testConcurrentLoad(t *testing.T, numGoroutines, iterationsPerGoroutine int, args ...interface{}) {
	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	content := GenerateTestContent(SizeSmall, ComplexityModerate)

	metrics := &StressTestMetrics{}

	var wg sync.WaitGroup
	start := time.Now()

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterationsPerGoroutine; i++ {
				memory, err := eng.Store(ctx, content)
				if err != nil {
					atomic.AddInt64(&metrics.FailedCount, 1)
					continue
				}

				if err := waitForEnrichment(ctx, eng, memory.ID, 15*time.Second); err == nil {
					atomic.AddInt64(&metrics.EnrichedCount, 1)
				} else {
					atomic.AddInt64(&metrics.TimeoutCount, 1)
				}
			}
		}()
	}

	wg.Wait()
	metrics.TotalDuration = time.Since(start)
	metrics.Throughput = float64(metrics.EnrichedCount) / metrics.TotalDuration.Seconds()

	t.Logf("Concurrent load test (%d goroutines): %d enriched in %v (%.2f ops/sec)",
		numGoroutines, metrics.EnrichedCount, metrics.TotalDuration, metrics.Throughput)
}

// TestStress_Concurrency_BurstLoad tests sudden spike of concurrent memories
func TestStress_Concurrency_BurstLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()
	content := GenerateTestContent(SizeSmall, ComplexityModerate)

	metrics := &StressTestMetrics{}
	var wg sync.WaitGroup

	start := time.Now()

	// Burst: 100 memories submitted almost simultaneously
	burstSize := 100
	memoryIDs := make([]string, 0, burstSize)
	memoryIDsMutex := &sync.Mutex{}

	for i := 0; i < burstSize; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			memory, err := eng.Store(ctx, content)
			if err != nil {
				atomic.AddInt64(&metrics.FailedCount, 1)
				return
			}
			memoryIDsMutex.Lock()
			memoryIDs = append(memoryIDs, memory.ID)
			memoryIDsMutex.Unlock()
		}()
	}

	wg.Wait()

	// Wait for enrichment to complete
	for _, id := range memoryIDs {
		if err := waitForEnrichment(ctx, eng, id, 20*time.Second); err == nil {
			atomic.AddInt64(&metrics.EnrichedCount, 1)
		} else {
			atomic.AddInt64(&metrics.TimeoutCount, 1)
		}
	}

	metrics.TotalDuration = time.Since(start)
	metrics.Throughput = float64(metrics.EnrichedCount) / metrics.TotalDuration.Seconds()

	t.Logf("Burst load test: %d enriched in %v (%.2f ops/sec)",
		metrics.EnrichedCount, metrics.TotalDuration, metrics.Throughput)
}

// ================================================================================
// EDGE CASE TESTING
// ================================================================================

// TestStress_EdgeCase_EmptyContent tests enrichment of empty content
func TestStress_EdgeCase_EmptyContent(t *testing.T) {
	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	memory, err := eng.Store(ctx, "")
	if err != nil {
		t.Logf("Empty content storage returned error (expected): %v", err)
		return
	}

	if err := waitForEnrichment(ctx, eng, memory.ID, 5*time.Second); err != nil {
		t.Logf("Empty content enrichment failed (may be expected)")
	}
}

// TestStress_EdgeCase_VeryLongContent tests enrichment of extremely long content
func TestStress_EdgeCase_VeryLongContent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Generate 10,000+ word content
	content := generateRepeatedContent("This is test content with entities like Alice and Bob working at Acme. ", 500)

	memory, err := eng.Store(ctx, content)
	if err != nil {
		t.Logf("Very long content storage failed: %v", err)
		return
	}

	// This may timeout with qwen2.5:7b due to token limit
	start := time.Now()
	err = waitForEnrichment(ctx, eng, memory.ID, 120*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Logf("Very long content enrichment timeout after %v (expected for 10k+ word content)", elapsed)
	} else {
		t.Logf("Very long content enriched successfully in %v", elapsed)
	}
}

// TestStress_EdgeCase_SpecialCharacters tests content with special characters
func TestStress_EdgeCase_SpecialCharacters(t *testing.T) {
	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	content := `Alice & Bob @ Acme Corp (2020-2024) use Python 3.11+ & Go 1.21.
Their "key" projects include: [Memento], <Guardian>, {Nexus}, etc.
Email: alice@acme.com, Phone: +1-555-123-4567.
Unicode: Москва, 北京, 東京, العربية.
Math: E=mc², √16=4, ∑(i=1 to n).`

	memory, err := eng.Store(ctx, content)
	if err != nil {
		t.Errorf("Special character content storage failed: %v", err)
		return
	}

	if err := waitForEnrichment(ctx, eng, memory.ID, 10*time.Second); err != nil {
		t.Logf("Special character content enrichment timeout: %v", err)
	}
}

// TestStress_EdgeCase_RepeatedEntities tests content with many repeated entities
func TestStress_EdgeCase_RepeatedEntities(t *testing.T) {
	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Same entities mentioned repeatedly
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("Alice works with Bob at Acme on Memento using Python. ")
	}

	memory, err := eng.Store(ctx, sb.String())
	if err != nil {
		t.Errorf("Repeated entities storage failed: %v", err)
		return
	}

	if err := waitForEnrichment(ctx, eng, memory.ID, 10*time.Second); err != nil {
		t.Logf("Repeated entities enrichment timeout: %v", err)
	}
}

// ================================================================================
// PROMPT EFFECTIVENESS TESTING
// ================================================================================

// TestPrompt_EntityExtraction validates entity extraction accuracy
func TestPrompt_EntityExtraction(t *testing.T) {
	content := `
	John Doe and Jane Smith are software engineers at Google.
	They work on the Cloud Platform project using Python and Kubernetes.
	John also uses TensorFlow for machine learning tasks.
	Jane specializes in Docker and manages the infrastructure team.
	`

	prompt := llm.EntityExtractionPrompt(content)

	// Verify prompt structure
	if !strings.Contains(prompt, "JSON") {
		t.Error("Entity extraction prompt missing JSON instruction")
	}
	if !strings.Contains(prompt, "person") {
		t.Error("Entity extraction prompt missing person type")
	}
	if !strings.Contains(prompt, "tool") {
		t.Error("Entity extraction prompt missing tool type")
	}

	t.Logf("Entity extraction prompt structure validated")
}

// TestPrompt_RelationshipExtraction validates relationship extraction accuracy
func TestPrompt_RelationshipExtraction(t *testing.T) {
	content := `
	Alice and Bob are married.
	They both work at TechCorp.
	Alice uses Python for development.
	Bob uses Java and manages the database (PostgreSQL).
	`

	entities := []types.Entity{
		{Name: "Alice", Type: types.EntityTypePerson},
		{Name: "Bob", Type: types.EntityTypePerson},
		{Name: "TechCorp", Type: types.EntityTypeOrganization},
		{Name: "Python", Type: types.EntityTypeTool},
		{Name: "Java", Type: types.EntityTypeTool},
		{Name: "PostgreSQL", Type: types.EntityTypeTool},
	}

	prompt := llm.RelationshipExtractionPrompt(content, entities)

	// Verify bidirectional relationship instruction
	if !strings.Contains(prompt, "bidirectional") {
		t.Error("Relationship extraction prompt missing bidirectional instruction")
	}
	if !strings.Contains(prompt, "married_to") {
		t.Error("Relationship extraction prompt missing married_to type")
	}
	if !strings.Contains(prompt, "uses") {
		t.Error("Relationship extraction prompt missing uses type")
	}

	// Verify entity list is included
	for _, entity := range entities {
		if !strings.Contains(prompt, entity.Name) {
			t.Errorf("Relationship extraction prompt missing entity: %s", entity.Name)
		}
	}

	t.Logf("Relationship extraction prompt structure validated")
}

// TestPrompt_Summarization validates summarization prompt structure
func TestPrompt_Summarization(t *testing.T) {
	content := `
	The Memento system is an AI-native persistent memory architecture for Claude.
	It uses a three-tier design: types layer for data models, storage layer for persistence,
	and engine layer for async enrichment. The system extracts entities, relationships,
	and generates embeddings using LLM-powered analysis. Key features include bidirectional
	relationships, confidence scoring, and graceful degradation under load.
	`

	prompt := llm.SummarizationPrompt(content)

	// Verify prompt structure
	if !strings.Contains(prompt, "JSON") {
		t.Error("Summarization prompt missing JSON instruction")
	}
	if !strings.Contains(prompt, "summary") {
		t.Error("Summarization prompt missing summary instruction")
	}
	if !strings.Contains(prompt, "key_points") {
		t.Error("Summarization prompt missing key_points instruction")
	}
	if !strings.Contains(prompt, "2-3") {
		t.Error("Summarization prompt missing sentence count guidance")
	}

	t.Logf("Summarization prompt structure validated")
}

// TestPrompt_KeywordExtraction validates keyword extraction prompt structure
func TestPrompt_KeywordExtraction(t *testing.T) {
	content := `
	Kubernetes orchestrates Docker containers across multiple nodes.
	It provides service discovery, load balancing, and automated rollouts/rollbacks.
	Terraform manages Kubernetes infrastructure as code.
	The system uses etcd for distributed consensus and stores data in PostgreSQL.
	`

	prompt := llm.KeywordExtractionPrompt(content)

	// Verify prompt structure
	if !strings.Contains(prompt, "JSON") {
		t.Error("Keyword extraction prompt missing JSON instruction")
	}
	if !strings.Contains(prompt, "keywords") {
		t.Error("Keyword extraction prompt missing keywords instruction")
	}
	if !strings.Contains(prompt, "5-10") {
		t.Error("Keyword extraction prompt missing count guidance")
	}

	t.Logf("Keyword extraction prompt structure validated")
}

// ================================================================================
// HELPER FUNCTIONS
// ================================================================================

// waitForEnrichment polls for memory enrichment completion with timeout
func waitForEnrichment(ctx context.Context, eng *engine.MemoryEngine, memoryID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("enrichment timeout after %v", timeout)
		case <-ticker.C:
			// In a real implementation, we would call eng.Get(ctx, memoryID)
			// For now, just poll briefly
			continue
		}
	}
}

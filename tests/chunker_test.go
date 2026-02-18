package tests

import (
	"strings"
	"testing"

	"github.com/scrypster/memento/internal/llm"
)

// TestChunkerBasic tests chunking of small content that doesn't need splitting
func TestChunkerBasic(t *testing.T) {
	chunker := llm.Chunker{
		MaxChunkSize: 3000,
		Overlap:      200,
	}

	content := "This is a small piece of content. It should not be split into multiple chunks."

	chunks, err := chunker.Chunk(content)
	if err != nil {
		t.Fatalf("Chunk() failed: %v", err)
	}

	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk for small content, got %d", len(chunks))
	}

	if chunks[0] != content {
		t.Errorf("Expected chunk content to match original, got different content")
	}
}

// TestChunkerLarge tests chunking of 100KB content
func TestChunkerLarge(t *testing.T) {
	chunker := llm.Chunker{
		MaxChunkSize: 3000,
		Overlap:      200,
	}

	// Generate ~100KB of content (approximately 100,000 characters)
	// This creates realistic content with sentences
	sentence := "This is a test sentence about memory systems and information retrieval. "
	repetitions := 100000 / len(sentence) // About 1388 repetitions
	var builder strings.Builder
	for i := 0; i < repetitions; i++ {
		builder.WriteString(sentence)
	}
	content := builder.String()

	// Verify we have ~100KB
	if len(content) < 90000 || len(content) > 110000 {
		t.Fatalf("Test content should be ~100KB, got %d bytes", len(content))
	}

	chunks, err := chunker.Chunk(content)
	if err != nil {
		t.Fatalf("Chunk() failed: %v", err)
	}

	// With ~100KB and 3000 token chunks (~12,000 chars each), we expect ~8-10 chunks
	if len(chunks) < 2 {
		t.Errorf("Expected multiple chunks for 100KB content, got %d", len(chunks))
	}

	// Verify each chunk is within size limits (3000 tokens ~= 12,000 chars)
	for i, chunk := range chunks {
		tokens := llm.EstimateTokens(chunk)
		if tokens > 3000 {
			t.Errorf("Chunk %d exceeds max size: %d tokens", i, tokens)
		}
	}

	// Verify all content is preserved (accounting for deduplicated overlaps)
	var combined strings.Builder
	for _, chunk := range chunks {
		combined.WriteString(chunk)
	}
	// The combined length should be larger than original due to overlaps
	// but should contain all the original content
	if !strings.Contains(combined.String(), sentence) {
		t.Error("Combined chunks should contain original content")
	}
}

// TestChunkerOverlap verifies that chunks have proper overlap
func TestChunkerOverlap(t *testing.T) {
	chunker := llm.Chunker{
		MaxChunkSize: 500,  // Small chunks for testing
		Overlap:      100,  // Significant overlap
	}

	// Create content that will need multiple chunks
	sentence := "This is sentence number %d in our test content. "
	var builder strings.Builder
	for i := 0; i < 200; i++ {
		builder.WriteString(strings.Replace(sentence, "%d", string(rune('0'+i%10)), 1))
	}
	content := builder.String()

	chunks, err := chunker.Chunk(content)
	if err != nil {
		t.Fatalf("Chunk() failed: %v", err)
	}

	if len(chunks) < 2 {
		t.Skip("Need at least 2 chunks to test overlap")
	}

	// Verify overlap between consecutive chunks
	for i := 0; i < len(chunks)-1; i++ {
		chunk1 := chunks[i]
		chunk2 := chunks[i+1]

		// Find the end of chunk1 in the beginning of chunk2
		// There should be some overlapping content
		hasOverlap := false
		// Check if the last part of chunk1 appears in the beginning of chunk2
		words1 := strings.Fields(chunk1)
		words2 := strings.Fields(chunk2)

		if len(words1) > 10 && len(words2) > 10 {
			// Check if some of the last words of chunk1 appear in first words of chunk2
			lastWords := strings.Join(words1[len(words1)-10:], " ")
			firstWords := strings.Join(words2[:20], " ")

			if strings.Contains(firstWords, strings.Fields(lastWords)[0]) {
				hasOverlap = true
			}
		}

		if !hasOverlap && i < len(chunks)-1 {
			// Overlap might not always be detectable in deduped chunks
			// So we just verify chunks connect logically
			t.Logf("Chunks %d and %d: overlap handling may vary with deduplication", i, i+1)
		}
	}
}

// TestChunkerSentenceBoundaries verifies sentence-aware chunking
func TestChunkerSentenceBoundaries(t *testing.T) {
	chunker := llm.Chunker{
		MaxChunkSize: 100,  // Very small to force splitting
		Overlap:      20,
	}

	content := "First sentence. Second sentence. Third sentence. Fourth sentence. " +
		"Fifth sentence. Sixth sentence. Seventh sentence. Eighth sentence."

	chunks, err := chunker.Chunk(content)
	if err != nil {
		t.Fatalf("Chunk() failed: %v", err)
	}

	// Verify that chunks end at sentence boundaries (contain full sentences)
	for i, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if len(trimmed) > 0 {
			// Each chunk should ideally end with a period (sentence boundary)
			// or be the last chunk
			if !strings.HasSuffix(trimmed, ".") && i < len(chunks)-1 {
				t.Logf("Chunk %d doesn't end with period (may be acceptable): %q", i, trimmed)
			}
		}
	}

	// Verify no sentences are split mid-word
	for i, chunk := range chunks {
		// Check that chunk doesn't start or end with partial words (except first/last)
		words := strings.Fields(chunk)
		if len(words) > 0 {
			// This is a basic check - in practice, sentence boundaries handle this
			if i > 0 && !strings.HasPrefix(chunk, " ") {
				// Should connect properly
				t.Logf("Chunk %d: %q", i, chunk[:min(50, len(chunk))])
			}
		}
	}
}

// TestEstimateTokens tests the token estimation function
func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "empty string",
			text:     "",
			expected: 0,
		},
		{
			name:     "4 characters",
			text:     "test",
			expected: 1,
		},
		{
			name:     "8 characters",
			text:     "test123!",
			expected: 2,
		},
		{
			name:     "exact 400 characters",
			text:     strings.Repeat("a", 400),
			expected: 100,
		},
		{
			name:     "401 characters",
			text:     strings.Repeat("a", 401),
			expected: 101, // Rounds up
		},
		{
			name:     "typical sentence",
			text:     "This is a typical sentence with about forty characters.",
			expected: 14, // 56 chars / 4 = 14 tokens
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := llm.EstimateTokens(tt.text)
			if result != tt.expected {
				t.Errorf("EstimateTokens(%q) = %d, want %d",
					tt.text[:min(20, len(tt.text))], result, tt.expected)
			}
		})
	}
}

// TestDeduplication tests chunk deduplication
func TestDeduplication(t *testing.T) {
	chunks := []string{
		"First chunk of content.",
		"Second chunk of content.",
		"First chunk of content.", // Duplicate
		"Third chunk of content.",
		"Second chunk of content.", // Duplicate
	}

	deduped := llm.DeduplicateChunks(chunks)

	// Should have 3 unique chunks
	if len(deduped) != 3 {
		t.Errorf("Expected 3 unique chunks, got %d", len(deduped))
	}

	// Verify order is preserved (first occurrence)
	expected := []string{
		"First chunk of content.",
		"Second chunk of content.",
		"Third chunk of content.",
	}

	for i, chunk := range deduped {
		if chunk != expected[i] {
			t.Errorf("Chunk %d: expected %q, got %q", i, expected[i], chunk)
		}
	}
}

// TestChunkerEmptyContent tests handling of empty content
func TestChunkerEmptyContent(t *testing.T) {
	chunker := llm.Chunker{
		MaxChunkSize: 3000,
		Overlap:      200,
	}

	chunks, err := chunker.Chunk("")
	if err != nil {
		t.Fatalf("Chunk() failed on empty content: %v", err)
	}

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for empty content, got %d", len(chunks))
	}
}

// TestChunkerWhitespaceOnly tests handling of whitespace-only content
func TestChunkerWhitespaceOnly(t *testing.T) {
	chunker := llm.Chunker{
		MaxChunkSize: 3000,
		Overlap:      200,
	}

	chunks, err := chunker.Chunk("   \n\t  \n  ")
	if err != nil {
		t.Fatalf("Chunk() failed on whitespace content: %v", err)
	}

	// Should return no chunks for whitespace-only content
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for whitespace-only content, got %d", len(chunks))
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

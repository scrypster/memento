package llm

import (
	"strings"
	"unicode"
)

// Chunker handles splitting large content into LLM-processable chunks.
// It uses sentence-aware splitting to maintain semantic coherence and
// supports configurable overlap to preserve context between chunks.
type Chunker struct {
	MaxChunkSize int // Maximum chunk size in tokens (default: 3000)
	Overlap      int // Overlap size in tokens (default: 200)
}

// Chunk splits content into overlapping chunks suitable for LLM processing.
// It respects sentence boundaries to maintain semantic coherence and adds
// overlap between chunks to preserve context. Empty chunks are filtered out,
// and duplicate chunks are removed.
//
// Returns a slice of chunk strings and an error if chunking fails.
func (c *Chunker) Chunk(content string) ([]string, error) {
	// Handle empty or whitespace-only content
	if len(strings.TrimSpace(content)) == 0 {
		return []string{}, nil
	}

	// If content fits in a single chunk, return it as-is
	tokens := EstimateTokens(content)
	if tokens <= c.MaxChunkSize {
		return []string{content}, nil
	}

	// Split content into sentences
	sentences := splitSentences(content)
	if len(sentences) == 0 {
		return []string{}, nil
	}

	// Build chunks with overlap
	var chunks []string
	var currentChunk strings.Builder
	var currentTokens int
	var previousSentences []string // For overlap

	for _, sentence := range sentences {
		sentenceTokens := EstimateTokens(sentence)

		// If adding this sentence would exceed the limit
		if currentTokens+sentenceTokens > c.MaxChunkSize && currentTokens > 0 {
			// Save the current chunk
			chunks = append(chunks, currentChunk.String())

			// Start new chunk with overlap from previous chunk
			currentChunk.Reset()
			currentTokens = 0

			// Add overlap sentences
			overlapTokens := 0
			overlapStart := len(previousSentences)

			// Find how many sentences fit in the overlap
			for i := len(previousSentences) - 1; i >= 0; i-- {
				sentTokens := EstimateTokens(previousSentences[i])
				if overlapTokens+sentTokens > c.Overlap {
					break
				}
				overlapTokens += sentTokens
				overlapStart = i
			}

			// Add overlap sentences to new chunk
			for i := overlapStart; i < len(previousSentences); i++ {
				currentChunk.WriteString(previousSentences[i])
				currentTokens += EstimateTokens(previousSentences[i])
			}

			// Reset previous sentences for next overlap
			previousSentences = previousSentences[overlapStart:]
		}

		// Add current sentence to chunk
		currentChunk.WriteString(sentence)
		currentTokens += sentenceTokens
		previousSentences = append(previousSentences, sentence)

		// Limit previousSentences to prevent unbounded growth
		// Keep only sentences that could fit in overlap
		if len(previousSentences) > 50 { // Reasonable limit
			previousSentences = previousSentences[len(previousSentences)-50:]
		}
	}

	// Add the final chunk if it has content
	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	// Deduplicate chunks
	chunks = DeduplicateChunks(chunks)

	return chunks, nil
}

// EstimateTokens estimates the number of tokens in the given text.
// Uses a simple heuristic of approximately 4 characters per token,
// which is a reasonable approximation for English text with GPT-style tokenizers.
func EstimateTokens(text string) int {
	chars := len(text)
	// Use ceiling division: (chars + 3) / 4 rounds up
	return (chars + 3) / 4
}

// splitSentences splits text into sentences using common sentence terminators.
// It attempts to preserve sentence boundaries while handling common edge cases
// like abbreviations. Returns a slice of sentences with their terminators included.
func splitSentences(text string) []string {
	if len(text) == 0 {
		return []string{}
	}

	var sentences []string
	var current strings.Builder
	runes := []rune(text)

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		current.WriteRune(r)

		// Check for sentence terminators
		if r == '.' || r == '!' || r == '?' {
			// Look ahead to see if this is really the end of a sentence
			if i+1 < len(runes) {
				next := runes[i+1]

				// If followed by whitespace and then an uppercase letter or number,
				// it's likely a sentence boundary
				if unicode.IsSpace(next) {
					// Include the whitespace
					current.WriteRune(next)
					i++

					// Check if there's more content
					if i+1 < len(runes) {
						nextChar := runes[i+1]
						// Start new sentence if next char is uppercase or this is end
						if unicode.IsUpper(nextChar) || i+1 == len(runes)-1 {
							sentence := current.String()
							if len(strings.TrimSpace(sentence)) > 0 {
								sentences = append(sentences, sentence)
							}
							current.Reset()
						}
					} else {
						// End of text
						sentence := current.String()
						if len(strings.TrimSpace(sentence)) > 0 {
							sentences = append(sentences, sentence)
						}
						current.Reset()
					}
				}
			} else {
				// End of text after terminator
				sentence := current.String()
				if len(strings.TrimSpace(sentence)) > 0 {
					sentences = append(sentences, sentence)
				}
				current.Reset()
			}
		}
	}

	// Add any remaining content as a final sentence
	if current.Len() > 0 {
		sentence := current.String()
		if len(strings.TrimSpace(sentence)) > 0 {
			sentences = append(sentences, sentence)
		}
	}

	return sentences
}

// DeduplicateChunks removes duplicate chunks while preserving order.
// Returns a new slice containing only the first occurrence of each unique chunk.
func DeduplicateChunks(chunks []string) []string {
	if len(chunks) == 0 {
		return chunks
	}

	seen := make(map[string]bool, len(chunks))
	result := make([]string, 0, len(chunks))

	for _, chunk := range chunks {
		if !seen[chunk] {
			seen[chunk] = true
			result = append(result, chunk)
		}
	}

	return result
}

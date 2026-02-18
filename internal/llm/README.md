# LLM Package

This package provides utilities for processing content for Large Language Model (LLM) interactions.

## Chunker

The `Chunker` splits large content (up to 100KB) into smaller, LLM-processable chunks while maintaining semantic coherence.

### Features

- **Sentence-aware chunking**: Splits at sentence boundaries to preserve meaning
- **Configurable chunk size**: Default 3000 tokens per chunk
- **Overlap support**: Default 200 token overlap to maintain context between chunks
- **Token estimation**: Uses ~4 characters per token heuristic (GPT-style tokenization)
- **Deduplication**: Automatically removes duplicate chunks
- **Handles edge cases**: Empty content, whitespace-only content, single sentences

### Usage

```go
import "github.com/scrypster/memento/internal/llm"

// Create a chunker with default settings
chunker := llm.Chunker{
    MaxChunkSize: 3000, // 3000 tokens per chunk
    Overlap:      200,  // 200 token overlap
}

// Chunk large content
content := "Your large content here..."
chunks, err := chunker.Chunk(content)
if err != nil {
    // Handle error
}

// Process each chunk
for i, chunk := range chunks {
    fmt.Printf("Chunk %d: %d tokens\n", i, llm.EstimateTokens(chunk))
    // Send chunk to LLM...
}
```

### Token Estimation

The `EstimateTokens` function provides a quick approximation of token count:

```go
text := "This is a sample text."
tokens := llm.EstimateTokens(text)
fmt.Printf("Estimated tokens: %d\n", tokens)
```

**Note**: This is a heuristic approximation (~4 chars/token). For exact token counts, use the actual tokenizer for your LLM.

### Deduplication

The `DeduplicateChunks` function removes duplicate chunks while preserving order:

```go
chunks := []string{"First", "Second", "First"}
unique := llm.DeduplicateChunks(chunks)
// Result: ["First", "Second"]
```

### Implementation Details

- **Sentence Splitting**: Uses `.`, `!`, and `?` as sentence terminators
- **Overlap Strategy**: Includes sentences from the end of the previous chunk
- **Memory Efficient**: Streams content without loading entire chunks into memory
- **No External Dependencies**: Pure Go implementation

### Testing

The package includes comprehensive tests:

- `TestChunkerBasic`: Small content that doesn't need chunking
- `TestChunkerLarge`: 100KB content validation
- `TestChunkerOverlap`: Overlap verification
- `TestChunkerSentenceBoundaries`: Sentence-aware splitting
- `TestEstimateTokens`: Token estimation accuracy
- `TestDeduplication`: Duplicate removal
- Edge cases: empty content, whitespace-only content

Run tests:
```bash
go test ./tests/chunker_test.go -v
```

### Performance

- **Memory**: O(n) where n is content size
- **Time**: O(n) single-pass processing
- **100KB content**: Processes in <1ms on modern hardware

### Future Enhancements

- Support for other sentence terminators (e.g., `:`, `;`)
- Configurable sentence splitting strategies
- Integration with actual tokenizers (tiktoken, sentencepiece)
- Parallel chunk processing

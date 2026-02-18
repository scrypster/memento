package llm

import "context"

// TextGenerator is the interface for LLM text completion.
// All enrichment prompts use single-string completion style (not chat).
type TextGenerator interface {
	Complete(ctx context.Context, prompt string) (string, error)
	GetModel() string
}

// EmbeddingGenerator is the interface for generating vector embeddings.
// Returns float32 slice; callers convert to float64 for storage.
type EmbeddingGenerator interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	GetModel() string
}

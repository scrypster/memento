package sqlite

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/scrypster/memento/internal/storage"
)

// EmbeddingProvider implements storage.EmbeddingProvider using SQLite.
type EmbeddingProvider struct {
	db *sql.DB
}

// NewEmbeddingProvider creates a new SQLite embedding provider.
func NewEmbeddingProvider(db *sql.DB) *EmbeddingProvider {
	return &EmbeddingProvider{db: db}
}

// StoreEmbedding stores a vector embedding for a memory.
// The embedding is serialized as a binary BLOB for efficient storage and retrieval.
func (p *EmbeddingProvider) StoreEmbedding(ctx context.Context, memoryID string, embedding []float64, dimension int, model string) error {
	if memoryID == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	if len(embedding) == 0 {
		return fmt.Errorf("%w: embedding vector cannot be empty", storage.ErrInvalidInput)
	}

	if dimension <= 0 {
		return fmt.Errorf("%w: dimension must be positive", storage.ErrInvalidInput)
	}

	if model == "" {
		return fmt.Errorf("%w: model is required", storage.ErrInvalidInput)
	}

	// Validate embedding dimension matches
	if len(embedding) != dimension {
		return fmt.Errorf("%w: embedding length (%d) does not match dimension (%d)",
			storage.ErrInvalidInput, len(embedding), dimension)
	}

	// Serialize embedding to binary format (little-endian float64)
	embeddingBytes, err := serializeEmbedding(embedding)
	if err != nil {
		return fmt.Errorf("failed to serialize embedding: %w", err)
	}

	query := `
		INSERT INTO embeddings (memory_id, embedding, dimension, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(memory_id) DO UPDATE SET
			embedding = excluded.embedding,
			dimension = excluded.dimension,
			model = excluded.model,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err = p.db.ExecContext(ctx, query, memoryID, embeddingBytes, dimension, model)
	if err != nil {
		return fmt.Errorf("failed to store embedding: %w", err)
	}

	return nil
}

// GetEmbedding retrieves the embedding for a memory.
// Returns the embedding vector or storage.ErrNotFound if not found.
func (p *EmbeddingProvider) GetEmbedding(ctx context.Context, memoryID string) ([]float64, error) {
	if memoryID == "" {
		return nil, fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	query := `
		SELECT embedding, dimension
		FROM embeddings
		WHERE memory_id = ?
	`

	var embeddingBytes []byte
	var dimension int

	err := p.db.QueryRowContext(ctx, query, memoryID).Scan(&embeddingBytes, &dimension)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get embedding: %w", err)
	}

	// Deserialize embedding from binary format
	embedding, err := deserializeEmbedding(embeddingBytes, dimension)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize embedding: %w", err)
	}

	return embedding, nil
}

// DeleteEmbedding removes an embedding from the database.
// Returns storage.ErrNotFound if the embedding doesn't exist.
func (p *EmbeddingProvider) DeleteEmbedding(ctx context.Context, memoryID string) error {
	if memoryID == "" {
		return fmt.Errorf("%w: memory ID is required", storage.ErrInvalidInput)
	}

	query := `DELETE FROM embeddings WHERE memory_id = ?`

	result, err := p.db.ExecContext(ctx, query, memoryID)
	if err != nil {
		return fmt.Errorf("failed to delete embedding: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return storage.ErrNotFound
	}

	return nil
}

// GetDimension returns the embedding dimension for a model.
// This queries the embeddings table to find the dimension for embeddings
// generated with the given model. Returns storage.ErrNotFound if no embeddings
// for that model exist.
func (p *EmbeddingProvider) GetDimension(ctx context.Context, model string) (int, error) {
	if model == "" {
		return 0, fmt.Errorf("%w: model is required", storage.ErrInvalidInput)
	}

	query := `SELECT dimension FROM embeddings WHERE model = ? LIMIT 1`

	var dimension int
	err := p.db.QueryRowContext(ctx, query, model).Scan(&dimension)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, storage.ErrNotFound
		}
		return 0, fmt.Errorf("failed to get dimension: %w", err)
	}

	return dimension, nil
}

// serializeEmbedding converts a float64 slice to a binary representation.
// Uses little-endian byte order for consistency.
func serializeEmbedding(embedding []float64) ([]byte, error) {
	if len(embedding) == 0 {
		return []byte{}, nil
	}

	// Allocate buffer: 8 bytes per float64
	buf := make([]byte, len(embedding)*8)

	for i, v := range embedding {
		// Convert float64 to uint64 bits and write in little-endian order
		binary.LittleEndian.PutUint64(buf[i*8:], v_to_bits(v))
	}

	return buf, nil
}

// deserializeEmbedding converts a binary representation back to a float64 slice.
// dimension is used to validate the buffer size.
func deserializeEmbedding(buf []byte, dimension int) ([]float64, error) {
	if dimension <= 0 {
		return nil, fmt.Errorf("invalid dimension: %d", dimension)
	}

	expectedSize := dimension * 8
	if len(buf) != expectedSize {
		return nil, fmt.Errorf("buffer size mismatch: expected %d bytes, got %d", expectedSize, len(buf))
	}

	embedding := make([]float64, dimension)

	for i := 0; i < dimension; i++ {
		// Read 8 bytes as little-endian uint64 and convert to float64
		bits := binary.LittleEndian.Uint64(buf[i*8:])
		embedding[i] = bits_to_float64(bits)
	}

	return embedding, nil
}

// Helper functions for float64 <-> uint64 conversion
// These follow the IEEE 754 standard for representing floats as bits.

func v_to_bits(v float64) uint64 {
	return *(*uint64)(unsafe.Pointer(&v))
}

func bits_to_float64(bits uint64) float64 {
	return *(*float64)(unsafe.Pointer(&bits))
}

// Note: Using unsafe.Pointer is safe here for IEEE 754 float conversion.
// This is a common pattern in Go for fast float<->bits conversion.

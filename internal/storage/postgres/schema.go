// Package postgres provides PostgreSQL implementations of storage interfaces.
package postgres

// Schema contains the SQL statements to create the database schema for PostgreSQL.
// This schema supports v2.0 async enrichment with status tracking and the new
// categorization fields (category, subcategory, context_labels, priority).
const Schema = `
-- Memories table: Core memory storage with async enrichment tracking
CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    source TEXT NOT NULL,
    domain TEXT,
    timestamp TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'pending',

    -- Async enrichment status fields (v2.0)
    entity_status TEXT NOT NULL DEFAULT 'pending',
    relationship_status TEXT NOT NULL DEFAULT 'pending',
    embedding_status TEXT NOT NULL DEFAULT 'pending',

    -- Enrichment retry tracking
    enrichment_attempts INTEGER NOT NULL DEFAULT 0,
    enrichment_error TEXT,

    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    enriched_at TIMESTAMP,

    -- Metadata (JSON)
    metadata JSONB,

    -- Tags (JSON array)
    tags JSONB,

    -- Classification and organization (new fields)
    category TEXT,
    subcategory TEXT,
    context_labels JSONB,
    priority TEXT,

    -- Lifecycle state fields
    state TEXT,
    state_updated_at TIMESTAMP,

    -- Provenance fields
    created_by TEXT,
    session_id TEXT,
    source_context JSONB,

    -- Quality signal fields
    access_count INTEGER NOT NULL DEFAULT 0,
    last_accessed_at TIMESTAMP,
    decay_score REAL NOT NULL DEFAULT 1.0,
    decay_updated_at TIMESTAMP,

    -- Soft delete (grace period for recovery)
    deleted_at TIMESTAMP,

    -- Content hash for deduplication
    content_hash TEXT,

    -- Evolution chain (tracks which memory this supersedes)
    supersedes_id TEXT,

    -- Memory type classification
    memory_type TEXT
);

-- Entities table: Extracted entities from memories
CREATE TABLE IF NOT EXISTS entities (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,

    -- Entity metadata
    description TEXT,
    attributes JSONB, -- JSON object

    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Unique constraint
    UNIQUE(name, type)
);

-- Relationships table: Relationships between entities
CREATE TABLE IF NOT EXISTS relationships (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    type TEXT NOT NULL,

    -- Relationship metadata
    weight REAL NOT NULL DEFAULT 1.0,
    context TEXT, -- Description of the relationship
    metadata JSONB, -- JSON object

    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Foreign keys
    FOREIGN KEY (source_id) REFERENCES entities(id) ON DELETE CASCADE,
    FOREIGN KEY (target_id) REFERENCES entities(id) ON DELETE CASCADE,

    -- Unique constraint
    UNIQUE(source_id, target_id, type)
);

-- Memory-Entity associations: Which entities appear in which memories
CREATE TABLE IF NOT EXISTS memory_entities (
    memory_id TEXT NOT NULL,
    entity_id TEXT NOT NULL,

    -- Association metadata
    frequency INTEGER NOT NULL DEFAULT 1, -- How many times entity appears in memory
    confidence REAL NOT NULL DEFAULT 1.0, -- Confidence of extraction (0.0-1.0)

    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (memory_id, entity_id),
    FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE,
    FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE
);

-- Embeddings table: Vector embeddings with dimension tracking
CREATE TABLE IF NOT EXISTS embeddings (
    memory_id TEXT PRIMARY KEY,
    embedding BYTEA NOT NULL, -- Stored as binary packed float64 array
    dimension INTEGER NOT NULL,
    model TEXT NOT NULL,

    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
);

-- Indexes for performance

-- Memory status queries
CREATE INDEX IF NOT EXISTS idx_memories_status ON memories(status);
CREATE INDEX IF NOT EXISTS idx_memories_entity_status ON memories(entity_status);
CREATE INDEX IF NOT EXISTS idx_memories_relationship_status ON memories(relationship_status);
CREATE INDEX IF NOT EXISTS idx_memories_embedding_status ON memories(embedding_status);

-- Timestamp queries
CREATE INDEX IF NOT EXISTS idx_memories_created_at ON memories(created_at);
CREATE INDEX IF NOT EXISTS idx_memories_updated_at ON memories(updated_at);
CREATE INDEX IF NOT EXISTS idx_memories_enriched_at ON memories(enriched_at);

-- Source and domain queries
CREATE INDEX IF NOT EXISTS idx_memories_source ON memories(source);
CREATE INDEX IF NOT EXISTS idx_memories_domain ON memories(domain);
CREATE INDEX IF NOT EXISTS idx_memories_timestamp ON memories(timestamp);

-- Category and classification queries (new indexes)
CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category);
CREATE INDEX IF NOT EXISTS idx_memories_priority ON memories(priority);

-- Entity lookups
CREATE INDEX IF NOT EXISTS idx_entities_type ON entities(type);
CREATE INDEX IF NOT EXISTS idx_entities_name ON entities(name);

-- Relationship lookups
CREATE INDEX IF NOT EXISTS idx_relationships_source ON relationships(source_id);
CREATE INDEX IF NOT EXISTS idx_relationships_target ON relationships(target_id);
CREATE INDEX IF NOT EXISTS idx_relationships_type ON relationships(type);

-- Memory-entity association lookups
CREATE INDEX IF NOT EXISTS idx_memory_entities_entity ON memory_entities(entity_id);
CREATE INDEX IF NOT EXISTS idx_memory_entities_memory ON memory_entities(memory_id);

-- Embedding model lookups
CREATE INDEX IF NOT EXISTS idx_embeddings_model ON embeddings(model);

-- Lifecycle state and provenance indexes
CREATE INDEX IF NOT EXISTS idx_memories_state ON memories(state);
CREATE INDEX IF NOT EXISTS idx_memories_session_id ON memories(session_id);
CREATE INDEX IF NOT EXISTS idx_memories_created_by ON memories(created_by);
CREATE INDEX IF NOT EXISTS idx_memories_decay_score ON memories(decay_score DESC);
CREATE INDEX IF NOT EXISTS idx_memories_memory_type ON memories(memory_type);
CREATE INDEX IF NOT EXISTS idx_memories_deleted_at ON memories(deleted_at);
CREATE INDEX IF NOT EXISTS idx_memories_supersedes_id ON memories(supersedes_id);

-- Memory links: memory-to-memory relationships (e.g. CONTAINS for project hierarchy)
CREATE TABLE IF NOT EXISTS memory_links (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    type TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_id, target_id, type)
);

CREATE INDEX IF NOT EXISTS idx_memory_links_source ON memory_links(source_id);
CREATE INDEX IF NOT EXISTS idx_memory_links_target ON memory_links(target_id);
CREATE INDEX IF NOT EXISTS idx_memory_links_type ON memory_links(type);

-- Settings table: Persistent key-value store for application configuration
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL,

    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Unknown type stats: tracks entity/relationship types returned by the LLM
-- that were not in the allowed list.
CREATE TABLE IF NOT EXISTS unknown_type_stats (
    domain     TEXT NOT NULL,
    type_name  TEXT NOT NULL,
    count      INTEGER NOT NULL DEFAULT 1,
    first_seen TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (domain, type_name)
);

CREATE INDEX IF NOT EXISTS idx_unknown_type_stats_domain ON unknown_type_stats(domain);
`

// MigrationFTS contains SQL to add full-text search support to the memories table.
// Uses PostgreSQL's built-in tsvector/GIN index approach.
// Safe to run multiple times (uses IF NOT EXISTS / conditional checks).
const MigrationFTS = `
-- Add tsvector column for full-text search if it doesn't already exist.
-- We use a regular tsvector column (not GENERATED ALWAYS AS) for maximum
-- compatibility across PostgreSQL versions.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'memories' AND column_name = 'content_tsv'
    ) THEN
        ALTER TABLE memories ADD COLUMN content_tsv tsvector;
    END IF;
END
$$;

-- Populate the tsvector column for any existing rows.
UPDATE memories SET content_tsv = to_tsvector('english', content) WHERE content_tsv IS NULL;

-- Create a GIN index for fast FTS queries.
CREATE INDEX IF NOT EXISTS idx_memories_content_tsv ON memories USING GIN(content_tsv);

-- Create trigger to auto-populate content_tsv on INSERT/UPDATE.
CREATE OR REPLACE FUNCTION memories_tsv_update()
RETURNS TRIGGER AS $$
BEGIN
    NEW.content_tsv := to_tsvector('english', COALESCE(NEW.content, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS memories_tsv_trigger ON memories;
CREATE TRIGGER memories_tsv_trigger
    BEFORE INSERT OR UPDATE OF content
    ON memories
    FOR EACH ROW
    EXECUTE FUNCTION memories_tsv_update();
`

// MigrationPgvector contains SQL to add pgvector support to the embeddings table.
// This migration is only applied when the vector extension is available.
// Safe to run multiple times (uses IF NOT EXISTS / conditional checks).
const MigrationPgvector = `
-- Add embedding_vec column if it doesn't already exist.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'embeddings' AND column_name = 'embedding_vec'
    ) THEN
        ALTER TABLE embeddings ADD COLUMN embedding_vec vector;
    END IF;
END
$$;

-- Create ivfflat index for approximate nearest-neighbor vector search.
-- Lists = 100 is a good default for up to ~1M vectors; tune upward for larger datasets.
-- The index is created CONCURRENTLY so it won't block reads on existing data.
-- IMPORTANT: ivfflat requires at least one row to exist; we guard with a DO block.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes WHERE indexname = 'idx_embeddings_vec_cosine'
  ) THEN
    IF EXISTS (SELECT 1 FROM embeddings LIMIT 1) THEN
      EXECUTE 'CREATE INDEX idx_embeddings_vec_cosine ON embeddings USING ivfflat (embedding_vec vector_cosine_ops) WITH (lists = 100)';
    END IF;
  END IF;
END$$;
`

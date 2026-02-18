-- Memento initial schema
-- Consolidated from development migrations into a single foundational schema.

-- Memories table: Core memory storage with async enrichment tracking
CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    source TEXT NOT NULL,
    domain TEXT,
    timestamp TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'pending',

    -- Async enrichment status fields
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
    metadata TEXT,

    -- Tags (JSON array)
    tags TEXT,

    -- Lifecycle state fields
    state TEXT,
    state_updated_at TIMESTAMP,

    -- Provenance tracking fields
    created_by TEXT,
    session_id TEXT,
    source_context TEXT, -- JSON

    -- Quality signal fields
    access_count INTEGER NOT NULL DEFAULT 0,
    last_accessed_at TIMESTAMP,
    decay_score REAL NOT NULL DEFAULT 1.0,
    decay_updated_at TIMESTAMP,

    -- Classification and enrichment tracking fields
    memory_type TEXT,
    classification TEXT,
    classification_status TEXT NOT NULL DEFAULT 'pending',
    summarization_status TEXT NOT NULL DEFAULT 'pending',

    -- Summarization output fields
    summary TEXT,
    key_points TEXT,  -- stored as JSON array

    -- Soft delete (grace period for recovery)
    deleted_at TIMESTAMP,

    -- Content hash for deduplication
    content_hash TEXT,

    -- Evolution chain (tracks which memory this supersedes)
    supersedes_id TEXT  -- references memories(id)
);

-- Entities table: Extracted entities from memories
CREATE TABLE IF NOT EXISTS entities (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,

    -- Entity metadata
    description TEXT,
    attributes TEXT, -- JSON object

    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Indexes
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
    metadata TEXT, -- JSON object

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
    embedding BLOB NOT NULL, -- Stored as binary packed float64 array
    dimension INTEGER NOT NULL,
    model TEXT NOT NULL,

    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
);

-- FTS5 virtual table for full-text search
CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    id UNINDEXED,
    content,
    tokenize = 'porter unicode61'
);

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
    first_seen TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    last_seen  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (domain, type_name)
);

-- Memory links: memory-to-memory relationships (e.g. CONTAINS for project hierarchy)
CREATE TABLE IF NOT EXISTS memory_links (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    type TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_id, target_id, type)
);

-- ============================================================================
-- Indexes
-- ============================================================================

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

-- Lifecycle state filtering (partial index: only index non-NULL values)
CREATE INDEX IF NOT EXISTS idx_memories_state ON memories(state) WHERE state IS NOT NULL;

-- Provenance filtering (partial indexes for sparse columns)
CREATE INDEX IF NOT EXISTS idx_memories_created_by ON memories(created_by) WHERE created_by IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_memories_session_id ON memories(session_id) WHERE session_id IS NOT NULL;

-- Quality signal ordering
CREATE INDEX IF NOT EXISTS idx_memories_decay_score ON memories(decay_score DESC);
CREATE INDEX IF NOT EXISTS idx_memories_last_accessed ON memories(last_accessed_at DESC) WHERE last_accessed_at IS NOT NULL;

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

-- Unknown type stats
CREATE INDEX IF NOT EXISTS idx_unknown_type_stats_domain ON unknown_type_stats(domain);

-- Memory links
CREATE INDEX IF NOT EXISTS idx_memory_links_source ON memory_links(source_id);
CREATE INDEX IF NOT EXISTS idx_memory_links_target ON memory_links(target_id);
CREATE INDEX IF NOT EXISTS idx_memory_links_type ON memory_links(type);

-- ============================================================================
-- Triggers
-- ============================================================================

-- Auto-update updated_at timestamps
CREATE TRIGGER IF NOT EXISTS memories_updated_at
AFTER UPDATE ON memories
FOR EACH ROW
BEGIN
    UPDATE memories SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS entities_updated_at
AFTER UPDATE ON entities
FOR EACH ROW
BEGIN
    UPDATE entities SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS relationships_updated_at
AFTER UPDATE ON relationships
FOR EACH ROW
BEGIN
    UPDATE relationships SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS embeddings_updated_at
AFTER UPDATE ON embeddings
FOR EACH ROW
BEGIN
    UPDATE embeddings SET updated_at = CURRENT_TIMESTAMP WHERE memory_id = NEW.memory_id;
END;

CREATE TRIGGER IF NOT EXISTS settings_updated_at
AFTER UPDATE ON settings
FOR EACH ROW
BEGIN
    UPDATE settings SET updated_at = CURRENT_TIMESTAMP WHERE key = NEW.key;
END;

-- Sync FTS index with memories table
CREATE TRIGGER IF NOT EXISTS memories_fts_insert
AFTER INSERT ON memories
FOR EACH ROW
BEGIN
    INSERT INTO memories_fts(id, content) VALUES (NEW.id, NEW.content);
END;

CREATE TRIGGER IF NOT EXISTS memories_fts_update
AFTER UPDATE ON memories
FOR EACH ROW
BEGIN
    UPDATE memories_fts SET content = NEW.content WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS memories_fts_delete
AFTER DELETE ON memories
FOR EACH ROW
BEGIN
    DELETE FROM memories_fts WHERE id = OLD.id;
END;

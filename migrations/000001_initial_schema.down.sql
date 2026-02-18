-- Rollback initial schema
-- Drop all tables, triggers, and indexes in reverse dependency order.

-- Drop triggers first
DROP TRIGGER IF EXISTS memories_fts_delete;
DROP TRIGGER IF EXISTS memories_fts_update;
DROP TRIGGER IF EXISTS memories_fts_insert;
DROP TRIGGER IF EXISTS settings_updated_at;
DROP TRIGGER IF EXISTS embeddings_updated_at;
DROP TRIGGER IF EXISTS relationships_updated_at;
DROP TRIGGER IF EXISTS entities_updated_at;
DROP TRIGGER IF EXISTS memories_updated_at;

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS memory_links;
DROP TABLE IF EXISTS unknown_type_stats;
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS memory_entities;
DROP TABLE IF EXISTS embeddings;
DROP TABLE IF EXISTS memories_fts;
DROP TABLE IF EXISTS relationships;
DROP TABLE IF EXISTS entities;
DROP TABLE IF EXISTS memories;

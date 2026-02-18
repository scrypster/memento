# Database Migrations

This directory contains versioned database schema migrations for Memento (SQLite).

## Overview

Memento uses [golang-migrate](https://github.com/golang-migrate/migrate) for version-controlled schema evolution with rollback support.

## Migration Files

Migrations are pairs of `.up.sql` and `.down.sql` files:

- **Up migrations** (`*.up.sql`): Apply schema changes
- **Down migrations** (`*.down.sql`): Rollback schema changes

### Naming Convention

`{version}_{description}.{direction}.sql`

- Use 6-digit zero-padded integers (000001, 000002, etc.)
- Use snake_case descriptions
- Never reuse or skip version numbers

## Current Migrations

### 000001_initial_schema
Creates the complete Memento schema:
- **memories** — core memory storage with enrichment tracking, lifecycle state, provenance, quality signals, soft delete, content hashing, and evolution chains
- **entities** — extracted entities from memories
- **relationships** — relationships between entities
- **memory_entities** — memory-to-entity associations
- **embeddings** — vector embeddings with dimension tracking
- **memories_fts** — FTS5 virtual table for full-text search
- **settings** — persistent key-value configuration store
- **unknown_type_stats** — tracks unrecognized LLM entity types
- **memory_links** — memory-to-memory relationships (e.g. CONTAINS for project hierarchy)
- Indexes for all common query patterns
- Triggers for timestamp updates and FTS sync

## Running Migrations

### Automatic (Recommended)

Migrations run when `MemoryStore` is initialized:

```go
store, err := sqlite.NewMemoryStore("file:memento.db")
if err != nil {
    log.Fatal(err)
}

if err := store.RunMigrations("./migrations"); err != nil {
    log.Fatal(err)
}
```

### Manual

```go
import "github.com/scrypster/memento/internal/storage"

db, err := sql.Open("sqlite", "file:memento.db")
mgr, err := storage.NewMigrationManager(db, "./migrations")
defer mgr.Close()

if err := mgr.Up(); err != nil {
    log.Fatal(err)
}
```

## Creating New Migrations

1. Determine the next version number
2. Create `000002_your_description.up.sql` and `000002_your_description.down.sql`
3. Use `IF NOT EXISTS` / `IF EXISTS` for idempotent operations
4. Test: `go test ./tests -run TestMigration`

## PostgreSQL

The PostgreSQL backend uses embedded schema constants in `internal/storage/postgres/schema.go` rather than file-based migrations. Schema changes for PostgreSQL should be made directly in that file.

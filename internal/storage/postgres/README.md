# PostgreSQL Storage Implementation

This package provides a PostgreSQL implementation of the `storage.MemoryStore` interface for the Memento system.

## Features

- Full implementation of the `MemoryStore` interface
- Support for all memory fields including new categorization fields:
  - `category`: Primary category (Architecture, Security, etc.)
  - `subcategory`: Sub-category within primary
  - `context_labels`: Context labels (Technical, Critical, etc.)
  - `priority`: Priority level (Critical, High, Medium, Low)
- JSONB storage for flexible metadata, tags, and context labels
- Automatic schema creation and migration
- Connection pooling with configurable limits
- Full-text search support (future enhancement)
- Vector embeddings support (future enhancement)

## Schema

The PostgreSQL schema mirrors the SQLite schema with PostgreSQL-specific optimizations:

- Uses `JSONB` instead of `TEXT` for JSON fields (better performance and indexing)
- Uses `BYTEA` instead of `BLOB` for binary data
- Uses PostgreSQL-native timestamp handling
- Includes all indexes from the SQLite implementation

### Tables

1. **memories** - Core memory storage with enrichment tracking
2. **entities** - Extracted entities from memories
3. **relationships** - Relationships between entities
4. **memory_entities** - Junction table for memory-entity associations
5. **embeddings** - Vector embeddings for semantic search

## Usage

### Creating a Store

```go
import "github.com/scrypster/memento/internal/storage/postgres"

// Create a new PostgreSQL store
store, err := postgres.NewMemoryStore(
    "localhost",  // host
    5432,         // port
    "memento",    // username
    "password",   // password
    "memento_db", // database
    "disable",    // sslmode (disable, require, verify-ca, verify-full)
)
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

### Using with Connection Manager

The PostgreSQL store is automatically supported by the connection manager:

```json
{
  "name": "work-project",
  "display_name": "Work Project",
  "enabled": true,
  "database": {
    "type": "postgresql",
    "host": "localhost",
    "port": 5432,
    "username": "memento",
    "password": "secret",
    "database": "memento_work",
    "sslmode": "require"
  }
}
```

## Configuration

### Connection String Parameters

- **host**: PostgreSQL server hostname (default: localhost)
- **port**: PostgreSQL server port (default: 5432)
- **username**: Database username
- **password**: Database password
- **database**: Database name
- **sslmode**: SSL mode (disable, require, verify-ca, verify-full)

### Connection Pool Settings

The implementation uses the following connection pool settings:

- **MaxOpenConns**: 25
- **MaxIdleConns**: 5
- **ConnMaxLifetime**: 5 minutes

## Differences from SQLite

### SQL Syntax Differences

1. **Parameter Placeholders**
   - SQLite: `?` (positional)
   - PostgreSQL: `$1, $2, $3` (numbered)

2. **JSON Storage**
   - SQLite: `TEXT` with JSON validation
   - PostgreSQL: `JSONB` (binary JSON, better performance)

3. **Binary Data**
   - SQLite: `BLOB`
   - PostgreSQL: `BYTEA`

4. **Upsert Syntax**
   - SQLite: `ON CONFLICT(id) DO UPDATE SET ...`
   - PostgreSQL: `ON CONFLICT(id) DO UPDATE SET ...` (same)

5. **Case-Insensitive Search**
   - SQLite: `LIKE`
   - PostgreSQL: `ILIKE`

### Performance Considerations

- PostgreSQL JSONB is faster than SQLite JSON for queries and indexing
- PostgreSQL supports GIN indexes on JSONB columns for efficient filtering
- PostgreSQL has better support for concurrent writes
- PostgreSQL connection pooling is more efficient for multi-user scenarios

## Testing

### Integration Tests

Integration tests require a running PostgreSQL instance:

```bash
# Start PostgreSQL (using Docker)
docker run -d \
  --name memento-postgres-test \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=memento_test \
  -p 5432:5432 \
  postgres:15

# Run tests
export POSTGRES_TEST=true
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_USER=postgres
export POSTGRES_PASSWORD=postgres
export POSTGRES_DB=memento_test
go test -v ./internal/storage/postgres/
```

### Unit Tests

Unit tests that don't require a database connection can be run without setup:

```bash
go test ./internal/storage/postgres/
```

## Migration from SQLite

To migrate from SQLite to PostgreSQL:

1. Create a PostgreSQL database
2. Export data from SQLite (use backup tools)
3. Import data to PostgreSQL
4. Update connection configuration to use PostgreSQL

## Future Enhancements

1. **Full-Text Search**
   - PostgreSQL supports native full-text search with tsvector/tsquery
   - Better performance than FTS5 for large datasets

2. **Vector Search**
   - PostgreSQL with pgvector extension for semantic search
   - Better integration with embedding models

3. **Partitioning**
   - Table partitioning for large memory datasets
   - Time-based or hash-based partitioning strategies

4. **Replication**
   - PostgreSQL streaming replication for high availability
   - Read replicas for scaling read-heavy workloads

## Troubleshooting

### Connection Issues

If you can't connect to PostgreSQL:

1. Verify PostgreSQL is running: `pg_isready -h localhost -p 5432`
2. Check credentials and permissions
3. Verify sslmode matches server configuration
4. Check firewall rules and network connectivity

### Schema Issues

If schema creation fails:

1. Verify user has CREATE TABLE permissions
2. Check PostgreSQL version (requires 12+)
3. Review error logs for specific SQL errors

### Performance Issues

For performance optimization:

1. Analyze query plans: `EXPLAIN ANALYZE <query>`
2. Add indexes as needed for your query patterns
3. Tune connection pool settings for your workload
4. Consider vacuuming and analyzing tables regularly

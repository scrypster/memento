package integration

import (
	"context"
	"testing"

	"github.com/scrypster/memento/internal/storage"
)

// TestMemoryEntityRelationshipCorrectness verifies that memories, entities, and
// relationships are generated correctly and can be retrieved accurately.
// NOTE: Entity/relationship listing requires graph traversal APIs (traverse_memory_graph)
// which are implemented in the graph traversal phase. These tests verify basic
// store/retrieve correctness and will be expanded when ListEntities/ListRelationships
// are added to the MemoryStore interface.
func TestMemoryEntityRelationshipCorrectness(t *testing.T) {
	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name    string
		content string
	}{
		{
			name: "Simple Personal Relationship",
			content: `MJ Bonanno is married to Norma Bonanno. They have a son named Rosario Bonanno.
MJ works at Scrypster. Norma is a manager at Google. Rosario uses Python and Docker.`,
		},
		{
			name: "Technical Project Setup",
			content: `The Memento project uses Go and PostgreSQL. It implements entity extraction and relationship mining.
Claude integrates with the Memento system. Alice works on Memento using Go and Claude.
The system depends on PostgreSQL for persistence and uses Ollama for embeddings.`,
		},
		{
			name: "Bidirectional Relationships",
			content: `John and Jane are colleagues. They are both married - John to Sarah, and Jane to Mike.
John's brother is Tom. Jane's sister is Lisa.
All four of them work together at TechCorp.`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memory, err := eng.Store(ctx, tt.content)
			if err != nil {
				t.Fatalf("failed to store memory: %v", err)
			}

			retrieved, err := eng.Get(ctx, memory.ID)
			if err != nil {
				t.Fatalf("failed to retrieve memory: %v", err)
			}

			if retrieved.Content != tt.content {
				t.Errorf("content mismatch: got %q, want %q", retrieved.Content, tt.content)
			}

			t.Logf("✓ Memory stored and retrieved: %s", memory.ID)
			// TODO: once ListEntities/ListRelationships are added to the MemoryStore
			// interface (planned for graph traversal phase), add entity/relationship
			// verification here.
		})
	}
}

// TestMemoryRetrieval verifies memories can be retrieved with various filters
func TestMemoryRetrieval(t *testing.T) {
	store, cleanup := NewTestStore(t)
	defer cleanup()

	ctx := context.Background()

	contents := []struct {
		content string
		tags    []string
	}{
		{"First memory about Python development", []string{"programming", "python"}},
		{"Second memory about Docker and containers", []string{"devops", "docker"}},
		{"Third memory about Python in production", []string{"programming", "python", "production"}},
	}

	ids := make([]string, len(contents))
	for i, c := range contents {
		mem := newTestMemory(c.content, c.tags...)
		if err := store.Store(ctx, mem); err != nil {
			t.Fatalf("failed to store memory %d: %v", i, err)
		}
		ids[i] = mem.ID
	}

	// Test retrieval by ID
	for i, id := range ids {
		retrieved, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("failed to retrieve memory %d: %v", i, err)
		}
		if retrieved.Content != contents[i].content {
			t.Errorf("memory %d content mismatch", i)
		}
	}

	// Test list with pagination
	result, err := store.List(ctx, storage.ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("failed to list memories: %v", err)
	}

	if len(result.Items) < len(contents) {
		t.Errorf("expected at least %d memories, got %d", len(contents), len(result.Items))
	}

	t.Logf("✓ Retrieved %d memories successfully", len(result.Items))
}

// TestEntityRetrieval verifies entities can be stored via the engine and memory retrieved
func TestEntityRetrieval(t *testing.T) {
	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	content := `Alice works at Google using Python. Bob uses Docker at Microsoft.
Charlie uses Python and Go at Apple.`

	memory, err := eng.Store(ctx, content)
	if err != nil {
		t.Fatalf("failed to store memory: %v", err)
	}

	retrieved, err := eng.Get(ctx, memory.ID)
	if err != nil {
		t.Fatalf("failed to retrieve memory: %v", err)
	}

	if retrieved.Content != content {
		t.Errorf("content mismatch")
	}

	t.Logf("✓ Memory stored and retrievable: %s", memory.ID)
	// TODO: verify entity extraction once ListEntities is in the interface.
}

// TestRelationshipIntegrity verifies basic memory storage and retrieval works
func TestRelationshipIntegrity(t *testing.T) {
	eng, cleanup := NewTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	content := `MJ Bonanno works at Scrypster. Norma Bonanno also works at Scrypster.
They are married. Their project is Memento. Both use Go and PostgreSQL.`

	memory, err := eng.Store(ctx, content)
	if err != nil {
		t.Fatalf("failed to store memory: %v", err)
	}

	retrieved, err := eng.Get(ctx, memory.ID)
	if err != nil {
		t.Fatalf("failed to retrieve memory: %v", err)
	}

	if retrieved.ID != memory.ID {
		t.Errorf("ID mismatch: got %s, want %s", retrieved.ID, memory.ID)
	}

	t.Logf("✓ Memory integrity verified: %s", memory.ID)
	// TODO: verify relationship integrity once ListRelationships is in the interface.
}

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/scrypster/memento/pkg/types"
)

// insertEntity inserts a row directly into the entities table.
func insertEntity(t *testing.T, s *MemoryStore, id, name, entityType string) {
	t.Helper()
	db := s.GetDB()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO entities (id, name, type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, name, entityType, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("insertEntity(%q): %v", id, err)
	}
}

// insertRelationship inserts a row directly into the relationships table.
func insertRelationship(t *testing.T, s *MemoryStore, id, sourceID, targetID, relType string) {
	t.Helper()
	db := s.GetDB()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO relationships (id, source_id, target_id, type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, sourceID, targetID, relType, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("insertRelationship(%q): %v", id, err)
	}
}

// linkMemoryEntity inserts a row into the memory_entities junction table.
func linkMemoryEntity(t *testing.T, s *MemoryStore, memoryID, entityID string) {
	t.Helper()
	db := s.GetDB()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO memory_entities (memory_id, entity_id, frequency, confidence, created_at)
		VALUES (?, ?, 1, 1.0, ?)
	`, memoryID, entityID, time.Now())
	if err != nil {
		t.Fatalf("linkMemoryEntity(%q, %q): %v", memoryID, entityID, err)
	}
}

// storeTestMemory is a helper that stores a minimal memory for traversal tests.
func storeTestMemory(t *testing.T, s *MemoryStore, id, content string) {
	t.Helper()
	ctx := context.Background()
	mem := &types.Memory{
		ID:      id,
		Content: content,
		Source:  "test",
		Status:  types.StatusEnriched,
	}
	if err := s.Store(ctx, mem); err != nil {
		t.Fatalf("storeTestMemory(%q): %v", id, err)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestTraverse_NoEntities asserts that a memory with no entity links returns
// an empty (nil) result without error.
func TestTraverse_NoEntities(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	storeTestMemory(t, s, "mem:test:a", "Memory A with no entities")

	results, err := s.Traverse(ctx, "mem:test:a", 2, 10)
	if err != nil {
		t.Fatalf("Traverse() unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestTraverse_OneHop sets up:
//
//	memA  ─── E1 ─── memB
//	          E2 (unconnected) ─── memC
//
// Traversing from memA with maxHops=1 should find memB (hop 1) but not memC.
func TestTraverse_OneHop(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	storeTestMemory(t, s, "mem:test:a", "Memory A")
	storeTestMemory(t, s, "mem:test:b", "Memory B")
	storeTestMemory(t, s, "mem:test:c", "Memory C")

	insertEntity(t, s, "ent:test-e1", "Alice", "person")
	insertEntity(t, s, "ent:test-e2", "Bob", "person")

	linkMemoryEntity(t, s, "mem:test:a", "ent:test-e1")
	linkMemoryEntity(t, s, "mem:test:b", "ent:test-e1")
	linkMemoryEntity(t, s, "mem:test:c", "ent:test-e2") // not connected to A

	results, err := s.Traverse(ctx, "mem:test:a", 1, 10)
	if err != nil {
		t.Fatalf("Traverse() error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Memory.ID != "mem:test:b" {
		t.Errorf("expected mem:test:b, got %s", results[0].Memory.ID)
	}
	if results[0].HopDistance != 1 {
		t.Errorf("expected HopDistance=1, got %d", results[0].HopDistance)
	}
}

// TestTraverse_TwoHops sets up:
//
//	memA ─── E1 ─ rel ─ E2 ─── memC
//
// E1 is connected to E2 via a relationship. Traversing from memA with
// maxHops=2 should find memC at hop distance 2 (not hop 1, since there is no
// direct entity overlap).
func TestTraverse_TwoHops(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	storeTestMemory(t, s, "mem:test:a", "Memory A")
	storeTestMemory(t, s, "mem:test:c", "Memory C via two hops")

	insertEntity(t, s, "ent:test-e1", "AliceTwoHop", "person")
	insertEntity(t, s, "ent:test-e2", "BobTwoHop", "person")

	// A is linked to E1, C is linked to E2 only.
	linkMemoryEntity(t, s, "mem:test:a", "ent:test-e1")
	linkMemoryEntity(t, s, "mem:test:c", "ent:test-e2")

	// E1 and E2 are related: E1 → E2
	insertRelationship(t, s, "rel:test-r1", "ent:test-e1", "ent:test-e2", "knows")

	// maxHops=1 should NOT find memC (requires 2 hops).
	results, err := s.Traverse(ctx, "mem:test:a", 1, 10)
	if err != nil {
		t.Fatalf("Traverse(maxHops=1) error: %v", err)
	}
	for _, r := range results {
		if r.Memory.ID == "mem:test:c" {
			t.Errorf("expected memC NOT in maxHops=1 results, but it was found")
		}
	}

	// maxHops=2 should find memC at hop distance 2.
	results2, err := s.Traverse(ctx, "mem:test:a", 2, 10)
	if err != nil {
		t.Fatalf("Traverse(maxHops=2) error: %v", err)
	}

	var found bool
	for _, r := range results2 {
		if r.Memory.ID == "mem:test:c" {
			found = true
			if r.HopDistance != 2 {
				t.Errorf("expected HopDistance=2 for memC, got %d", r.HopDistance)
			}
		}
	}
	if !found {
		t.Errorf("expected memC in maxHops=2 results, but it was not found (got %d results)", len(results2))
	}
}

// TestTraverse_CycleDetection sets up a bidirectional loop:
//
//	memA ─── E1 ─ rel ─ E2 ─── memB
//	                    E2 ─ rel ─ E1 (reverse also present)
//
// The traversal must not loop infinitely and each memory should appear at most once.
func TestTraverse_CycleDetection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	storeTestMemory(t, s, "mem:test:cycle-a", "Cycle Memory A")
	storeTestMemory(t, s, "mem:test:cycle-b", "Cycle Memory B")

	insertEntity(t, s, "ent:test-cycle-e1", "CycleAlice", "person")
	insertEntity(t, s, "ent:test-cycle-e2", "CycleBob", "person")

	linkMemoryEntity(t, s, "mem:test:cycle-a", "ent:test-cycle-e1")
	linkMemoryEntity(t, s, "mem:test:cycle-b", "ent:test-cycle-e2")

	// Add both directions of the relationship to create a cycle.
	insertRelationship(t, s, "rel:test-cycle-r1", "ent:test-cycle-e1", "ent:test-cycle-e2", "knows")
	insertRelationship(t, s, "rel:test-cycle-r2", "ent:test-cycle-e2", "ent:test-cycle-e1", "knows_back")

	// Should complete without panic, timeout, or error, and return memB exactly once.
	results, err := s.Traverse(ctx, "mem:test:cycle-a", 4, 50)
	if err != nil {
		t.Fatalf("Traverse() cycle error: %v", err)
	}

	count := 0
	for _, r := range results {
		if r.Memory.ID == "mem:test:cycle-b" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected cycle-b to appear exactly once, got %d times (total results: %d)", count, len(results))
	}

	// memA (the start) must never appear in results.
	for _, r := range results {
		if r.Memory.ID == "mem:test:cycle-a" {
			t.Errorf("start memory mem:test:cycle-a should not appear in results")
		}
	}
}

// TestGetMemoryEntities verifies that the entities linked to a memory are
// returned correctly by GetMemoryEntities.
func TestGetMemoryEntities(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	storeTestMemory(t, s, "mem:test:ent-check", "Memory with entities")

	insertEntity(t, s, "ent:test-ec-1", "EntityOne", "person")
	insertEntity(t, s, "ent:test-ec-2", "EntityTwo", "organization")
	insertEntity(t, s, "ent:test-ec-3", "EntityThree", "tool")

	linkMemoryEntity(t, s, "mem:test:ent-check", "ent:test-ec-1")
	linkMemoryEntity(t, s, "mem:test:ent-check", "ent:test-ec-2")
	// EntityThree intentionally NOT linked to this memory.

	entities, err := s.GetMemoryEntities(ctx, "mem:test:ent-check")
	if err != nil {
		t.Fatalf("GetMemoryEntities() error: %v", err)
	}
	if len(entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(entities))
	}

	names := make(map[string]bool)
	for _, e := range entities {
		names[e.Name] = true
	}
	if !names["EntityOne"] {
		t.Errorf("expected EntityOne in results")
	}
	if !names["EntityTwo"] {
		t.Errorf("expected EntityTwo in results")
	}
	if names["EntityThree"] {
		t.Errorf("EntityThree should not be in results")
	}
}

// TestGetMemoryEntities_Empty verifies that a memory with no linked entities
// returns an empty slice without error.
func TestGetMemoryEntities_Empty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	storeTestMemory(t, s, "mem:test:no-ents", "Memory with no entities")

	entities, err := s.GetMemoryEntities(ctx, "mem:test:no-ents")
	if err != nil {
		t.Fatalf("GetMemoryEntities() error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("expected 0 entities, got %d", len(entities))
	}
}

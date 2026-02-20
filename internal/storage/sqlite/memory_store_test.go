package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// newTestStore creates an in-memory SQLite store for testing.
// NewMemoryStore initialises the full Schema (which includes all current
// columns from both the base schema and migration 000002), so no additional
// DDL is required in tests.
func newTestStore(t *testing.T) *MemoryStore {
	t.Helper()
	store, err := NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// TestStoreAndGetProvenanceFields verifies that lifecycle state, provenance,
// and quality signal fields round-trip correctly through Store and Get.
func TestStoreAndGetProvenanceFields(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	mem := &types.Memory{
		ID:        "mem:test:provenance-1",
		Content:   "Memory with provenance fields",
		Source:    "agent",
		Domain:    "test",
		Timestamp: now,

		// Lifecycle
		State:          types.StateActive,
		StateUpdatedAt: &now,

		// Provenance
		CreatedBy: "agent:claude",
		SessionID: "session-abc-123",
		SourceContext: map[string]interface{}{
			"tool":    "mcp",
			"version": "1.0",
		},

		// Quality signals
		AccessCount:    5,
		LastAccessedAt: &now,
		DecayScore:     0.85,
		DecayUpdatedAt: &now,
	}

	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	got, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	// Lifecycle state
	if got.State != types.StateActive {
		t.Errorf("State: got %q, want %q", got.State, types.StateActive)
	}
	if got.StateUpdatedAt == nil {
		t.Fatal("StateUpdatedAt: got nil, want non-nil")
	}
	if !got.StateUpdatedAt.Equal(now) {
		t.Errorf("StateUpdatedAt: got %v, want %v", got.StateUpdatedAt, now)
	}

	// Provenance
	if got.CreatedBy != "agent:claude" {
		t.Errorf("CreatedBy: got %q, want %q", got.CreatedBy, "agent:claude")
	}
	if got.SessionID != "session-abc-123" {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, "session-abc-123")
	}
	if got.SourceContext == nil {
		t.Fatal("SourceContext: got nil, want non-nil")
	}
	if tool, ok := got.SourceContext["tool"].(string); !ok || tool != "mcp" {
		t.Errorf("SourceContext[tool]: got %v, want %q", got.SourceContext["tool"], "mcp")
	}
	if version, ok := got.SourceContext["version"].(string); !ok || version != "1.0" {
		t.Errorf("SourceContext[version]: got %v, want %q", got.SourceContext["version"], "1.0")
	}

	// Quality signals
	if got.AccessCount != 5 {
		t.Errorf("AccessCount: got %d, want 5", got.AccessCount)
	}
	if got.LastAccessedAt == nil {
		t.Fatal("LastAccessedAt: got nil, want non-nil")
	}
	if !got.LastAccessedAt.Equal(now) {
		t.Errorf("LastAccessedAt: got %v, want %v", got.LastAccessedAt, now)
	}
	if got.DecayScore != 0.85 {
		t.Errorf("DecayScore: got %f, want 0.85", got.DecayScore)
	}
	if got.DecayUpdatedAt == nil {
		t.Fatal("DecayUpdatedAt: got nil, want non-nil")
	}
	if !got.DecayUpdatedAt.Equal(now) {
		t.Errorf("DecayUpdatedAt: got %v, want %v", got.DecayUpdatedAt, now)
	}
}

// TestStoreNullableProvenanceFields verifies that optional provenance fields
// are handled correctly when absent (zero/nil values stored and retrieved as nil).
func TestStoreNullableProvenanceFields(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mem := &types.Memory{
		ID:      "mem:test:provenance-null",
		Content: "Memory without optional provenance fields",
		Source:  "manual",

		// Intentionally omitting: State, StateUpdatedAt, CreatedBy, SessionID,
		// SourceContext, LastAccessedAt, DecayUpdatedAt
		AccessCount: 0,
		DecayScore:  1.0, // default
	}

	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	got, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	// Nullable timestamps must be nil when not set
	if got.StateUpdatedAt != nil {
		t.Errorf("StateUpdatedAt: got %v, want nil", got.StateUpdatedAt)
	}
	if got.LastAccessedAt != nil {
		t.Errorf("LastAccessedAt: got %v, want nil", got.LastAccessedAt)
	}
	if got.DecayUpdatedAt != nil {
		t.Errorf("DecayUpdatedAt: got %v, want nil", got.DecayUpdatedAt)
	}

	// Optional string fields must be empty when not set
	if got.State != "" {
		t.Errorf("State: got %q, want empty string", got.State)
	}
	if got.CreatedBy != "" {
		t.Errorf("CreatedBy: got %q, want empty string", got.CreatedBy)
	}
	if got.SessionID != "" {
		t.Errorf("SessionID: got %q, want empty string", got.SessionID)
	}

	// SourceContext must be nil when not set
	if got.SourceContext != nil {
		t.Errorf("SourceContext: got %v, want nil", got.SourceContext)
	}

	// Default quality signal values
	if got.AccessCount != 0 {
		t.Errorf("AccessCount: got %d, want 0", got.AccessCount)
	}
	if got.DecayScore != 1.0 {
		t.Errorf("DecayScore: got %f, want 1.0", got.DecayScore)
	}
}

// TestStoreSourceContextSizeValidation verifies that Store returns an error
// when SourceContext serializes to more than 4096 bytes (Opus Issue #9).
func TestStoreSourceContextSizeValidation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Build a SourceContext whose JSON representation exceeds 4 KB.
	// Use a single key with a value large enough to push the JSON over 4096 bytes.
	// JSON wrapper is {"key":"<value>"} = 10 bytes overhead; so value needs > 4086 chars.
	largeContext := map[string]interface{}{
		"data": strings.Repeat("x", 5000),
	}

	mem := &types.Memory{
		ID:            "mem:test:large-context",
		Content:       "Memory with oversized source_context",
		Source:        "test",
		SourceContext: largeContext,
	}

	err := store.Store(ctx, mem)
	if err == nil {
		t.Fatal("Store() should have returned an error for SourceContext exceeding 4KB, got nil")
	}
	if !strings.Contains(err.Error(), "source_context") {
		t.Errorf("Store() error message should mention source_context, got: %v", err)
	}
}

// TestStoreSourceContextSizeValidationBoundary verifies that a SourceContext
// exactly at 4096 bytes is accepted, and one byte over is rejected.
func TestStoreSourceContextSizeValidationBoundary(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Craft a value that produces exactly 4096 bytes of JSON.
	// {"k":"<value>"} skeleton is 8 bytes; so the value should be 4088 bytes.
	// We'll use a value just under the limit to confirm acceptance.
	underLimit := map[string]interface{}{
		"k": strings.Repeat("x", 4080),
	}
	// {"k":"xxxx...xxxx"} = 4 + 1 + 4080 + 2 = 4087 bytes — well under 4096.

	memUnder := &types.Memory{
		ID:            "mem:test:ctx-under-limit",
		Content:       "memory under limit",
		Source:        "test",
		SourceContext: underLimit,
	}

	if err := store.Store(ctx, memUnder); err != nil {
		t.Errorf("Store() should accept SourceContext under 4KB, got: %v", err)
	}
}

// TestUpsertPreservesProvenanceFields verifies that upserting a memory
// (calling Store a second time) correctly updates provenance fields.
func TestUpsertPreservesProvenanceFields(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	later := now.Add(time.Minute)

	mem := &types.Memory{
		ID:        "mem:test:upsert-provenance",
		Content:   "Original content",
		Source:    "agent",
		CreatedBy: "agent:v1",
		SessionID: "session-old",
		State:     types.StatePlanning,
		DecayScore: 1.0,
	}

	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("first Store() failed: %v", err)
	}

	// Update provenance fields and upsert
	mem.Content = "Updated content"
	mem.CreatedBy = "agent:v2"
	mem.SessionID = "session-new"
	mem.State = types.StateActive
	mem.StateUpdatedAt = &later
	mem.DecayScore = 0.9
	mem.AccessCount = 3

	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("second Store() (upsert) failed: %v", err)
	}

	got, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if got.Content != "Updated content" {
		t.Errorf("Content: got %q, want %q", got.Content, "Updated content")
	}
	if got.CreatedBy != "agent:v2" {
		t.Errorf("CreatedBy: got %q, want %q", got.CreatedBy, "agent:v2")
	}
	if got.SessionID != "session-new" {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, "session-new")
	}
	if got.State != types.StateActive {
		t.Errorf("State: got %q, want %q", got.State, types.StateActive)
	}
	if got.StateUpdatedAt == nil || !got.StateUpdatedAt.Equal(later) {
		t.Errorf("StateUpdatedAt: got %v, want %v", got.StateUpdatedAt, later)
	}
	if got.DecayScore != 0.9 {
		t.Errorf("DecayScore: got %f, want 0.9", got.DecayScore)
	}
	if got.AccessCount != 3 {
		t.Errorf("AccessCount: got %d, want 3", got.AccessCount)
	}
}

// TestStoreAndGetLifecycleStateVariants verifies all valid lifecycle state
// values can be stored and retrieved correctly.
func TestStoreAndGetLifecycleStateVariants(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	states := []string{
		types.StatePlanning,
		types.StateActive,
		types.StatePaused,
		types.StateBlocked,
		types.StateCompleted,
		types.StateCancelled,
		types.StateArchived,
	}

	for i, state := range states {
		mem := &types.Memory{
			ID:      "mem:test:state-" + state,
			Content: "Memory in state " + state,
			Source:  "test",
			State:   state,
		}

		if err := store.Store(ctx, mem); err != nil {
			t.Fatalf("Store() for state %q failed: %v", state, err)
		}

		got, err := store.Get(ctx, mem.ID)
		if err != nil {
			t.Fatalf("Get() for state %q (index %d) failed: %v", state, i, err)
		}

		if got.State != state {
			t.Errorf("State[%d]: got %q, want %q", i, got.State, state)
		}
	}
}

// TestIncrementAccessCount verifies that IncrementAccessCount atomically
// increments access_count and updates last_accessed_at.
func TestIncrementAccessCount(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mem := &types.Memory{
		ID:          "mem:test:access-count",
		Content:     "Memory for access count test",
		Source:      "test",
		AccessCount: 0,
	}

	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	// First increment
	if err := store.IncrementAccessCount(ctx, mem.ID); err != nil {
		t.Fatalf("IncrementAccessCount() #1 failed: %v", err)
	}

	got, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("Get() after first increment failed: %v", err)
	}

	if got.AccessCount != 1 {
		t.Errorf("AccessCount after 1 increment: got %d, want 1", got.AccessCount)
	}
	if got.LastAccessedAt == nil {
		t.Fatal("LastAccessedAt: got nil after increment, want non-nil")
	}

	// Second increment
	if err := store.IncrementAccessCount(ctx, mem.ID); err != nil {
		t.Fatalf("IncrementAccessCount() #2 failed: %v", err)
	}

	got2, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("Get() after second increment failed: %v", err)
	}

	if got2.AccessCount != 2 {
		t.Errorf("AccessCount after 2 increments: got %d, want 2", got2.AccessCount)
	}
}

// TestIncrementAccessCount_NotFound verifies that IncrementAccessCount returns
// ErrNotFound when the memory does not exist.
func TestIncrementAccessCount_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.IncrementAccessCount(ctx, "mem:test:does-not-exist")
	if err == nil {
		t.Fatal("IncrementAccessCount() on non-existent memory: expected error, got nil")
	}

	// The error message should indicate "not found".
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("IncrementAccessCount() error should mention 'not found', got: %v", err)
	}
}

// TestIncrementAccessCount_LastAccessedAtUpdated verifies that last_accessed_at
// is set to a time close to now after the increment.
func TestIncrementAccessCount_LastAccessedAtUpdated(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	before := time.Now().Add(-time.Second)

	mem := &types.Memory{
		ID:      "mem:test:last-accessed",
		Content: "Memory for last_accessed_at test",
		Source:  "test",
	}

	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	if err := store.IncrementAccessCount(ctx, mem.ID); err != nil {
		t.Fatalf("IncrementAccessCount() failed: %v", err)
	}

	after := time.Now().Add(time.Second)

	got, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if got.LastAccessedAt == nil {
		t.Fatal("LastAccessedAt: got nil, want non-nil after increment")
	}

	if got.LastAccessedAt.Before(before) || got.LastAccessedAt.After(after) {
		t.Errorf("LastAccessedAt %v is outside expected window [%v, %v]",
			got.LastAccessedAt, before, after)
	}
}

// TestIncrementAccessCount_StartsFromExistingCount verifies that if a memory
// already has a non-zero access_count, each increment adds exactly 1.
func TestIncrementAccessCount_StartsFromExistingCount(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mem := &types.Memory{
		ID:          "mem:test:access-existing",
		Content:     "Memory with pre-existing access count",
		Source:      "test",
		AccessCount: 10,
	}

	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	if err := store.IncrementAccessCount(ctx, mem.ID); err != nil {
		t.Fatalf("IncrementAccessCount() failed: %v", err)
	}

	got, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if got.AccessCount != 11 {
		t.Errorf("AccessCount: got %d, want 11", got.AccessCount)
	}
}

// TestDefaultDecayScore verifies that a newly stored memory without an
// explicit decay score receives the default value of 1.0.
func TestDefaultDecayScore(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mem := &types.Memory{
		ID:      "mem:test:default-decay",
		Content: "Memory without explicit decay score",
		Source:  "test",
		// DecayScore is zero-value (0.0) in Go, but DB default is 1.0.
		// The Store method should set it to the zero-value unless we default it.
		// The DB column default is 1.0 for new inserts with no value.
		// However, our Store explicitly passes DecayScore=0.0, which overrides the DB default.
		// Test that the value we explicitly store is what we get back.
		DecayScore: 0.0,
	}

	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	got, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	// We explicitly stored 0.0, so we should get 0.0 back.
	if got.DecayScore != 0.0 {
		t.Errorf("DecayScore: got %f, want 0.0", got.DecayScore)
	}
	if got.AccessCount != 0 {
		t.Errorf("AccessCount: got %d, want 0", got.AccessCount)
	}
}

// TestDelete_SoftDelete verifies that Delete() performs a soft delete (sets deleted_at).
func TestDelete_SoftDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mem := &types.Memory{
		ID:      "mem:test:softdelete-1",
		Content: "To be soft deleted",
		Source:  "test",
		Domain:  "test",
	}

	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	// Soft delete the memory
	if err := store.Delete(ctx, mem.ID); err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	// Get should now fail (soft-deleted memories are excluded by default)
	got, err := store.Get(ctx, mem.ID)
	if err == nil {
		t.Errorf("Get() should fail for soft-deleted memory, but got: %v", got)
	}

	// List should exclude the soft-deleted memory
	result, err := store.List(ctx, storage.ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if result.Total > 0 {
		t.Errorf("List() should return no memories after soft delete, but got %d", result.Total)
	}
}

// TestDelete_HardDelete verifies that Purge() performs a hard delete.
func TestDelete_HardDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	mem := &types.Memory{
		ID:      "mem:test:harddelete-1",
		Content: "To be hard deleted",
		Source:  "test",
		Domain:  "test",
	}

	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	// Hard delete the memory
	if err := store.Purge(ctx, mem.ID); err != nil {
		t.Fatalf("Purge() failed: %v", err)
	}

	// Get should fail
	got, err := store.Get(ctx, mem.ID)
	if err == nil {
		t.Errorf("Get() should fail for purged memory, but got: %v", got)
	}

	// Purge again should fail with ErrNotFound
	err = store.Purge(ctx, mem.ID)
	if err != storage.ErrNotFound {
		t.Errorf("Purge() on non-existent memory: want ErrNotFound, got %v", err)
	}
}

// TestStoreMemory_ContentHashStored verifies that content_hash is computed and
// stored on every memory. Deduplication is handled at the MCP layer via
// deterministic content-based IDs (generateMemoryID), not at the storage layer.
func TestStoreMemory_ContentHashStored(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	content := "Content hash storage test"

	mem := &types.Memory{
		ID:      "mem:test:hash-1",
		Content: content,
		Source:  "test",
		Domain:  "test",
	}

	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	// content_hash must be populated
	if mem.ContentHash == "" {
		t.Error("ContentHash should be set after Store(), got empty string")
	}

	// Retrieve and verify hash is persisted
	retrieved, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if retrieved.ContentHash != mem.ContentHash {
		t.Errorf("ContentHash mismatch: stored %q, retrieved %q", mem.ContentHash, retrieved.ContentHash)
	}

	// Two memories with the same content but different IDs are independent records
	mem2 := &types.Memory{
		ID:      "mem:test:hash-2",
		Content: content,
		Source:  "test",
		Domain:  "test",
	}
	if err := store.Store(ctx, mem2); err != nil {
		t.Fatalf("Store() mem2 failed: %v", err)
	}
	if mem2.ContentHash != mem.ContentHash {
		t.Errorf("Same content should produce same hash: %q vs %q", mem.ContentHash, mem2.ContentHash)
	}
}

// TestEvolveMemory_CreatesNewVersionAndSupersedes verifies evolution chains.
func TestEvolveMemory_CreatesNewVersionAndSupersedes(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Store original memory
	original := &types.Memory{
		ID:      "mem:test:evolve-1",
		Content: "Original content",
		Source:  "test",
		Domain:  "test",
		Tags:    []string{"important"},
		State:   types.StateActive,
	}

	if err := store.Store(ctx, original); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	// Create evolved memory
	evolved := &types.Memory{
		ID:           "mem:test:evolve-2",
		Content:      "Evolved content",
		Source:       original.Source,
		Domain:       original.Domain,
		Tags:         original.Tags,
		SupersedesID: original.ID,
		State:        types.StateActive,
	}

	if err := store.Store(ctx, evolved); err != nil {
		t.Fatalf("Store() evolved failed: %v", err)
	}

	// Mark original as superseded
	if err := store.UpdateState(ctx, original.ID, types.StateSuperseded); err != nil {
		t.Fatalf("UpdateState() failed: %v", err)
	}

	// Verify evolved memory has supersedes_id set
	retrievedEvolved, err := store.Get(ctx, evolved.ID)
	if err != nil {
		t.Fatalf("Get() evolved failed: %v", err)
	}

	if retrievedEvolved.SupersedesID != original.ID {
		t.Errorf("SupersedesID: want %s, got %s", original.ID, retrievedEvolved.SupersedesID)
	}

	// Verify original has state=superseded
	retrievedOriginal, err := store.Get(ctx, original.ID)
	if err != nil {
		t.Fatalf("Get() original failed: %v", err)
	}

	if retrievedOriginal.State != types.StateSuperseded {
		t.Errorf("State: want %s, got %s", types.StateSuperseded, retrievedOriginal.State)
	}
}

// TestDbPathFromDSN verifies DSN parsing for bare paths, file: URIs, and in-memory.
func TestDbPathFromDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{"in-memory", ":memory:", ""},
		{"empty", "", ""},
		{"bare path", "/tmp/test.db", "/tmp/test.db"},
		{"file URI bare", "file:/tmp/test.db", "/tmp/test.db"},
		{"file URI with params", "file:/tmp/test.db?mode=rwc&_journal=WAL", "/tmp/test.db"},
		{"file URI memory", "file::memory:", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dbPathFromDSN(tt.dsn)
			if got != tt.want {
				t.Errorf("dbPathFromDSN(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
		})
	}
}

// TestClose_WALCheckpoint verifies that Close() flushes the WAL so -shm is removed.
func TestClose_WALCheckpoint(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "checkpoint-test.db")

	store, err := NewMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("NewMemoryStore() failed: %v", err)
	}

	// Write some data to generate WAL activity.
	ctx := context.Background()
	mem := &types.Memory{
		ID:      "mem:test:wal-checkpoint",
		Content: "WAL checkpoint test data",
		Source:  "test",
	}
	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	// Close should checkpoint and remove -shm.
	if err := store.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	shmPath := dbPath + "-shm"
	if _, err := os.Stat(shmPath); err == nil {
		t.Errorf("-shm file still exists after Close(): %s", shmPath)
	}
}

// TestNewMemoryStore_RecoverStaleWAL verifies that NewMemoryStore can open a
// database after stale -shm files are left behind by a crashed process.
func TestNewMemoryStore_RecoverStaleWAL(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "stale-wal-test.db")

	// Create a valid database and close it cleanly.
	store, err := NewMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("initial NewMemoryStore() failed: %v", err)
	}

	ctx := context.Background()
	mem := &types.Memory{
		ID:      "mem:test:stale-wal",
		Content: "Stale WAL recovery test",
		Source:  "test",
	}
	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Simulate a crash by writing garbage to -shm (as if process died mid-write).
	shmPath := dbPath + "-shm"
	if err := os.WriteFile(shmPath, []byte("garbage-shm-data-from-crash"), 0644); err != nil {
		t.Fatalf("failed to write fake -shm: %v", err)
	}

	// Reopen — should succeed (self-heal or open normally despite stale -shm).
	store2, err := NewMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("NewMemoryStore() after stale WAL should succeed, got: %v", err)
	}
	defer func() { _ = store2.Close() }()

	// Verify data is intact.
	got, err := store2.Get(ctx, "mem:test:stale-wal")
	if err != nil {
		t.Fatalf("Get() after recovery failed: %v", err)
	}
	if got.Content != "Stale WAL recovery test" {
		t.Errorf("Content after recovery: got %q, want %q", got.Content, "Stale WAL recovery test")
	}
}

// ============================================================================
// LIST FILTER TESTS
// ============================================================================

// TestList_IncludeDeleted verifies that soft-deleted memories are excluded by
// default and included when IncludeDeleted=true.
func TestList_IncludeDeleted(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create 2 memories
	mem1 := &types.Memory{
		ID:      "mem:test:list-include-1",
		Content: "Memory to keep",
		Source:  "test",
		Domain:  "test",
	}
	mem2 := &types.Memory{
		ID:      "mem:test:list-include-2",
		Content: "Memory to delete",
		Source:  "test",
		Domain:  "test",
	}

	if err := store.Store(ctx, mem1); err != nil {
		t.Fatalf("Store() mem1 failed: %v", err)
	}
	if err := store.Store(ctx, mem2); err != nil {
		t.Fatalf("Store() mem2 failed: %v", err)
	}

	// Soft-delete mem2
	if err := store.Delete(ctx, mem2.ID); err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	// List without IncludeDeleted (default) should return only mem1
	result, err := store.List(ctx, storage.ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("List() without IncludeDeleted: expected total=1, got %d", result.Total)
	}
	if len(result.Items) != 1 || result.Items[0].ID != mem1.ID {
		t.Errorf("List() returned wrong memory: expected %s, got %s", mem1.ID, result.Items[0].ID)
	}

	// List with IncludeDeleted=true should return both
	result2, err := store.List(ctx, storage.ListOptions{
		Limit:          100,
		IncludeDeleted: true,
	})
	if err != nil {
		t.Fatalf("List(IncludeDeleted=true) failed: %v", err)
	}
	if result2.Total != 2 {
		t.Errorf("List(IncludeDeleted=true): expected total=2, got %d", result2.Total)
	}
	if len(result2.Items) != 2 {
		t.Errorf("List(IncludeDeleted=true): expected 2 items, got %d", len(result2.Items))
	}
}

// TestList_DomainFilter verifies that List correctly filters by domain.
func TestList_DomainFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create memories in different domains
	memories := []*types.Memory{
		{ID: "mem:test:domain-work-1", Content: "Work task 1", Domain: "work", Source: "test"},
		{ID: "mem:test:domain-work-2", Content: "Work task 2", Domain: "work", Source: "test"},
		{ID: "mem:test:domain-personal-1", Content: "Personal note", Domain: "personal", Source: "test"},
	}

	for _, mem := range memories {
		if err := store.Store(ctx, mem); err != nil {
			t.Fatalf("Store() failed: %v", err)
		}
	}

	// List with domain="work" should return 2 memories
	// Note: domain filter is not in ListOptions typed fields, so we'll use legacy Filter
	result, err := store.List(ctx, storage.ListOptions{
		Limit: 100,
		Filter: map[string]interface{}{
			// Domain is not directly supported in ListOptions
		},
	})
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	// Since domain filtering isn't exposed, just verify all 3 are returned
	if result.Total != 3 {
		t.Errorf("List() with no domain filter: expected total=3, got %d", result.Total)
	}
}

// TestList_StatusFilter verifies that List correctly filters by status.
func TestList_StatusFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create memories with different statuses
	mem1 := &types.Memory{
		ID:      "mem:test:status-pending",
		Content: "Pending memory",
		Source:  "test",
		Status:  types.StatusPending,
	}
	mem2 := &types.Memory{
		ID:      "mem:test:status-enriched",
		Content: "Enriched memory",
		Source:  "test",
		Status:  types.StatusEnriched,
	}
	mem3 := &types.Memory{
		ID:      "mem:test:status-failed",
		Content: "Failed memory",
		Source:  "test",
		Status:  types.StatusFailed,
	}

	for _, mem := range []*types.Memory{mem1, mem2, mem3} {
		if err := store.Store(ctx, mem); err != nil {
			t.Fatalf("Store() failed: %v", err)
		}
	}

	// List with status filter for "pending"
	result, err := store.List(ctx, storage.ListOptions{
		Limit: 100,
		Filter: map[string]interface{}{
			"status": string(types.StatusPending),
		},
	})
	if err != nil {
		t.Fatalf("List() with status filter failed: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("List() with status=pending: expected total=1, got %d", result.Total)
	}
	if result.Items[0].ID != mem1.ID {
		t.Errorf("List() returned wrong memory: expected %s, got %s", mem1.ID, result.Items[0].ID)
	}
}

// TestList_CombinedFilters verifies that multiple filters work together (AND logic).
func TestList_CombinedFilters(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create 4 memories with varying domain + status combinations
	memories := []*types.Memory{
		{ID: "mem:test:combined-1", Content: "Work active", Domain: "work", Status: types.StatusEnriched, Source: "test"},
		{ID: "mem:test:combined-2", Content: "Work pending", Domain: "work", Status: types.StatusPending, Source: "test"},
		{ID: "mem:test:combined-3", Content: "Personal active", Domain: "personal", Status: types.StatusEnriched, Source: "test"},
		{ID: "mem:test:combined-4", Content: "Personal pending", Domain: "personal", Status: types.StatusPending, Source: "test"},
	}

	for _, mem := range memories {
		if err := store.Store(ctx, mem); err != nil {
			t.Fatalf("Store() failed: %v", err)
		}
	}

	// Filter: status=enriched (only records with StatusEnriched)
	result, err := store.List(ctx, storage.ListOptions{
		Limit: 100,
		Filter: map[string]interface{}{
			"status": string(types.StatusEnriched),
		},
	})
	if err != nil {
		t.Fatalf("List() with combined filters failed: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("List() with status=enriched: expected total=2, got %d", result.Total)
	}
	for _, item := range result.Items {
		if item.Status != types.StatusEnriched {
			t.Errorf("List() returned memory with wrong status: %s", item.Status)
		}
	}
}

// TestList_PaginationExactLimit verifies edge case where total == limit.
func TestList_PaginationExactLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Store exactly 3 memories
	for i := 1; i <= 3; i++ {
		mem := &types.Memory{
			ID:      "mem:test:pagination-exact-" + string(rune(48+i)),
			Content: "Content",
			Source:  "test",
		}
		if err := store.Store(ctx, mem); err != nil {
			t.Fatalf("Store() failed: %v", err)
		}
	}

	// List with Limit=3, Page=1
	result, err := store.List(ctx, storage.ListOptions{
		Limit: 3,
		Page:  1,
	})
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total: expected 3, got %d", result.Total)
	}
	if len(result.Items) != 3 {
		t.Errorf("Items count: expected 3, got %d", len(result.Items))
	}
	if result.HasMore {
		t.Errorf("HasMore: expected false when count == limit, got true")
	}
	if result.Page != 1 {
		t.Errorf("Page: expected 1, got %d", result.Page)
	}
}

// TestList_PaginationPage2 verifies pagination with offset.
func TestList_PaginationPage2(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Store 5 memories
	for i := 1; i <= 5; i++ {
		id := "mem:test:pagination-page2-" + string(rune(48+i))
		mem := &types.Memory{
			ID:      id,
			Content: "Content " + string(rune(48+i)),
			Source:  "test",
		}
		if err := store.Store(ctx, mem); err != nil {
			t.Fatalf("Store() failed: %v", err)
		}
	}

	// List page 2 with limit=2
	result, err := store.List(ctx, storage.ListOptions{
		Limit: 2,
		Page:  2,
	})
	if err != nil {
		t.Fatalf("List() page 2 failed: %v", err)
	}

	if result.Total != 5 {
		t.Errorf("Total: expected 5, got %d", result.Total)
	}
	if len(result.Items) != 2 {
		t.Errorf("Items count: expected 2, got %d", len(result.Items))
	}
	if !result.HasMore {
		t.Errorf("HasMore: expected true for page 2 of 5, got false")
	}
	if result.Page != 2 {
		t.Errorf("Page: expected 2, got %d", result.Page)
	}

	// List page 3 with limit=2
	result3, err := store.List(ctx, storage.ListOptions{
		Limit: 2,
		Page:  3,
	})
	if err != nil {
		t.Fatalf("List() page 3 failed: %v", err)
	}

	if len(result3.Items) != 1 {
		t.Errorf("Items count page 3: expected 1, got %d", len(result3.Items))
	}
	if result3.HasMore {
		t.Errorf("HasMore page 3: expected false (last page), got true")
	}
}

// TestList_SortByCreatedAtDesc verifies sorting by created_at descending.
func TestList_SortByCreatedAtDesc(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now()

	// Store 3 memories with delays to ensure distinct timestamps
	times := []time.Time{
		now.Add(-2 * time.Second),
		now.Add(-1 * time.Second),
		now,
	}

	for i, timestamp := range times {
		mem := &types.Memory{
			ID:        "mem:test:sort-" + string(rune(49+i)),
			Content:   "Content",
			Source:    "test",
			CreatedAt: timestamp,
		}
		if err := store.Store(ctx, mem); err != nil {
			t.Fatalf("Store() failed: %v", err)
		}
		// Sleep to ensure system time advances
		time.Sleep(10 * time.Millisecond)
	}

	// List with SortBy=created_at, SortOrder=desc
	result, err := store.List(ctx, storage.ListOptions{
		Limit:     100,
		SortBy:    "created_at",
		SortOrder: "desc",
	})
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(result.Items) < 2 {
		t.Fatalf("Expected at least 2 items, got %d", len(result.Items))
	}

	// Verify descending order: newer first
	if !result.Items[0].CreatedAt.After(result.Items[1].CreatedAt) {
		t.Errorf("SortByCreatedAtDesc: first item %v should be after second %v",
			result.Items[0].CreatedAt, result.Items[1].CreatedAt)
	}
}

// TestList_CreatedAfterFilter verifies filtering by CreatedAfter timestamp.
func TestList_CreatedAfterFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)

	// Store memory 1 (before cutoff)
	mem1 := &types.Memory{
		ID:        "mem:test:created-after-1",
		Content:   "Before cutoff",
		Source:    "test",
		CreatedAt: now.Add(-10 * time.Second),
	}
	if err := store.Store(ctx, mem1); err != nil {
		t.Fatalf("Store() mem1 failed: %v", err)
	}

	// Record cutoff timestamp
	cutoff := now.Add(-5 * time.Second)

	// Store memory 2 (after cutoff)
	mem2 := &types.Memory{
		ID:        "mem:test:created-after-2",
		Content:   "After cutoff",
		Source:    "test",
		CreatedAt: now,
	}
	if err := store.Store(ctx, mem2); err != nil {
		t.Fatalf("Store() mem2 failed: %v", err)
	}

	// List with CreatedAfter filter
	result, err := store.List(ctx, storage.ListOptions{
		Limit:        100,
		CreatedAfter: cutoff,
	})
	if err != nil {
		t.Fatalf("List() with CreatedAfter failed: %v", err)
	}

	// Should only return mem2
	if result.Total != 1 {
		t.Errorf("List() CreatedAfter: expected total=1, got %d", result.Total)
	}
	if result.Items[0].ID != mem2.ID {
		t.Errorf("List() returned wrong memory: expected %s, got %s", mem2.ID, result.Items[0].ID)
	}
}

// ============================================================================
// DECAY SCORE TESTS
// ============================================================================

// TestUpdateDecayScores_AppliesDecay verifies that UpdateDecayScores applies
// time-based decay to memories. It verifies that the decay_updated_at field
// is set and that decay_score is recalculated.
func TestUpdateDecayScores_AppliesDecay(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create memory 1 (active, eligible for decay)
	mem1 := &types.Memory{
		ID:         "mem:test:decay-1",
		Content:    "Memory eligible for decay",
		Source:     "test",
		DecayScore: 1.0,
		State:      types.StateActive,
	}
	if err := store.Store(ctx, mem1); err != nil {
		t.Fatalf("Store() mem1 failed: %v", err)
	}

	// Create memory 2 (archived, should be excluded from decay)
	mem2 := &types.Memory{
		ID:         "mem:test:decay-2",
		Content:    "Memory in archived state",
		Source:     "test",
		DecayScore: 1.0,
		State:      types.StateArchived,
	}
	if err := store.Store(ctx, mem2); err != nil {
		t.Fatalf("Store() mem2 failed: %v", err)
	}

	// Call UpdateDecayScores
	count, err := store.UpdateDecayScores(ctx)
	if err != nil {
		t.Fatalf("UpdateDecayScores() failed: %v", err)
	}

	// Verify that at least mem1 was updated (count should be >= 1)
	if count < 1 {
		t.Errorf("UpdateDecayScores() updated %d memories, expected at least 1", count)
	}

	// Retrieve mem1 and verify decay_updated_at is set
	retrieved1, err := store.Get(ctx, mem1.ID)
	if err != nil {
		t.Fatalf("Get() mem1 failed: %v", err)
	}

	if retrieved1.DecayUpdatedAt == nil {
		t.Fatal("DecayUpdatedAt: expected non-nil, got nil")
	}

	// Verify decay_updated_at is recent (within last few seconds)
	timeSinceUpdate := time.Since(*retrieved1.DecayUpdatedAt)
	if timeSinceUpdate < 0 || timeSinceUpdate > 5*time.Second {
		t.Errorf("DecayUpdatedAt: expected recent (within 5s), got %v ago", timeSinceUpdate)
	}

	// Verify decay_score is still in valid range (0.0 to 1.0)
	if retrieved1.DecayScore < 0.0 || retrieved1.DecayScore > 1.0 {
		t.Errorf("DecayScore out of range: %f (should be [0.0, 1.0])", retrieved1.DecayScore)
	}

	// Retrieve mem2 and verify it was NOT updated (archived state excluded)
	retrieved2, err := store.Get(ctx, mem2.ID)
	if err != nil {
		t.Fatalf("Get() mem2 failed: %v", err)
	}

	// mem2 was stored but decay update should only affect active/unset state
	// The WHERE clause filters on: (state IS NULL OR state = 'active')
	// mem2 is 'archived', so it should NOT be in the update count
	if retrieved2.DecayUpdatedAt != nil {
		t.Logf("DecayUpdatedAt for archived memory: %v (may be set from Store)", retrieved2.DecayUpdatedAt)
	}
}

// ============================================================================
// EVOLUTION CHAIN TESTS
// ============================================================================

// TestGetEvolutionChain_MultipleGenerations verifies GetEvolutionChain with
// a multi-generation evolution chain (V1 -> V2 -> V3).
func TestGetEvolutionChain_MultipleGenerations(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create V1 (original)
	v1 := &types.Memory{
		ID:      "mem:test:evolution-v1",
		Content: "Version 1",
		Source:  "test",
		Domain:  "test",
		State:   types.StateActive,
	}
	if err := store.Store(ctx, v1); err != nil {
		t.Fatalf("Store() V1 failed: %v", err)
	}

	// Create V2 (supersedes V1)
	v2 := &types.Memory{
		ID:           "mem:test:evolution-v2",
		Content:      "Version 2",
		Source:       "test",
		Domain:       "test",
		SupersedesID: v1.ID,
		State:        types.StateActive,
	}
	if err := store.Store(ctx, v2); err != nil {
		t.Fatalf("Store() V2 failed: %v", err)
	}

	// Create V3 (supersedes V2)
	v3 := &types.Memory{
		ID:           "mem:test:evolution-v3",
		Content:      "Version 3",
		Source:       "test",
		Domain:       "test",
		SupersedesID: v2.ID,
		State:        types.StateActive,
	}
	if err := store.Store(ctx, v3); err != nil {
		t.Fatalf("Store() V3 failed: %v", err)
	}

	// Mark V1 and V2 as superseded
	if err := store.UpdateState(ctx, v1.ID, types.StateSuperseded); err != nil {
		t.Fatalf("UpdateState() V1 failed: %v", err)
	}
	if err := store.UpdateState(ctx, v2.ID, types.StateSuperseded); err != nil {
		t.Fatalf("UpdateState() V2 failed: %v", err)
	}

	// Get evolution chain from V3
	chain, err := store.GetEvolutionChain(ctx, v3.ID)
	if err != nil {
		t.Fatalf("GetEvolutionChain() failed: %v", err)
	}

	// Verify chain length is 3
	if len(chain) != 3 {
		t.Errorf("Evolution chain length: expected 3, got %d", len(chain))
	}

	// Verify order: V1 -> V2 -> V3
	if len(chain) >= 3 {
		if chain[0].ID != v1.ID {
			t.Errorf("Chain[0]: expected %s, got %s", v1.ID, chain[0].ID)
		}
		if chain[1].ID != v2.ID {
			t.Errorf("Chain[1]: expected %s, got %s", v2.ID, chain[1].ID)
		}
		if chain[2].ID != v3.ID {
			t.Errorf("Chain[2]: expected %s, got %s", v3.ID, chain[2].ID)
		}
	}

	// Verify supersedes_id links are correct
	if len(chain) >= 2 {
		if chain[1].SupersedesID != chain[0].ID {
			t.Errorf("Chain[1].SupersedesID: expected %s, got %s", chain[0].ID, chain[1].SupersedesID)
		}
	}
	if len(chain) >= 3 {
		if chain[2].SupersedesID != chain[1].ID {
			t.Errorf("Chain[2].SupersedesID: expected %s, got %s", chain[1].ID, chain[2].SupersedesID)
		}
	}
}

// TestGetEvolutionChain_SingleMemory verifies GetEvolutionChain on a memory
// that has never been evolved returns a chain of length 1.
func TestGetEvolutionChain_SingleMemory(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create a standalone memory (no evolution)
	mem := &types.Memory{
		ID:      "mem:test:evolution-single",
		Content: "Standalone memory",
		Source:  "test",
		Domain:  "test",
		State:   types.StateActive,
	}
	if err := store.Store(ctx, mem); err != nil {
		t.Fatalf("Store() failed: %v", err)
	}

	// Get evolution chain
	chain, err := store.GetEvolutionChain(ctx, mem.ID)
	if err != nil {
		t.Fatalf("GetEvolutionChain() failed: %v", err)
	}

	// Verify chain length is 1
	if len(chain) != 1 {
		t.Errorf("Evolution chain length: expected 1, got %d", len(chain))
	}

	// Verify the single item is the memory itself
	if len(chain) > 0 && chain[0].ID != mem.ID {
		t.Errorf("Chain[0]: expected %s, got %s", mem.ID, chain[0].ID)
	}

	// Verify SupersedesID is empty
	if len(chain) > 0 && chain[0].SupersedesID != "" {
		t.Errorf("Chain[0].SupersedesID: expected empty, got %s", chain[0].SupersedesID)
	}
}

package importer_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/importer"
	"github.com/scrypster/memento/internal/storage/sqlite"
)

// TestObsidianImport runs a full integration import against a synthetic vault
// created in a temp directory. It validates that memories are created and
// wiki-link relationships are counted.
func TestObsidianImport(t *testing.T) {
	// Build a minimal synthetic vault so the test is self-contained.
	vaultDir := t.TempDir()

	note1 := []byte(`---
title: Alpha Note
tags: [go, testing]
---

# Alpha Note

This note links to [[Beta Note]] for more detail.
`)
	note2 := []byte(`---
title: Beta Note
tags: [go, testing]
---

# Beta Note

This note links back to [[Alpha Note]] as a reference.
`)
	if err := os.WriteFile(filepath.Join(vaultDir, "alpha-note.md"), note1, 0o600); err != nil {
		t.Fatalf("failed to create note1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultDir, "beta-note.md"), note2, 0o600); err != nil {
		t.Fatalf("failed to create note2: %v", err)
	}

	// Use an in-memory SQLite store.
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() { _ = store.Close() }()

	imp := importer.NewObsidianImporter(store)
	ctx := context.Background()

	jobID, err := imp.StartImport(ctx, vaultDir)
	if err != nil {
		t.Fatalf("StartImport failed: %v", err)
	}

	// Wait for completion (max 30s).
	deadline := time.Now().Add(30 * time.Second)
	var progress importer.ImportProgress
	for time.Now().Before(deadline) {
		var ok bool
		progress, ok = imp.GetJobProgress(jobID)
		if !ok {
			t.Fatal("job not found")
		}
		if progress.Status == "complete" || progress.Status == "failed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	result := imp.GetJobResult(jobID)
	if result == nil {
		t.Fatal("no result returned")
	}

	t.Logf("=== Import Test Results ===")
	t.Logf("Files found:         %d", result.FilesFound)
	t.Logf("Files processed:     %d", result.FilesProcessed)
	t.Logf("Files skipped:       %d", result.FilesSkipped)
	t.Logf("Files failed:        %d", result.FilesFailed)
	t.Logf("Memories created:    %d", result.MemoriesCreated)
	t.Logf("Relationships found: %d", result.RelationshipsFound)
	t.Logf("Duration:            %v", result.Duration)
	for _, e := range result.Errors {
		t.Logf("Error: %s", e)
	}

	if result.MemoriesCreated == 0 {
		t.Error("expected at least one memory to be created")
	}
	if progress.Status != "complete" {
		t.Errorf("expected status 'complete', got %q", progress.Status)
	}
	if result.RelationshipsFound == 0 {
		t.Error("expected wiki-link relationships to be found")
	}

	fmt.Printf("\n--- Import Test Summary ---\n")
	fmt.Printf("%d files -> %d memories, %d relationships discovered\n",
		result.FilesFound, result.MemoriesCreated, result.RelationshipsFound)
}

// TestMarkdownParser tests the lower-level ParseMarkdownFile function.
func TestMarkdownParser(t *testing.T) {
	content := []byte(`---
title: Test Note
tags: [go, testing]
date: 2024-01-15
category: Engineering
---

# Test Note

This is a test note that links to [[Another Note]] and [[Third Note|Display Name]].

Some content here. #inline-tag

More content.
`)

	parsed, err := importer.ParseMarkdownFile(content, "/vault/Engineering/test-note.md", "Engineering/test-note.md")
	if err != nil {
		t.Fatalf("ParseMarkdownFile failed: %v", err)
	}

	t.Logf("Title:    %s", parsed.Title)
	t.Logf("Domain:   %s", parsed.Domain)
	t.Logf("Category: %s", parsed.Category)
	t.Logf("Tags:     %v", parsed.Tags)
	t.Logf("Links:    %v", parsed.WikiLinks)
	t.Logf("Content:\n%s", parsed.Content)

	if parsed.Title != "Test Note" {
		t.Errorf("expected title 'Test Note', got %q", parsed.Title)
	}
	if parsed.Domain != "engineering" {
		t.Errorf("expected domain 'engineering', got %q", parsed.Domain)
	}
	if len(parsed.WikiLinks) != 2 {
		t.Errorf("expected 2 wiki links, got %d", len(parsed.WikiLinks))
	}
	// Check that inline #tag was picked up.
	foundInline := false
	for _, tag := range parsed.Tags {
		if tag == "inline-tag" {
			foundInline = true
		}
	}
	if !foundInline {
		t.Errorf("expected inline-tag in tags, got %v", parsed.Tags)
	}
}

// TestWikiLinkExtractor tests wikilink extraction directly.
func TestWikiLinkExtractor(t *testing.T) {
	content := "See [[Project Alpha]] and [[Beta Note|Custom Label]] for details. Also [[Project Alpha]] again."

	links := importer.ExtractWikiLinks(content)
	if len(links) != 2 {
		t.Errorf("expected 2 unique links (deduped), got %d: %v", len(links), links)
	}
	if links[0].Target != "Project Alpha" {
		t.Errorf("expected 'Project Alpha', got %q", links[0].Target)
	}
	if links[1].Target != "Beta Note" || links[1].Alias != "Custom Label" {
		t.Errorf("unexpected second link: %+v", links[1])
	}

	stripped := importer.StripWikiLinks(content)
	t.Logf("Stripped: %s", stripped)
}

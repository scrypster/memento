package importer

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scrypster/memento/internal/storage"
	"github.com/scrypster/memento/pkg/types"
)

// ImportResult is the final summary produced by a completed import job.
type ImportResult struct {
	JobID                string        `json:"job_id"`
	FilesFound           int           `json:"files_found"`
	FilesProcessed       int           `json:"files_processed"`
	FilesSkipped         int           `json:"files_skipped"`
	FilesFailed          int           `json:"files_failed"`
	MemoriesCreated      int           `json:"memories_created"`
	RelationshipsFound   int           `json:"relationships_found"`
	Errors               []string      `json:"errors,omitempty"`
	Duration             time.Duration `json:"duration_ms"`
}

// ImportProgress carries live progress data for a running job.
type ImportProgress struct {
	JobID          string `json:"job_id"`
	Status         string `json:"status"` // "running" | "complete" | "failed"
	FilesFound     int    `json:"files_found"`
	FilesProcessed int    `json:"files_processed"`
	FilesTotal     int    `json:"files_total"`
	CurrentFile    string `json:"current_file,omitempty"`
	Message        string `json:"message,omitempty"`
}

// ImportJob tracks the state of an async import operation.
type ImportJob struct {
	mu       sync.RWMutex
	Progress ImportProgress
	Result   *ImportResult
	Done     chan struct{}
}

// newImportJob initialises a new ImportJob with the given job ID.
func newImportJob(jobID string) *ImportJob {
	return &ImportJob{
		Progress: ImportProgress{
			JobID:  jobID,
			Status: "running",
		},
		Done: make(chan struct{}),
	}
}

// GetProgress returns a snapshot of the current import progress.
func (j *ImportJob) GetProgress() ImportProgress {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.Progress
}

// ObsidianImporter walks an Obsidian vault (or any Markdown directory) and
// creates Memento memories from the notes it finds.
type ObsidianImporter struct {
	store storage.MemoryStore

	// mu protects jobs.
	mu   sync.RWMutex
	jobs map[string]*ImportJob
}

// NewObsidianImporter creates an importer that stores memories in the given store.
func NewObsidianImporter(store storage.MemoryStore) *ObsidianImporter {
	return &ObsidianImporter{
		store: store,
		jobs:  make(map[string]*ImportJob),
	}
}

// StartImport begins an asynchronous import of the directory at dirPath.
// It returns a job ID that callers can use with GetJobProgress / GetJobResult.
func (imp *ObsidianImporter) StartImport(ctx context.Context, dirPath string) (string, error) {
	// Validate the path exists and is a directory.
	info, err := os.Stat(dirPath)
	if err != nil {
		return "", fmt.Errorf("cannot access directory %q: %w", dirPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", dirPath)
	}

	jobID := uuid.New().String()
	job := newImportJob(jobID)

	imp.mu.Lock()
	imp.jobs[jobID] = job
	imp.mu.Unlock()

	// Run import in background goroutine.
	go func() {
		result := imp.runImport(ctx, job, dirPath)
		job.mu.Lock()
		job.Result = result
		if len(result.Errors) > 0 && result.FilesProcessed == 0 {
			job.Progress.Status = "failed"
			job.Progress.Message = "Import failed"
		} else {
			job.Progress.Status = "complete"
			job.Progress.Message = fmt.Sprintf("Imported %d memories from %d files",
				result.MemoriesCreated, result.FilesProcessed)
		}
		job.mu.Unlock()
		close(job.Done)
	}()

	return jobID, nil
}

// GetJobProgress returns the live progress for a job, or false if unknown.
func (imp *ObsidianImporter) GetJobProgress(jobID string) (ImportProgress, bool) {
	imp.mu.RLock()
	job, ok := imp.jobs[jobID]
	imp.mu.RUnlock()
	if !ok {
		return ImportProgress{}, false
	}
	return job.GetProgress(), true
}

// GetJobResult returns the final result for a completed job.
// Returns nil if the job is still running or not found.
func (imp *ObsidianImporter) GetJobResult(jobID string) *ImportResult {
	imp.mu.RLock()
	job, ok := imp.jobs[jobID]
	imp.mu.RUnlock()
	if !ok {
		return nil
	}
	job.mu.RLock()
	defer job.mu.RUnlock()
	return job.Result
}

// runImport is the synchronous import logic executed in a goroutine.
func (imp *ObsidianImporter) runImport(ctx context.Context, job *ImportJob, dirPath string) *ImportResult {
	start := time.Now()
	result := &ImportResult{JobID: job.Progress.JobID}

	// Phase 1: Collect all Markdown files.
	files, err := collectMarkdownFiles(dirPath)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("walk error: %v", err))
		return result
	}

	result.FilesFound = len(files)
	job.mu.Lock()
	job.Progress.FilesFound = len(files)
	job.Progress.FilesTotal = len(files)
	job.mu.Unlock()

	if len(files) == 0 {
		result.Duration = time.Since(start)
		return result
	}

	// Phase 2: Parse and store each file.
	// We also build a wiki-link relationship map so we can count unique relationships.
	relationshipSet := make(map[string]bool)

	for i, absPath := range files {
		if ctx.Err() != nil {
			result.Errors = append(result.Errors, "context cancelled")
			break
		}

		// Compute path relative to vault root.
		rel, _ := filepath.Rel(dirPath, absPath)

		job.mu.Lock()
		job.Progress.FilesProcessed = i
		job.Progress.CurrentFile = rel
		job.mu.Unlock()

		// Read file.
		data, err := os.ReadFile(absPath)
		if err != nil {
			log.Printf("import: skip %s: read error: %v", rel, err)
			result.FilesSkipped++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: read error: %v", rel, err))
			continue
		}

		// Skip empty files.
		if len(strings.TrimSpace(string(data))) == 0 {
			result.FilesSkipped++
			continue
		}

		// Parse Markdown.
		parsed, err := ParseMarkdownFile(data, absPath, rel)
		if err != nil {
			log.Printf("import: skip %s: parse error: %v", rel, err)
			result.FilesFailed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: parse error: %v", rel, err))
			continue
		}

		// Count unique wiki-link relationships.
		for _, wl := range parsed.WikiLinks {
			key := fmt.Sprintf("%s->%s", rel, strings.ToLower(wl.Target))
			if !relationshipSet[key] {
				relationshipSet[key] = true
				result.RelationshipsFound++
			}
		}

		// Create memory.
		if err := imp.storeMemory(ctx, parsed); err != nil {
			log.Printf("import: failed to store %s: %v", rel, err)
			result.FilesFailed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: store error: %v", rel, err))
			continue
		}

		result.FilesProcessed++
		result.MemoriesCreated++
	}

	result.Duration = time.Since(start)
	return result
}

// storeMemory converts a ParsedFile into a types.Memory and stores it.
func (imp *ObsidianImporter) storeMemory(ctx context.Context, pf *ParsedFile) error {
	now := time.Now()
	ts := pf.Timestamp
	if ts.IsZero() {
		ts = now
	}

	// Build metadata map with import provenance.
	meta := map[string]interface{}{
		"import_source":    "obsidian",
		"import_path":      pf.RelativePath,
		"wiki_link_count":  len(pf.WikiLinks),
	}
	if pf.Category != "" {
		meta["category"] = pf.Category
	}

	// Copy wiki link targets into metadata.
	if len(pf.WikiLinks) > 0 {
		targets := make([]string, len(pf.WikiLinks))
		for i, wl := range pf.WikiLinks {
			targets[i] = wl.Target
		}
		meta["wiki_links"] = targets
	}

	// Copy frontmatter keys not already handled.
	for k, v := range pf.Frontmatter {
		switch k {
		case "tags", "date", "created", "created_at", "updated_at",
			"category", "domain", "title":
			// Already handled.
		default:
			meta[fmt.Sprintf("fm_%s", k)] = v
		}
	}

	domain := pf.Domain
	if domain == "" {
		domain = "import"
	}

	// Generate ID in the canonical mem:domain:slug format.
	slug := uuid.New().String()[:8]
	id := fmt.Sprintf("mem:%s:%s", domain, slug)

	memory := &types.Memory{
		ID:        id,
		Content:   pf.Content,
		Source:    "obsidian-import",
		Domain:    domain,
		Tags:      pf.Tags,
		Metadata:  meta,
		Timestamp: ts,
		CreatedAt: now,
		UpdatedAt: now,

		Status:             types.StatusPending,
		EntityStatus:       types.EnrichmentPending,
		RelationshipStatus: types.EnrichmentPending,
		EmbeddingStatus:    types.EnrichmentPending,
	}

	return imp.store.Store(ctx, memory)
}

// collectMarkdownFiles walks dirPath and returns all .md / .markdown files found.
// Obsidian hidden directories (e.g. .obsidian) are skipped.
func collectMarkdownFiles(dirPath string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip hidden directories (e.g. .obsidian, .git, .trash).
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".md" || ext == ".markdown" {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

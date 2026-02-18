package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/scrypster/memento/internal/importer"
	"github.com/scrypster/memento/internal/storage"
)

// ImportHandlers contains HTTP handlers for the import API.
type ImportHandlers struct {
	importer *importer.ObsidianImporter
}

// NewImportHandlers creates a new ImportHandlers backed by the given memory store.
func NewImportHandlers(store storage.MemoryStore) *ImportHandlers {
	return &ImportHandlers{
		importer: importer.NewObsidianImporter(store),
	}
}

// --- Request / Response types ---

// importByPathRequest is the JSON body for POST /api/import/obsidian or
// POST /api/import/markdown when a server-side path is supplied.
type importByPathRequest struct {
	// Path is a directory path accessible on the server's filesystem.
	Path string `json:"path"`
}

// importJobResponse is returned immediately after starting an import.
type importJobResponse struct {
	JobID   string `json:"job_id"`
	Message string `json:"message"`
}

// --- Handlers ---

// PostObsidianImport handles POST /api/import/obsidian.
// Accepts a JSON body with {"path": "/absolute/or/relative/path"}.
func (h *ImportHandlers) PostObsidianImport(w http.ResponseWriter, r *http.Request) {
	h.handleImportByPath(w, r, "obsidian")
}

// PostMarkdownImport handles POST /api/import/markdown.
// Accepts a JSON body with {"path": "/absolute/or/relative/path"}.
func (h *ImportHandlers) PostMarkdownImport(w http.ResponseWriter, r *http.Request) {
	h.handleImportByPath(w, r, "markdown")
}

// handleImportByPath is the shared implementation for both import endpoints.
func (h *ImportHandlers) handleImportByPath(w http.ResponseWriter, r *http.Request, kind string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse JSON body.
	var req importByPathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse request body", err)
		return
	}

	if strings.TrimSpace(req.Path) == "" {
		respondError(w, http.StatusBadRequest, "path is required", nil)
		return
	}

	// Resolve the path relative to the working directory when not absolute.
	dirPath := req.Path
	if !filepath.IsAbs(dirPath) {
		wd, err := os.Getwd()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "cannot determine working directory", err)
			return
		}
		dirPath = filepath.Join(wd, dirPath)
	}

	// Validate the directory exists.
	if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
		respondError(w, http.StatusBadRequest,
			fmt.Sprintf("directory not found: %s", req.Path), nil)
		return
	}

	// Start the async import job.
	jobID, err := h.importer.StartImport(r.Context(), dirPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to start import", err)
		return
	}

	respondJSON(w, http.StatusAccepted, importJobResponse{
		JobID:   jobID,
		Message: fmt.Sprintf("Import started for %s vault at %s", kind, req.Path),
	})
}

// GetImportStatus handles GET /api/import/status/{job_id}.
// Returns live progress while running, and the full result when complete.
func (h *ImportHandlers) GetImportStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("job_id")
	if jobID == "" {
		respondError(w, http.StatusBadRequest, "job_id is required", nil)
		return
	}

	progress, ok := h.importer.GetJobProgress(jobID)
	if !ok {
		respondError(w, http.StatusNotFound, "import job not found", nil)
		return
	}

	// If complete, return the full result alongside progress.
	if progress.Status == "complete" || progress.Status == "failed" {
		result := h.importer.GetJobResult(jobID)
		type statusResponse struct {
			Progress importer.ImportProgress `json:"progress"`
			Result   *importer.ImportResult  `json:"result,omitempty"`
		}
		respondJSON(w, http.StatusOK, statusResponse{
			Progress: progress,
			Result:   result,
		})
		return
	}

	respondJSON(w, http.StatusOK, progress)
}

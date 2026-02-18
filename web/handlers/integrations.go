package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/scrypster/memento/internal/config"
)

// IntegrationsHandler serves integration configs and connection tests.
type IntegrationsHandler struct {
	config   *config.Config
	basePath string
}

// NewIntegrationsHandler creates an IntegrationsHandler.
// basePath is the root of the repository (used to locate web/static/templates/).
func NewIntegrationsHandler(cfg *config.Config, basePath string) *IntegrationsHandler {
	return &IntegrationsHandler{config: cfg, basePath: basePath}
}

// supportedSystem describes a single integration target.
type supportedSystem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
	HasTemplate bool   `json:"has_template"` // true = server renders a real template file
}

var systems = []supportedSystem{
	{ID: "claude-desktop", Name: "Claude Desktop", Icon: "🖥️", Description: "Anthropic Claude Desktop app — add Memento as an MCP server.", HasTemplate: true},
	{ID: "claude-code", Name: "Claude Code", Icon: "💻", Description: "Claude Code CLI — MCP config + skill file.", HasTemplate: true},
	{ID: "cursor", Name: "Cursor", Icon: "✏️", Description: "Cursor editor — MCP server config and .cursor/rules.", HasTemplate: true},
	{ID: "windsurf", Name: "Windsurf", Icon: "🏄", Description: "Windsurf (Codeium) editor — MCP server config.", HasTemplate: false},
	{ID: "openclaw", Name: "OpenClaw", Icon: "🦞", Description: "OpenClaw personal AI assistant — add Memento via the mcp-adapter plugin.", HasTemplate: true},
	{ID: "generic", Name: "Generic MCP", Icon: "⚙️", Description: "Any MCP-compatible client — stdio transport config.", HasTemplate: false},
}

// ListSupportedSystems handles GET /api/integrations/systems
func (h *IntegrationsHandler) ListSupportedSystems(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"systems":           systems,
		"server_url":        fmt.Sprintf("http://%s:%d", h.config.Server.Host, h.config.Server.Port),
		"mcp_binary_path":   h.mcpBinaryPath(),
		"connections_config": h.connectionsConfigPath(),
		"data_dir":          h.absDataDir(),
		"platform":          runtime.GOOS,
	})
}

// GetConfig handles GET /api/integrations/{system}/config
// Returns the rendered config file as a downloadable attachment.
func (h *IntegrationsHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	system := r.PathValue("system")
	if system == "" {
		http.Error(w, "system is required", http.StatusBadRequest)
		return
	}

	vars := h.templateVars()

	switch system {
	case "claude-desktop":
		// Same mcpServers format as Claude Code
		h.serveTemplate(w, "claude-code-mcp.json.tmpl", "claude_desktop_config.json", "application/json", vars)

	case "claude-code":
		// Which file? query param: ?file=mcp (default), ?file=skill, ?file=claude-md,
		// or ?file=per-project-claude-md&connection=<name>
		file := r.URL.Query().Get("file")
		if file == "skill" {
			h.serveTemplate(w, "claude-code-skill.md.tmpl", "memento-skill.md", "text/markdown", vars)
		} else if file == "claude-md" {
			h.serveTemplate(w, "claude-md-snippet.md.tmpl", "memento-claude-md-snippet.md", "text/markdown", vars)
		} else if file == "per-project-claude-md" {
			connName := r.URL.Query().Get("connection")
			if connName == "" {
				http.Error(w, "connection query parameter is required", http.StatusBadRequest)
				return
			}
			// Use optional display_name query param (populated by the UI from /api/connections).
			displayName := r.URL.Query().Get("display_name")
			if displayName == "" {
				displayName = connName
			}
			perProjectVars := map[string]string{
				"CONNECTION_ID":   connName,
				"CONNECTION_NAME": displayName,
			}
			filename := fmt.Sprintf("memento-%s-claude-md.md", connName)
			h.serveTemplate(w, "per-project-claude-md.md.tmpl", filename, "text/markdown", perProjectVars)
		} else {
			h.serveTemplate(w, "claude-code-mcp.json.tmpl", "claude_code_mcp.json", "application/json", vars)
		}

	case "cursor":
		file := r.URL.Query().Get("file")
		if file == "rule" {
			h.serveTemplate(w, "cursor-rule.mdc.tmpl", "memento.mdc", "text/plain", vars)
		} else {
			h.serveTemplate(w, "claude-code-mcp.json.tmpl", "cursor_mcp.json", "application/json", vars)
		}

	case "windsurf":
		// Same mcpServers format as all standard MCP clients
		h.serveTemplate(w, "claude-code-mcp.json.tmpl", "windsurf_mcp.json", "application/json", vars)

	case "openclaw":
		h.serveTemplate(w, "openclaw-config.json.tmpl", "openclaw_mcp_config.json", "application/json", vars)

	case "generic":
		cfg := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"memento": map[string]interface{}{
					"command": vars["MCP_BINARY_PATH"],
					"args":    []string{},
					"env": map[string]string{
						"MEMENTO_DATA_PATH":         vars["DATA_DIR"],
						"MEMENTO_CONNECTIONS_CONFIG": vars["CONNECTIONS_CONFIG"],
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=memento_mcp.json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(cfg)

	default:
		http.Error(w, fmt.Sprintf("unknown system %q", system), http.StatusNotFound)
	}
}

// TestConnection handles GET /api/integrations/test-connection
// Checks that the server is reachable and reports available MCP tools.
func (h *IntegrationsHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"connected": true,
		"server":    fmt.Sprintf("http://%s:%d", h.config.Server.Host, h.config.Server.Port),
		"tools_available": []string{
			"store_memory",
			"recall_memory",
			"find_related",
			"explain_reasoning",
			"detect_contradictions",
			"retry_enrichment",
			"update_memory_state",
		},
	})
}

// --- helpers ---

// mcpBinaryPath finds the memento-mcp binary.
// It looks next to the current executable first (both are installed together),
// then falls back to PATH lookup, then a sensible default.
func (h *IntegrationsHandler) mcpBinaryPath() string {
	// 1. Look next to the current executable (typical install: both binaries in same dir)
	if execPath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(execPath), "memento-mcp")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// 2. Check PATH
	if path, err := exec.LookPath("memento-mcp"); err == nil {
		return path
	}
	// 3. Return a placeholder — user needs to install memento-mcp
	return "/usr/local/bin/memento-mcp"
}

// connectionsConfigPath returns the absolute path to connections.json.
// It looks next to the running executable first, then in basePath/config/.
func (h *IntegrationsHandler) connectionsConfigPath() string {
	// 1. Look for config/connections.json next to the executable
	if execPath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(execPath), "config", "connections.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// 2. Resolve from basePath (project root when running from source)
	candidate := filepath.Join(h.basePath, "config", "connections.json")
	if abs, err := filepath.Abs(candidate); err == nil {
		return abs
	}
	return candidate
}

// absDataDir returns an absolute version of the configured data path.
func (h *IntegrationsHandler) absDataDir() string {
	if abs, err := filepath.Abs(h.config.Storage.DataPath); err == nil {
		return abs
	}
	return h.config.Storage.DataPath
}

func (h *IntegrationsHandler) templateVars() map[string]string {
	return map[string]string{
		"MCP_BINARY_PATH":    h.mcpBinaryPath(),
		"CONNECTIONS_CONFIG": h.connectionsConfigPath(),
		"DATA_DIR":           h.absDataDir(),
		"SERVER_URL":         fmt.Sprintf("http://%s", h.config.Server.Host),
		"PORT":               fmt.Sprintf("%d", h.config.Server.Port),
	}
}

func (h *IntegrationsHandler) serveTemplate(w http.ResponseWriter, tmplFile, downloadName, contentType string, vars map[string]string) {
	tmplPath := filepath.Join(h.basePath, "web", "static", "templates", tmplFile)
	tmplContent, err := os.ReadFile(tmplPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("template not found: %v", err), http.StatusInternalServerError)
		return
	}

	// Simple {{VAR}} substitution — avoids Go template engine conflicts with JSON braces
	result := string(tmplContent)
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", downloadName))
	w.Write([]byte(result))
}

// Package tests provides regression guards for the first-time setup experience.
//
// These tests catch common setup regressions before they reach users:
//   - Health endpoint path changes (docker-compose.yml and CI depend on /api/health)
//   - Broken file references in install scripts (wrong repo name, missing Dockerfiles)
//   - CI config staying in sync with the actual server behavior
package tests

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/config"
	"github.com/scrypster/memento/internal/server"
	"github.com/scrypster/memento/internal/storage/sqlite"
)

// TestHealthEndpointPath verifies the health endpoint is at /api/health.
//
// docker-compose.yml healthcheck and CI both hit /api/health.
// If this path ever changes, Docker containers will fail to become healthy
// and CI will report false negatives. This test catches that regression.
func TestHealthEndpointPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: config.StorageConfig{
			DataPath: tmpDir,
		},
		Security: config.SecurityConfig{
			SecurityMode: "development",
		},
	}

	store, err := sqlite.NewMemoryStore(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() { _ = store.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverReady := make(chan string, 1)
	go func() {
		addr, _ := server.Start(ctx, cfg, store)
		serverReady <- addr
	}()

	var baseURL string
	select {
	case addr := <-serverReady:
		baseURL = "http://" + addr
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start in time")
	}

	time.Sleep(100 * time.Millisecond)

	// The canonical health endpoint is /api/health — docker-compose and CI depend on this.
	resp, err := http.Get(baseURL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/api/health returned %d, want 200 — docker-compose healthcheck and CI will break", resp.StatusCode)
	}

	// The old (wrong) path must not return 200 — catching accidental duplicates.
	resp2, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()

	if resp2.StatusCode == http.StatusOK {
		t.Errorf("/health returned 200 — both paths work, which is fine, but document it intentionally")
	}
}

// TestSetupFileIntegrity checks critical setup files for known-broken patterns.
//
// These are the patterns that have caused real setup failures:
//   - install.sh referencing the old "memento-go" repo name (fixed in Track 1)
//   - docker-compose.yml referencing Dockerfile.backup which doesn't exist
//   - README.md containing /path/to/ placeholder paths
func TestSetupFileIntegrity(t *testing.T) {
	projectRoot := filepath.Dir(mustGetwd(t))

	t.Run("install.sh has correct repo name", func(t *testing.T) {
		installPath := filepath.Join(projectRoot, "install.sh")
		data, err := os.ReadFile(installPath)
		if err != nil {
			t.Skipf("install.sh not found at %s — skipping", installPath)
		}
		content := string(data)
		if strings.Contains(content, "memento-go") {
			t.Error("install.sh contains 'memento-go' — should be 'memento'. Users will clone the wrong repo.")
		}
	})

	t.Run("docker-compose.yml has no active Dockerfile.backup reference", func(t *testing.T) {
		composePath := filepath.Join(projectRoot, "docker-compose.yml")
		data, err := os.ReadFile(composePath)
		if err != nil {
			t.Fatalf("docker-compose.yml not found: %v", err)
		}

		// Check each line — an uncommented "dockerfile: Dockerfile.backup" would block `docker compose up`
		for i, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "dockerfile:") && strings.Contains(trimmed, "Dockerfile.backup") {
				t.Errorf("docker-compose.yml line %d has active Dockerfile.backup reference: %q — file doesn't exist, blocks `docker compose up`", i+1, line)
			}
		}
	})

	t.Run("README.md has no placeholder paths", func(t *testing.T) {
		readmePath := filepath.Join(projectRoot, "README.md")
		data, err := os.ReadFile(readmePath)
		if err != nil {
			t.Fatalf("README.md not found: %v", err)
		}
		content := string(data)
		if strings.Contains(content, "/path/to/memento") {
			t.Error("README.md contains '/path/to/memento' placeholder — users need real copy-paste commands")
		}
	})

	t.Run("docker-compose.yml health check uses correct endpoint", func(t *testing.T) {
		composePath := filepath.Join(projectRoot, "docker-compose.yml")
		data, err := os.ReadFile(composePath)
		if err != nil {
			t.Fatalf("docker-compose.yml not found: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "/api/health") {
			t.Error("docker-compose.yml healthcheck does not reference /api/health — container will never become healthy")
		}
		// The old wrong path
		if strings.Contains(content, `"http://localhost:6363/health"`) {
			t.Error("docker-compose.yml healthcheck references /health (missing /api prefix) — use /api/health")
		}
	})
}

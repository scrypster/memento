# Setup Hardening Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix five critical setup bugs (Track 1) then overhaul launch.sh and README (Track 2) so first-time setup is bulletproof for both Docker and local users.

**Architecture:** Track 1 tasks are fully independent — dispatch all five to haiku agents in parallel. Track 2 is sequential and runs after Track 1 is merged.

**Tech Stack:** bash, Go (config.go), Markdown (README.md)

---

## TRACK 1 — Critical Bug Fixes (Parallel, delegate to haiku)

---

### Task 1.1: Fix install.sh repo name

**Files:**
- Modify: `install.sh`

**Context:**
`install.sh` references `scrypster/memento-go` in three places (line 5 comment, line 7 REPO variable, lines 47-49 output). The actual repo is `scrypster/memento`. This causes a GitHub 404 for anyone following the install script.

**Step 1: Open the file and find all occurrences**

Run:
```bash
grep -n "memento-go" install.sh
```
Expected output: 3-4 lines containing `memento-go`

**Step 2: Fix all occurrences**

In `install.sh`:
- Line 5: Change comment URL from `...scrypster/memento-go/main/install.sh` to `...scrypster/memento/main/install.sh`
- Line 7: Change `REPO="scrypster/memento-go"` to `REPO="scrypster/memento"`
- Lines 47-49: The `cd memento-go` reference becomes `cd memento`

**Step 3: Verify no old name remains**

Run:
```bash
grep "memento-go" install.sh
```
Expected: no output

**Step 4: Commit**

```bash
git add install.sh
git commit -m "Fix install.sh repo name: memento-go -> memento"
```

---

### Task 1.2: Fix connections.json hardcoded path

**Files:**
- Modify: `scripts/docker-entrypoint.sh`

**Context:**
The entrypoint writes a `connections.json` with `"path": "/data/memento.db"` hardcoded regardless of `MEMENTO_DATA_PATH`. If a user sets a custom data path, their database is silently ignored.

**Step 1: Find the heredoc that writes connections.json**

Run:
```bash
grep -n "memento.db\|JSONEOF\|connections" scripts/docker-entrypoint.sh
```
Expected: lines showing the cat heredoc writing `/data/memento.db`

**Step 2: Replace hardcoded path with variable expansion**

Find this block in `scripts/docker-entrypoint.sh`:
```sh
cat > "$CONFIG_FILE" << 'JSONEOF'
{
  ...
      "database": {
        "type": "sqlite",
        "path": "/data/memento.db"
      },
```

The problem is the single-quoted `'JSONEOF'` — single quotes prevent variable expansion. Change it to use a subshell-expanded path:

Replace the entire heredoc section with:
```sh
DATA_PATH="${MEMENTO_DATA_PATH:-/data}"
cat > "$CONFIG_FILE" << JSONEOF
{
  "default_connection": "default",
  "connections": [
    {
      "name": "default",
      "display_name": "Default",
      "description": "Default memory workspace",
      "enabled": true,
      "database": {
        "type": "sqlite",
        "path": "${DATA_PATH}/memento.db"
      },
      "llm": {
        "provider": "ollama",
        "model": "qwen2.5:7b"
      }
    }
  ],
  "settings": {
    "auto_create_default": true,
    "max_connections": 10,
    "allow_user_create": true
  }
}
JSONEOF
```

Note: Remove the single quotes from `'JSONEOF'` so shell variables expand inside the heredoc.

**Step 3: Verify expansion works**

Run locally:
```bash
MEMENTO_DATA_PATH=/custom/path bash -c '
DATA_PATH="${MEMENTO_DATA_PATH:-/data}"
cat << JSONEOF
{"path": "${DATA_PATH}/memento.db"}
JSONEOF'
```
Expected output: `{"path": "/custom/path/memento.db"}`

**Step 4: Commit**

```bash
git add scripts/docker-entrypoint.sh
git commit -m "Fix connections.json to use MEMENTO_DATA_PATH instead of hardcoded /data"
```

---

### Task 1.3: Ollama silent failure → loud exit

**Files:**
- Modify: `launch.sh`

**Context:**
In the local setup path, when Ollama is not running the script prints a warning but continues. Users build everything successfully then hit cryptic enrichment failures at runtime. The fix: exit immediately with specific install/start commands.

**Step 1: Find the Ollama check in launch.sh**

Run:
```bash
grep -n "ollama\|OLLAMA\|Ollama" launch.sh | head -30
```
Identify the section that checks if Ollama is running and currently warns-but-continues.

**Step 2: Find the local path Ollama failure branch**

Look for a pattern like:
```bash
if ! curl -sf ...; then
    echo "WARNING: Ollama..."
    # currently continues
fi
```

**Step 3: Replace warn-and-continue with loud exit**

Replace the warning block with:
```bash
if ! curl -sf "${OLLAMA_URL}/api/tags" > /dev/null 2>&1; then
    echo ""
    echo -e "${RED}✗ Ollama is not running.${NC}"
    echo ""
    echo "  Memento requires Ollama for entity extraction and embeddings."
    echo "  Install and start Ollama, then re-run this script:"
    echo ""
    echo -e "${YELLOW}  macOS:${NC}"
    echo "    brew install ollama"
    echo "    ollama serve"
    echo ""
    echo -e "${YELLOW}  Linux:${NC}"
    echo "    curl -fsSL https://ollama.ai/install.sh | sh"
    echo "    ollama serve"
    echo ""
    echo "  Or run Ollama as a background service:"
    echo "    ollama serve &"
    echo ""
    exit 1
fi
```

**Step 4: Verify script exits on Ollama failure**

With Ollama stopped, run:
```bash
bash -n launch.sh  # syntax check first
```
Then test the Ollama detection block manually by temporarily setting `OLLAMA_URL` to an unreachable address.

**Step 5: Commit**

```bash
git add launch.sh
git commit -m "Fail loud when Ollama is not running in local setup path"
```

---

### Task 1.4: Development mode security warning

**Files:**
- Modify: `scripts/docker-entrypoint.sh`
- Modify: `launch.sh`

**Context:**
Default security mode is `development` with no authentication. Users who expose port 6363 have a completely open instance. A visible warning on startup prevents accidental exposure.

**Step 1: Add warning to docker-entrypoint.sh**

Find the `echo "Starting Memento..."` line near the bottom of `scripts/docker-entrypoint.sh`.

Insert this block immediately before it:
```sh
# Warn if running without authentication
SECURITY_MODE="${MEMENTO_SECURITY_MODE:-development}"
API_TOKEN="${MEMENTO_API_TOKEN:-}"
if [ "$SECURITY_MODE" = "development" ] && [ -z "$API_TOKEN" ]; then
    echo ""
    echo "⚠  WARNING: Running in development mode — no authentication required."
    echo "   Anyone with access to port ${MEMENTO_PORT:-6363} can read and write your memories."
    echo "   To enable auth: set MEMENTO_API_TOKEN in your environment or .env file."
    echo ""
fi
```

**Step 2: Add warning to launch.sh**

In `launch.sh`, find where the server start command or final success message is printed (near the end of both the Docker and local paths).

Add the same warning block before the final "open http://localhost:6363" output, checking if `MEMENTO_API_TOKEN` is set in the environment or .env file.

**Step 3: Verify warning appears and disappears correctly**

Run:
```bash
# Should show warning
MEMENTO_SECURITY_MODE=development MEMENTO_API_TOKEN="" bash scripts/docker-entrypoint.sh 2>&1 | grep -A4 "WARNING"

# Should NOT show warning
MEMENTO_API_TOKEN="mytoken" bash scripts/docker-entrypoint.sh 2>&1 | grep "WARNING" || echo "no warning (correct)"
```

**Step 4: Commit**

```bash
git add scripts/docker-entrypoint.sh launch.sh
git commit -m "Warn on startup when running in unauthenticated development mode"
```

---

### Task 1.5: Fix README MCP path placeholder

**Files:**
- Modify: `README.md`

**Context:**
README line 91 shows `claude mcp add memento -- /path/to/memento-mcp` — a literal placeholder that breaks if copied. Users need to know: (1) `launch.sh` prints the real command at the end, (2) the path must be absolute.

**Step 1: Find the placeholder**

Run:
```bash
grep -n "path/to" README.md
```
Expected: line 91 in the integrations table.

**Step 2: Replace the placeholder row**

Find:
```markdown
| **Claude Code** | `claude mcp add memento -- /path/to/memento-mcp -e MEMENTO_DATA_PATH=~/.memento` |
```

Replace with:
```markdown
| **Claude Code** | Run `./launch.sh` — it prints the exact command at the end. Pattern: `` claude mcp add memento -- `pwd`/memento-mcp `` |
```

**Step 3: Verify no placeholders remain**

Run:
```bash
grep "path/to" README.md
```
Expected: no output (or only in non-user-facing sections like project structure)

**Step 4: Commit**

```bash
git add README.md
git commit -m "Replace /path/to/memento-mcp placeholder with launch.sh reference"
```

---

## TRACK 2 — launch.sh + README Overhaul (Sequential, after Track 1)

---

### Task 2.1: Harden launch.sh preflight checks

**Files:**
- Modify: `launch.sh`

**Context:**
Current preflight: passes with 3/5 checks, Node.js not checked, disk space check uses platform-specific flags, Ollama models not verified. Goal: all critical checks must pass, Node.js in preflight, cross-platform disk check, model verification.

**Step 1: Read the current preflight section**

```bash
grep -n "CHECKS_PASSED\|check_\|preflight\|node\|npm" launch.sh
```

**Step 2: Add Node.js to preflight**

Find the Go version check block. Immediately after it, add:
```bash
# Check Node.js
if command -v node &>/dev/null; then
    NODE_VERSION=$(node --version)
    echo -e "${GREEN}✓ Node.js $NODE_VERSION${NC}"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    echo -e "${RED}✗ Node.js not found${NC}"
    echo "  Required for building CSS assets."
    echo "  Install: https://nodejs.org  or  brew install node"
    CHECKS_FAILED=$((CHECKS_FAILED + 1))
fi
```

**Step 3: Change preflight threshold to require all checks**

Find:
```bash
if [ "$CHECKS_PASSED" -lt 3 ]; then
```

Replace with:
```bash
if [ "$CHECKS_FAILED" -gt 0 ]; then
    echo ""
    echo -e "${RED}✗ Preflight failed. Fix the issues above and re-run.${NC}"
    exit 1
fi
```

(Make sure `CHECKS_FAILED` is initialized to 0 at the top of preflight.)

**Step 4: Fix disk space check to be cross-platform**

Find the disk space block using `df -g` / `df -BG`. Replace with:
```bash
AVAILABLE_KB=$(df -k . | awk 'NR==2 {print $4}')
AVAILABLE_GB=$((AVAILABLE_KB / 1024 / 1024))
if [ "$AVAILABLE_GB" -lt 8 ]; then
    echo -e "${YELLOW}⚠ Low disk space: ~${AVAILABLE_GB}GB available, 8GB recommended for Ollama models${NC}"
fi
```

**Step 5: Add Ollama model check (local path only)**

After confirming Ollama is running in the local path, add:
```bash
EXTRACTION_MODEL="${MEMENTO_OLLAMA_MODEL:-qwen2.5:7b}"
EMBEDDING_MODEL="${MEMENTO_EMBEDDING_MODEL:-nomic-embed-text}"

for MODEL in "$EXTRACTION_MODEL" "$EMBEDDING_MODEL"; do
    if ! curl -sf "${OLLAMA_URL}/api/tags" | grep -q "\"${MODEL}\""; then
        echo -e "${YELLOW}⚠ Model not found: ${MODEL}${NC}"
        echo -n "  Pull it now? [Y/n] "
        read -r PULL_RESPONSE
        if [ "${PULL_RESPONSE:-Y}" = "Y" ] || [ "${PULL_RESPONSE:-Y}" = "y" ]; then
            echo "  Pulling ${MODEL} (this may take several minutes)..."
            ollama pull "$MODEL"
        else
            echo "  Run: ollama pull $MODEL"
            echo "  Then re-run this script."
            exit 1
        fi
    else
        echo -e "${GREEN}✓ Model ready: ${MODEL}${NC}"
    fi
done
```

**Step 6: Run the script locally (dry-run syntax check)**

```bash
bash -n launch.sh
```
Expected: no errors

**Step 7: Commit**

```bash
git add launch.sh
git commit -m "Harden launch.sh preflight: Node.js check, all-must-pass, cross-platform disk, model verification"
```

---

### Task 2.2: Add copy-paste MCP command to launch.sh success output

**Files:**
- Modify: `launch.sh`

**Context:**
Both Docker and local paths should end with a green success box that includes the exact `claude mcp add` command with real absolute paths. Currently the local path has a partial version; Docker path has none.

**Step 1: Find the current end-of-setup output**

```bash
grep -n "claude mcp\|mcp add\|Next steps\|success" launch.sh | tail -20
```

**Step 2: Update local path success output**

Find the local path success message. Replace or augment with:
```bash
BINARY_PATH="$(pwd)/memento-mcp"
DATA_PATH="$(pwd)/data"
echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║  ✓ Memento is ready!                                     ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "  Web UI:  http://localhost:6363"
echo ""
echo "  Connect Claude Code — copy and run this command:"
echo ""
echo -e "${YELLOW}  claude mcp add memento -- ${BINARY_PATH} -e MEMENTO_DATA_PATH=${DATA_PATH}${NC}"
echo ""
echo "  Then ask Claude: \"Remember that we're using PostgreSQL for the main database.\""
echo "  Then ask:        \"What database are we using?\""
echo ""
```

**Step 3: Add equivalent output to Docker path success**

Find where the Docker path confirms it's running. Add:
```bash
echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║  ✓ Memento is running!                                   ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "  Web UI:  http://localhost:6363"
echo ""
echo "  Connect Claude Code — the web UI generates the exact command:"
echo "  → Open http://localhost:6363/integrations"
echo ""
echo "  Or run manually:"
echo -e "${YELLOW}  claude mcp add memento -- /path/to/memento-mcp -e MEMENTO_DATABASE_URL=http://localhost:6363${NC}"
echo ""
echo "  First memory to try:"
echo "  \"Remember that we're using PostgreSQL — chose it for pgvector support.\""
echo ""
```

**Step 4: Commit**

```bash
git add launch.sh
git commit -m "Add copy-paste MCP command and first-memory example to setup success output"
```

---

### Task 2.3: Rewrite README Quick Start and add prerequisites

**Files:**
- Modify: `README.md`

**Context:**
Quick Start is too long and buries the point. Prerequisites don't list Node.js. No "first thing to try" after setup. Goal: 60-second checklist, correct prereqs, immediate value example.

**Step 1: Update prerequisites / Quick Start section**

Replace the current Quick Start block (lines ~23-51) with:

```markdown
## Quick Start

**Prerequisites:** Git, [Docker](https://docker.com) OR [Go 1.23+](https://go.dev) + [Node.js 18+](https://nodejs.org) + [Ollama](https://ollama.ai)

```bash
git clone https://github.com/scrypster/memento.git
cd memento
./launch.sh
```

The script detects your environment (Docker or local), runs preflight checks, and prints the exact command to connect your AI tool at the end. First run downloads Ollama models (~5 GB).

### Your first memory

Once connected, try this in Claude:

```
"We're using PostgreSQL — chose it for pgvector support."
```

Close the tab. Open a new session. Ask:

```
"What database are we using?"
```

Your AI already knows.
```

**Step 2: Remove the Manual Docker and Manual Go binary sections that duplicate Quick Start**

These add length without helping new users. Move them under a collapsible "Manual setup" heading or to a `docs/` file.

**Step 3: Verify no `/path/to/` placeholders remain**

```bash
grep -n "path/to" README.md
```
Expected: no output in user-facing sections

**Step 4: Commit**

```bash
git add README.md
git commit -m "Rewrite README Quick Start: 60-second checklist, correct prereqs, first-memory example"
```

---

## Execution Order

**Track 1** — dispatch all 5 tasks to haiku agents in parallel:
- Task 1.1, 1.2, 1.3, 1.4, 1.5 run simultaneously

**Track 2** — run sequentially after Track 1 is merged:
- Task 2.1 → 2.2 → 2.3

## Success Criteria

- `grep "memento-go" install.sh` → no output
- `MEMENTO_DATA_PATH=/custom docker run ... && cat /app/config/connections.json | grep path` → `/custom/memento.db`
- `launch.sh` with Ollama stopped → exits non-zero with install instructions
- `grep "path/to" README.md` → no output in user-facing sections
- New user with Docker can complete setup in < 5 minutes
- New user with local Go/Node/Ollama can complete setup in < 10 minutes

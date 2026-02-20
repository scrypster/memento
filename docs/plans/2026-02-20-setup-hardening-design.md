# Setup Hardening Design
**Date:** 2026-02-20
**Status:** Approved

## Problem

First-time setup has multiple friction points that will cause users to abandon Memento before they experience its value. Key failures: broken install script, silent Ollama failures, hardcoded database paths, placeholder MCP commands in README, and a rambling Quick Start that buries the actual next step.

## Approach

Two parallel tracks. Track 1 ships first (critical bug fixes, parallelizable). Track 2 follows (launch.sh and README overhaul). Web UI setup wizard is out of scope for this plan.

---

## Track 1 — Critical Bug Fixes

Five independent fixes. Each targets a single file. All can run in parallel.

### 1.1 — `install.sh` repo name
- **File:** `install.sh`
- **Fix:** Replace all occurrences of `scrypster/memento-go` with `scrypster/memento`
- **Success:** No `/memento-go` strings remain in the file

### 1.2 — `connections.json` path resolution
- **File:** `scripts/docker-entrypoint.sh`
- **Fix:** Replace hardcoded `"path": "/data/memento.db"` with shell variable expansion: `"path": "${MEMENTO_DATA_PATH:-/data}/memento.db"`. Write the JSON via a shell heredoc that expands variables rather than a literal string.
- **Success:** When `MEMENTO_DATA_PATH=/custom`, the written connections.json contains `/custom/memento.db`

### 1.3 — Ollama silent failure → loud failure
- **File:** `launch.sh` (local path only)
- **Fix:** After Ollama readiness check fails, print a red error box with exact fix commands, then `exit 1`. Do not continue. Docker path is unaffected.
- **Error message format:**
  ```
  ✗ Ollama is not running.
    Fix: brew install ollama && ollama serve   (macOS)
         curl -fsSL https://ollama.ai/install.sh | sh && ollama serve   (Linux)
  ```
- **Success:** Script exits non-zero when Ollama is unreachable on the local path

### 1.4 — Development mode security warning
- **Files:** `scripts/docker-entrypoint.sh`, `launch.sh`
- **Fix:** When `MEMENTO_SECURITY_MODE` is `development` or unset AND no API token is configured, print a yellow warning block:
  ```
  ⚠  WARNING: Running in development mode — no authentication required.
     Anyone with access to port 6363 can read and write your memories.
     Set MEMENTO_API_TOKEN in your .env to enable authentication.
  ```
- **Success:** Warning appears in both Docker and local starts when no token is set. Does not appear when token is set.

### 1.5 — MCP path placeholder in README
- **File:** `README.md`
- **Fix:** Remove the literal `/path/to/memento-mcp` placeholder. Replace with:
  1. A note that `./launch.sh` prints the exact copy-paste command at the end
  2. A pattern showing the correct form: `` claude mcp add memento -- `pwd`/memento-mcp ``
- **Success:** No literal `/path/to/` strings remain in the README's setup instructions

---

## Track 2 — launch.sh + README Overhaul

Sequential. Begins after Track 1 is merged.

### launch.sh Changes

**Preflight hardening:**
- Move Node.js check into preflight alongside Go and Ollama. Print fix commands on failure.
- All preflight checks must pass — no "3 out of 5" threshold. Any failure exits.
- After confirming Ollama is running, verify required models (`qwen2.5:7b`, `nomic-embed-text`) are pulled. Offer to pull them interactively or exit with pull commands.
- Disk space check: use `df -k` (cross-platform), convert in awk. Remove `df -g` / `df -BG` platform branching.

**End-of-setup output:**
- Both Docker and local paths end with a green success box containing:
  - The exact `claude mcp add` command with the real absolute binary path
  - The web UI URL
  - One example of what to type in Claude first

### README Changes

**Quick Start rewrite:**
- Single command: `./launch.sh`
- Single outcome: copy the MCP command it prints
- Done. No setup details in Quick Start — those belong in the script output.

**"Your first memory" section** (immediately after setup):
```
You: "We're using PostgreSQL — chose it for pgvector support."
→ stored. Now try: "What database are we using?"
→ Your AI already knows.
```

**Prerequisites section:**
- Add Node.js alongside Go and Docker (currently missing)
- Add disk space requirement (8GB for Docker with Ollama models)

**No placeholder paths:**
- All MCP setup examples reference `./launch.sh` output, not hand-edited paths

---

## Success Criteria

- A user with Docker installed can run `./launch.sh`, choose Docker, and have a working MCP connection in under 5 minutes
- A user with Go + Node.js + Ollama installed can run `./launch.sh`, choose local, and have a working MCP connection in under 10 minutes
- Zero placeholder paths (`/path/to/`) in any user-facing documentation
- Ollama failure is never silent — always exits with fix instructions
- Security warning is visible on every start without a token

## Out of Scope

- Web UI setup wizard (separate plan)
- PostgreSQL team setup improvements
- Windows support improvements

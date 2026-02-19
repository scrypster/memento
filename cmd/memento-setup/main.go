package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/scrypster/memento/internal/connections"
)

func main() {
	// Check for --verify flag
	for _, arg := range os.Args[1:] {
		if arg == "--verify" {
			runVerify()
			return
		}
	}

	printBanner()

	fmt.Println("Welcome to Memento Setup!")
	fmt.Println("Memento gives AI assistants persistent memory across sessions.")
	fmt.Println()

	runMode := prompt("How would you like to run Memento?", []string{
		"Docker (recommended -- zero dependencies)",
		"Local (requires Go, Ollama)",
	})

	switch runMode {
	case "1":
		runDockerSetup()
	case "2":
		runLocalSetup()
	}
}

func printBanner() {
	fmt.Print(`
 __  __                           _
|  \/  | ___ _ __ ___   ___ _ __ | |_ ___
| |\/| |/ _ \ '_ ` + "`" + ` _ \ / _ \ '_ \| __/ _ \
| |  | |  __/ | | | | |  __/ | | | || (_) |
|_|  |_|\___|_| |_| |_|\___|_| |_|\__\___/

Persistent Memory for AI Assistants
`)
}

// runVerify performs a health check of the Memento installation.
func runVerify() {
	fmt.Println("Memento Setup Verification")
	fmt.Println("==========================")
	fmt.Println()

	statusOK := true

	// Check MCP binary
	mcpBinary := ""
	mcpFound := false
	mcpCheckPath := []string{
		"./memento-mcp",
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "memento-mcp"),
	}

	// Also check in the same directory as the current executable
	if execPath, err := os.Executable(); err == nil {
		mcpCheckPath = append(mcpCheckPath, filepath.Join(filepath.Dir(execPath), "memento-mcp"))
	}

	// Check project directory
	if projectDir, err := os.Getwd(); err == nil {
		mcpCheckPath = append(mcpCheckPath, filepath.Join(projectDir, "memento-mcp"))
	}

	for _, path := range mcpCheckPath {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			// Check if executable
			if info.Mode()&0111 != 0 {
				mcpBinary = path
				mcpFound = true
				break
			}
		}
	}

	if mcpFound {
		fmt.Printf("MCP binary:   ✓ Found at %s\n", mcpBinary)
	} else {
		fmt.Println("MCP binary:   ✗ Not found (run memento-setup to install)")
		statusOK = false
	}

	// Check data directory
	dataDir := os.Getenv("MEMENTO_DATA_PATH")
	if dataDir == "" {
		dataDir = filepath.Join(os.Getenv("HOME"), ".memento")
	}

	dataDirExists := false
	dataDirWritable := false

	if info, err := os.Stat(dataDir); err == nil && info.IsDir() {
		dataDirExists = true

		// Check if writable
		testFile := filepath.Join(dataDir, ".memento-write-test")
		if err := os.WriteFile(testFile, []byte("test"), 0644); err == nil {
			os.Remove(testFile)
			dataDirWritable = true
		}
	}

	if dataDirExists {
		if dataDirWritable {
			fmt.Printf("Data path:    ✓ %s (writable)\n", dataDir)
		} else {
			fmt.Printf("Data path:    ✗ %s (not writable)\n", dataDir)
			statusOK = false
		}
	} else {
		fmt.Printf("Data path:    ✗ %s (does not exist)\n", dataDir)
		statusOK = false
	}

	// Print status
	fmt.Println()
	if statusOK {
		fmt.Println("Status:       READY")
		fmt.Println()
		fmt.Println("Connect to Claude Code:")
		fmt.Printf("  claude mcp add memento %s --scope user\n", mcpBinary)
		os.Exit(0)
	} else {
		fmt.Println("Status:       NOT READY")
		fmt.Println()
		fmt.Println("Run memento-setup to install missing components.")
		os.Exit(1)
	}
}

// prompt shows a numbered menu and returns the selected number as string.
func prompt(question string, options []string) string {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("\n%s\n", question)
		for i, opt := range options {
			fmt.Printf("  [%d] %s\n", i+1, opt)
		}
		fmt.Print("\nEnter choice: ")
		scanner.Scan()
		choice := strings.TrimSpace(scanner.Text())
		for i := range options {
			if choice == fmt.Sprintf("%d", i+1) {
				return choice
			}
		}
		fmt.Printf("Please enter a number between 1 and %d\n", len(options))
	}
}

// ask asks a free-text question with an optional default.
func ask(question, defaultVal string) string {
	scanner := bufio.NewScanner(os.Stdin)
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", question, defaultVal)
	} else {
		fmt.Printf("%s: ", question)
	}
	scanner.Scan()
	val := strings.TrimSpace(scanner.Text())
	if val == "" {
		return defaultVal
	}
	return val
}

func runDockerSetup() {
	fmt.Println("\nDocker Setup")
	fmt.Println("-------------------------------------")

	// Check Docker is installed
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Println("ERROR: Docker not found. Please install Docker Desktop from https://docker.com/desktop")
		os.Exit(1)
	}
	fmt.Println("OK: Docker found")

	// Get user preferences
	userName := ask("Your name (used to personalize memory)", "")
	connectionName := ask("Workspace name", "default")
	displayName := ask("Workspace display name", "My Workspace")

	// Find project directory (CWD)
	projectDir, _ := os.Getwd()

	// Create config and data directories
	configDir := filepath.Join(projectDir, "config")
	os.MkdirAll(configDir, 0755)
	dataDir := filepath.Join(projectDir, "data")
	os.MkdirAll(dataDir, 0755)

	// Build connections.json
	conn := connections.Connection{
		Name:        connectionName,
		DisplayName: displayName,
		Description: fmt.Sprintf("Memory workspace for %s", userName),
		Enabled:     true,
		CreatedAt:   time.Now().Format(time.RFC3339),
		Database:    connections.DatabaseConfig{Type: "sqlite", Path: "/data/memento.db"},
		LLM:         connections.LLMConfig{Provider: "ollama", Model: "qwen2.5:7b"},
	}
	cfg := connections.ConnectionsConfig{
		DefaultConnection: connectionName,
		Connections:       []connections.Connection{conn},
	}
	cfg.Settings.AutoCreateDefault = true
	cfg.Settings.MaxConnections = 10
	cfg.Settings.AllowUserCreate = true

	writeConnectionsConfig(filepath.Join(configDir, "connections.json"), cfg)

	// Print docker compose instructions
	fmt.Printf(`
Configuration created!

Next steps:
  1. Start Memento:
     docker compose up -d

  2. Open the dashboard:
     http://localhost:6363

  3. Connect to Claude Code:
     After containers start, run the integration command shown at:
     http://localhost:6363/integrations

The first startup pulls Ollama models (qwen2.5:7b + nomic-embed-text).
This may take a few minutes depending on your internet connection.
`)
}

func runLocalSetup() {
	fmt.Println("\nLocal Setup")
	fmt.Println("-------------------------------------")

	// Check prerequisites
	fmt.Println("\nChecking prerequisites...")

	// Check Go
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Println("ERROR: Go not found. Install from https://go.dev/dl/")
		os.Exit(1)
	}
	fmt.Println("OK: Go found")

	// Check Ollama
	ollamaPath, err := exec.LookPath("ollama")
	if err != nil {
		fmt.Println("WARNING: Ollama not found.")
		fmt.Println("   Install from: https://ollama.ai")
		fmt.Println("   Then re-run this setup.")
		os.Exit(1)
	}
	fmt.Printf("OK: Ollama found at %s\n", ollamaPath)

	// Check Ollama is running
	fmt.Print("   Checking Ollama service... ")
	checkCmd := exec.Command("ollama", "list")
	if err := checkCmd.Run(); err != nil {
		fmt.Println("ERROR: not running")
		fmt.Println("   Start Ollama with: ollama serve")
		os.Exit(1)
	}
	fmt.Println("OK: running")

	// Pull required models
	fmt.Println("\nPulling required models (this may take a few minutes)...")

	for _, model := range []string{"qwen2.5:7b", "nomic-embed-text"} {
		fmt.Printf("   Pulling %s...\n", model)
		pullCmd := exec.Command("ollama", "pull", model)
		pullCmd.Stdout = os.Stdout
		pullCmd.Stderr = os.Stderr
		if err := pullCmd.Run(); err != nil {
			fmt.Printf("WARNING: Failed to pull %s: %v\n", model, err)
			fmt.Println("   You can pull it manually: ollama pull " + model)
		} else {
			fmt.Printf("OK: %s ready\n", model)
		}
	}

	// Get user preferences
	fmt.Println()
	userName := ask("Your name", "")
	connectionName := ask("Workspace name", "default")
	displayName := ask("Workspace display name", "My Workspace")

	// Find/create config and data directories
	projectDir, _ := os.Getwd()
	configDir := filepath.Join(projectDir, "config")
	dataDir := filepath.Join(projectDir, "data")
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(dataDir, 0755)

	// Create connections.json
	dbPath := filepath.Join(dataDir, "memento.db")
	conn := connections.Connection{
		Name:        connectionName,
		DisplayName: displayName,
		Description: fmt.Sprintf("Memory workspace for %s", userName),
		Enabled:     true,
		CreatedAt:   time.Now().Format(time.RFC3339),
		Database:    connections.DatabaseConfig{Type: "sqlite", Path: dbPath},
		LLM:         connections.LLMConfig{Provider: "ollama", Model: "qwen2.5:7b"},
	}
	cfg := connections.ConnectionsConfig{
		DefaultConnection: connectionName,
		Connections:       []connections.Connection{conn},
	}
	cfg.Settings.AutoCreateDefault = true
	cfg.Settings.MaxConnections = 10
	cfg.Settings.AllowUserCreate = true

	configPath := filepath.Join(configDir, "connections.json")
	writeConnectionsConfig(configPath, cfg)

	// Try to register MCP with Claude Code
	fmt.Println("\nRegistering with Claude Code...")
	execPath, _ := os.Executable()
	mcpBinary := filepath.Join(filepath.Dir(execPath), "memento-mcp")
	if _, err := os.Stat(mcpBinary); os.IsNotExist(err) {
		// Try project root
		mcpBinary = filepath.Join(projectDir, "memento-mcp")
	}

	if _, err := os.Stat(mcpBinary); err == nil {
		claudePath, err := exec.LookPath("claude")
		if err == nil {
			fmt.Printf("   Found claude at %s\n", claudePath)
			registerCmd := exec.Command(claudePath, "mcp", "add", "memento", mcpBinary,
				"--scope", "user",
				"-e", "MEMENTO_DATA_PATH="+dataDir,
				"-e", "MEMENTO_CONNECTIONS_CONFIG="+configPath,
			)
			if output, err := registerCmd.CombinedOutput(); err != nil {
				fmt.Printf("WARNING: Could not auto-register MCP: %v\n%s\n", err, output)
				printManualMCPInstructions(mcpBinary, dataDir, configPath)
			} else {
				fmt.Printf("OK: Registered with Claude Code!\n%s\n", output)
			}
		} else {
			fmt.Println("WARNING: claude CLI not found -- skipping auto-registration")
			printManualMCPInstructions(mcpBinary, dataDir, configPath)
		}
	} else {
		fmt.Printf("WARNING: memento-mcp binary not found at %s\n", mcpBinary)
		fmt.Println("   Build it first: go build -o memento-mcp ./cmd/memento-mcp/")
	}

	// Final instructions
	fmt.Printf(`
Setup complete! Workspace: %s

Start the web UI:
  ./memento-web

Then open: http://localhost:6363

`, displayName)

	if runtime.GOOS == "darwin" {
		fmt.Println("Tip: Add 'ollama serve' to your login items so Ollama starts automatically.")
	}
}

func writeConnectionsConfig(path string, cfg connections.ConnectionsConfig) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Printf("ERROR: Failed to marshal config: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Printf("ERROR: Failed to write config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("OK: Config written to %s\n", path)
}

func printManualMCPInstructions(mcpBinary, dataDir, configPath string) {
	fmt.Printf(`
To connect Memento to Claude Code, run:

  claude mcp add memento %s \
    --scope user \
    -e MEMENTO_DATA_PATH=%s \
    -e MEMENTO_CONNECTIONS_CONFIG=%s

Then restart Claude Code and verify with /mcp
`, mcpBinary, dataDir, configPath)
}

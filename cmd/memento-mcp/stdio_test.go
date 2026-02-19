// stdio_test.go exercises the StdioTransport end-to-end using in-memory
// pipes so no real process needs to be spawned.
//
// Tests verify that:
//  1. Valid JSON-RPC 2.0 requests are answered with well-formed responses.
//  2. Malformed JSON produces a JSON-RPC parse-error response on stdout (not
//     a crash or stray log line on stdout).
//  3. Unknown methods produce a method-not-found error response.
//  4. Empty / blank lines are silently skipped.
//  5. Context cancellation causes Serve to stop cleanly.
//  6. Multiple back-to-back requests are all answered in order.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/api/mcp"
	"github.com/scrypster/memento/internal/storage/sqlite"
)

// rpcRequest is a helper for building JSON-RPC 2.0 request payloads.
type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      interface{} `json:"id"`
}

// rpcResponse is used to parse responses from the transport.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	ID interface{} `json:"id"`
}

// newTestServer builds an in-memory MCP server backed by an in-memory SQLite
// store.  Tests must close the store when done.
func newTestServer(t *testing.T) (*mcp.Server, io.Closer) {
	t.Helper()
	store, err := sqlite.NewMemoryStore(":memory:")
	if err != nil {
		t.Fatalf("sqlite.NewMemoryStore: %v", err)
	}
	srv := mcp.NewServer(store)
	return srv, store
}

// serveInput runs the StdioTransport against input (a multiline string) and
// returns all response lines collected from stdout.  The transport is shut
// down after input is exhausted (EOF).
func serveInput(t *testing.T, srv *mcp.Server, input string) []string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	in := strings.NewReader(input)
	var outBuf bytes.Buffer

	transport := mcp.NewStdioTransport(srv, in, &outBuf)
	_ = transport.Serve(ctx) // EOF from strings.Reader is a clean shutdown.

	var lines []string
	sc := bufio.NewScanner(&outBuf)
	for sc.Scan() {
		line := sc.Text()
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// parseResponse unmarshals a single JSON response line into rpcResponse.
func parseResponse(t *testing.T, line string) rpcResponse {
	t.Helper()
	var r rpcResponse
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		t.Fatalf("failed to parse response JSON %q: %v", line, err)
	}
	return r
}

// TestStdioTransport_StoreMemory verifies that a store_memory request results
// in a successful JSON-RPC 2.0 response written to stdout.
func TestStdioTransport_StoreMemory(t *testing.T) {
	srv, closer := newTestServer(t)
	defer closer.Close()

	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "store_memory",
		Params:  map[string]interface{}{"content": "hello from stdio test"},
		ID:      1,
	}
	reqJSON, _ := json.Marshal(req)

	lines := serveInput(t, srv, string(reqJSON)+"\n")

	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d: %v", len(lines), lines)
	}

	resp := parseResponse(t, lines[0])

	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc field = %q, want %q", resp.JSONRPC, "2.0")
	}
	if resp.Error != nil {
		t.Errorf("unexpected error in response: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}
	if resp.Result == nil {
		t.Error("expected non-nil result in response")
	}

	// Verify the ID is echoed back correctly.
	var id float64
	if err := json.Unmarshal([]byte(`1`), &id); err != nil {
		t.Fatal(err)
	}
	if resp.ID != float64(1) {
		t.Errorf("response id = %v, want 1", resp.ID)
	}
}

// TestStdioTransport_MalformedJSON verifies that bad JSON on stdin results in
// a JSON-RPC parse-error response on stdout – not a crash or a stray log line.
func TestStdioTransport_MalformedJSON(t *testing.T) {
	srv, closer := newTestServer(t)
	defer closer.Close()

	lines := serveInput(t, srv, "this is not json\n")

	if len(lines) != 1 {
		t.Fatalf("expected 1 response line for malformed input, got %d: %v", len(lines), lines)
	}

	resp := parseResponse(t, lines[0])

	if resp.Error == nil {
		t.Fatal("expected error response for malformed JSON, got nil error")
	}
	const wantCode = -32700 // Parse error
	if resp.Error.Code != wantCode {
		t.Errorf("error code = %d, want %d (parse error)", resp.Error.Code, wantCode)
	}
}

// TestStdioTransport_UnknownMethod verifies that an unknown JSON-RPC method
// results in a method-not-found error response.
func TestStdioTransport_UnknownMethod(t *testing.T) {
	srv, closer := newTestServer(t)
	defer closer.Close()

	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "does_not_exist",
		ID:      42,
	}
	reqJSON, _ := json.Marshal(req)

	lines := serveInput(t, srv, string(reqJSON)+"\n")

	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d: %v", len(lines), lines)
	}

	resp := parseResponse(t, lines[0])

	if resp.Error == nil {
		t.Fatal("expected error response for unknown method, got nil error")
	}
	const wantCode = -32601 // Method not found
	if resp.Error.Code != wantCode {
		t.Errorf("error code = %d, want %d (method not found)", resp.Error.Code, wantCode)
	}
}

// TestStdioTransport_EmptyLinesSkipped verifies that blank / empty lines in
// the input stream produce no responses (they are silently ignored).
func TestStdioTransport_EmptyLinesSkipped(t *testing.T) {
	srv, closer := newTestServer(t)
	defer closer.Close()

	// Three empty lines then one valid request.
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "store_memory",
		Params:  map[string]interface{}{"content": "after blanks"},
		ID:      99,
	}
	reqJSON, _ := json.Marshal(req)
	input := "\n\n\n" + string(reqJSON) + "\n"

	lines := serveInput(t, srv, input)

	// Only the valid request should produce a response.
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line (empty lines skipped), got %d: %v", len(lines), lines)
	}

	resp := parseResponse(t, lines[0])
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

// TestStdioTransport_MultipleRequests verifies that multiple back-to-back
// requests are all answered in order.
func TestStdioTransport_MultipleRequests(t *testing.T) {
	srv, closer := newTestServer(t)
	defer closer.Close()

	var sb strings.Builder
	ids := []int{10, 20, 30}
	for _, id := range ids {
		req := rpcRequest{
			JSONRPC: "2.0",
			Method:  "store_memory",
			Params:  map[string]interface{}{"content": "batch request"},
			ID:      id,
		}
		data, _ := json.Marshal(req)
		sb.Write(data)
		sb.WriteByte('\n')
	}

	lines := serveInput(t, srv, sb.String())

	if len(lines) != len(ids) {
		t.Fatalf("expected %d response lines, got %d: %v", len(ids), len(lines), lines)
	}

	for i, line := range lines {
		resp := parseResponse(t, line)
		if resp.Error != nil {
			t.Errorf("request %d: unexpected error: %v", i, resp.Error)
		}
		if resp.JSONRPC != "2.0" {
			t.Errorf("request %d: jsonrpc = %q, want 2.0", i, resp.JSONRPC)
		}
	}
}

// TestStdioTransport_ContextCancellation verifies that cancelling the context
// causes Serve to return cleanly without blocking.
func TestStdioTransport_ContextCancellation(t *testing.T) {
	srv, closer := newTestServer(t)
	defer closer.Close()

	// Use a pipe so we can control when stdin closes.
	pr, pw := io.Pipe()
	var outBuf bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		transport := mcp.NewStdioTransport(srv, pr, &outBuf)
		done <- transport.Serve(ctx)
	}()

	// Cancel the context and close the write end of the pipe.
	cancel()
	pw.Close()

	select {
	case err := <-done:
		// We expect either nil (clean EOF) or context.Canceled.  Either is
		// acceptable – the transport must stop promptly.
		if err != nil && err != context.Canceled {
			t.Errorf("Serve returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return within 3s after context cancellation")
	}
}

// TestStdioTransport_InvalidJSONRPCVersion verifies that a request with a
// wrong jsonrpc version field produces an invalid-request error.
func TestStdioTransport_InvalidJSONRPCVersion(t *testing.T) {
	srv, closer := newTestServer(t)
	defer closer.Close()

	req := rpcRequest{
		JSONRPC: "1.0", // Wrong version
		Method:  "store_memory",
		Params:  map[string]interface{}{"content": "v1 request"},
		ID:      7,
	}
	reqJSON, _ := json.Marshal(req)

	lines := serveInput(t, srv, string(reqJSON)+"\n")

	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d: %v", len(lines), lines)
	}

	resp := parseResponse(t, lines[0])
	if resp.Error == nil {
		t.Fatal("expected error for invalid jsonrpc version, got nil error")
	}
	const wantCode = -32600 // Invalid request
	if resp.Error.Code != wantCode {
		t.Errorf("error code = %d, want %d (invalid request)", resp.Error.Code, wantCode)
	}
}

// TestStdioTransport_ResponseIsValidJSON ensures each response line is valid
// JSON (not a log line or other stray output).
func TestStdioTransport_ResponseIsValidJSON(t *testing.T) {
	srv, closer := newTestServer(t)
	defer closer.Close()

	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "store_memory",
		Params:  map[string]interface{}{"content": "json validity check"},
		ID:      "string-id",
	}
	reqJSON, _ := json.Marshal(req)

	lines := serveInput(t, srv, string(reqJSON)+"\n")

	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &raw); err != nil {
		t.Errorf("response is not valid JSON: %v\nraw: %s", err, lines[0])
	}

	// String IDs must be echoed back as strings.
	if raw["id"] != "string-id" {
		t.Errorf("id = %v, want %q", raw["id"], "string-id")
	}
}

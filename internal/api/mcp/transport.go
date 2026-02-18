// Package mcp – transport.go provides the StdioTransport that wires an MCP
// Server to Claude Desktop / Claude Code via line-delimited JSON-RPC 2.0 over
// stdin / stdout.
//
// Protocol rules (must be followed exactly):
//   - Each JSON-RPC request arrives as a single newline-terminated line on
//     stdin.
//   - Each JSON-RPC response is written as a single newline-terminated line to
//     stdout.
//   - ALL diagnostic output (logging, errors) MUST go to stderr only.  Any
//     stray bytes on stdout will corrupt the protocol framing.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
)

// StdioTransport reads line-delimited JSON-RPC 2.0 requests from an io.Reader
// and writes responses to an io.Writer.  It is the bridge between the raw
// stdio streams and the MCP Server.
//
// Logging is directed exclusively to stderr via a dedicated *log.Logger so
// that stdout is never contaminated.
type StdioTransport struct {
	server  *Server
	in      io.Reader
	out     io.Writer
	logger  *log.Logger
}

// NewStdioTransport constructs a StdioTransport that reads from in and writes
// to out.  All log messages are sent to stderr so that the stdout stream stays
// clean for JSON-RPC framing.
//
// Usage with real stdio:
//
//	t := mcp.NewStdioTransport(srv, os.Stdin, os.Stdout)
//	t.Serve(ctx)
func NewStdioTransport(srv *Server, in io.Reader, out io.Writer) *StdioTransport {
	return &StdioTransport{
		server: srv,
		in:     in,
		out:    out,
		// Explicitly target stderr so that log output never touches stdout.
		logger: log.New(os.Stderr, "memento-mcp: ", log.LstdFlags),
	}
}

// Serve processes JSON-RPC 2.0 requests until stdin is closed or ctx is
// cancelled.  It returns after the last line has been processed or the context
// is done, whichever comes first.
//
// Each request is handled synchronously in the order it arrives.  The MCP
// protocol does not require concurrent processing at the transport level.
func (t *StdioTransport) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(t.in)

	// Increase the scanner buffer to handle large payloads (up to 4 MB).
	const maxBuf = 4 * 1024 * 1024
	buf := make([]byte, maxBuf)
	scanner.Buffer(buf, maxBuf)

	for {
		// Check context before blocking on the next line.
		select {
		case <-ctx.Done():
			t.logger.Println("context cancelled – shutting down")
			return ctx.Err()
		default:
		}

		if !scanner.Scan() {
			// EOF or error.
			if err := scanner.Err(); err != nil {
				t.logger.Printf("stdin scanner error: %v", err)
				return fmt.Errorf("stdin scanner: %w", err)
			}
			// Clean EOF – stdin was closed.
			t.logger.Println("stdin closed – shutting down")
			return nil
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		resp, err := t.server.HandleRequest(ctx, line)
		if err != nil {
			// HandleRequest already produced a JSON-RPC error response in most
			// cases, but if it returned an error we synthesise one here so the
			// caller always gets a valid response frame.
			t.logger.Printf("handler error: %v", err)
			resp = t.internalErrorResponse(line, err)
		}

		if err := t.writeResponse(resp); err != nil {
			t.logger.Printf("write error: %v", err)
			return fmt.Errorf("write response: %w", err)
		}

		// Check context again after processing in case a signal arrived during
		// the (potentially slow) handler call.
		select {
		case <-ctx.Done():
			t.logger.Println("context cancelled after handler – shutting down")
			return ctx.Err()
		default:
		}
	}
}

// writeResponse writes a single JSON-RPC response line to stdout.  A trailing
// newline is appended so the caller can frame responses by line.
func (t *StdioTransport) writeResponse(resp []byte) error {
	_, err := fmt.Fprintf(t.out, "%s\n", resp)
	return err
}

// internalErrorResponse builds a best-effort JSON-RPC error response when the
// server returns an unexpected error.  It attempts to extract the request ID
// from the raw request bytes so the caller can correlate the response.
func (t *StdioTransport) internalErrorResponse(rawRequest []byte, handlerErr error) []byte {
	// Try to recover the request ID.
	var partial struct {
		ID interface{} `json:"id"`
	}
	_ = json.Unmarshal(rawRequest, &partial)

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      partial.ID,
		Error: &JSONRPCError{
			Code:    ErrCodeInternalError,
			Message: handlerErr.Error(),
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		// Last resort: return a hard-coded error so the protocol framing
		// does not stall.
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal error"}}`)
	}
	return data
}

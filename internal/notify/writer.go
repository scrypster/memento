// Package notify provides cross-process enrichment event notification
// between memento-mcp and memento-web using filesystem events.
package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Event is the payload written to an event file.
type Event struct {
	Type     string `json:"type"`
	MemoryID string `json:"memory_id"`
	Time     int64  `json:"time"`
}

// EventWriter writes notification event files to a shared directory.
type EventWriter struct {
	dir string
}

// NewEventWriter creates a writer that emits events to {dataPath}/events/.
func NewEventWriter(dataPath string) *EventWriter {
	return &EventWriter{dir: filepath.Join(dataPath, "events")}
}

// Notify writes an event file with the given type.
// Safe to call concurrently. Errors are returned but not fatal.
func (w *EventWriter) Notify(eventType, memoryID string) error {
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return fmt.Errorf("notify: mkdir %s: %w", w.dir, err)
	}
	evt := Event{
		Type:     eventType,
		MemoryID: memoryID,
		Time:     time.Now().UnixNano(),
	}
	data, _ := json.Marshal(evt)
	filename := fmt.Sprintf("%d-%s.event", evt.Time, sanitizeID(memoryID))
	path := filepath.Join(w.dir, filename)
	return os.WriteFile(path, data, 0o600)
}

// sanitizeID replaces characters unsafe for filenames.
func sanitizeID(id string) string {
	out := make([]byte, len(id))
	for i := 0; i < len(id); i++ {
		if id[i] == '/' || id[i] == ':' {
			out[i] = '_'
		} else {
			out[i] = id[i]
		}
	}
	return string(out)
}

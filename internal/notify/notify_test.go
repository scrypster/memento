package notify

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEventWriterCreatesFile(t *testing.T) {
	dir := t.TempDir()
	w := NewEventWriter(dir)

	if err := w.Notify("enrichment_complete", "mem:general:abc123"); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "events"))
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 event file, got %d", len(entries))
	}
	if filepath.Ext(entries[0].Name()) != ".event" {
		t.Errorf("expected .event extension, got %s", entries[0].Name())
	}
}

func TestEventWatcherReceivesEvent(t *testing.T) {
	dir := t.TempDir()

	type eventMsg struct {
		eventType string
		memoryID  string
	}
	received := make(chan eventMsg, 1)

	watcher := NewEventWatcher(dir, func(eventType, memoryID string) {
		received <- eventMsg{eventType, memoryID}
	})
	if err := watcher.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer watcher.Stop()

	// Give fsnotify a moment to register
	time.Sleep(50 * time.Millisecond)

	writer := NewEventWriter(dir)
	if err := writer.Notify("memory_created", "mem:general:test123"); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	select {
	case msg := <-received:
		if msg.eventType != "memory_created" {
			t.Errorf("expected event type memory_created, got %s", msg.eventType)
		}
		if msg.memoryID != "mem:general:test123" {
			t.Errorf("expected mem:general:test123, got %s", msg.memoryID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventWatcherDrainsExisting(t *testing.T) {
	dir := t.TempDir()

	// Write events BEFORE starting watcher
	writer := NewEventWriter(dir)
	_ = writer.Notify("memory_created", "mem:general:drain1")
	_ = writer.Notify("enrichment_complete", "mem:general:drain2")

	received := make(chan string, 10)
	watcher := NewEventWatcher(dir, func(eventType, memoryID string) {
		received <- memoryID
	})
	if err := watcher.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer watcher.Stop()

	// Drain should have processed both files synchronously during Start
	time.Sleep(100 * time.Millisecond)

	if len(received) != 2 {
		t.Fatalf("expected 2 drained events, got %d", len(received))
	}
}

func TestEventTypeRoundTrip(t *testing.T) {
	// Verify all three event types survive the write→watch round-trip
	eventTypes := []string{"memory_created", "enrichment_started", "enrichment_complete"}

	for _, evtType := range eventTypes {
		t.Run(evtType, func(t *testing.T) {
			dir := t.TempDir()

			type eventMsg struct {
				eventType string
				memoryID  string
			}
			received := make(chan eventMsg, 1)

			watcher := NewEventWatcher(dir, func(eventType, memoryID string) {
				received <- eventMsg{eventType, memoryID}
			})
			if err := watcher.Start(); err != nil {
				t.Fatalf("Start failed: %v", err)
			}
			defer watcher.Stop()

			time.Sleep(50 * time.Millisecond)

			writer := NewEventWriter(dir)
			if err := writer.Notify(evtType, "mem:test:roundtrip"); err != nil {
				t.Fatalf("Notify failed: %v", err)
			}

			select {
			case msg := <-received:
				if msg.eventType != evtType {
					t.Errorf("expected event type %s, got %s", evtType, msg.eventType)
				}
				if msg.memoryID != "mem:test:roundtrip" {
					t.Errorf("expected mem:test:roundtrip, got %s", msg.memoryID)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("timeout waiting for event")
			}
		})
	}
}

func TestSanitizeID(t *testing.T) {
	got := sanitizeID("mem:general:abc/def")
	if got != "mem_general_abc_def" {
		t.Errorf("expected mem_general_abc_def, got %s", got)
	}
}

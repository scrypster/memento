package notify

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// EventWatcher watches the events directory and dispatches callbacks.
type EventWatcher struct {
	dir      string
	callback func(eventType, memoryID string)
	watcher  *fsnotify.Watcher
	done     chan struct{}
}

// NewEventWatcher creates a watcher for {dataPath}/events/.
func NewEventWatcher(dataPath string, callback func(eventType, memoryID string)) *EventWatcher {
	return &EventWatcher{
		dir:      filepath.Join(dataPath, "events"),
		callback: callback,
		done:     make(chan struct{}),
	}
}

// Start begins watching. It drains any existing event files first,
// then watches for new ones. Call Stop() to clean up.
func (ew *EventWatcher) Start() error {
	if err := os.MkdirAll(ew.dir, 0o700); err != nil {
		return err
	}

	ew.drainExisting()

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(ew.dir); err != nil {
		_ = w.Close()
		return err
	}
	ew.watcher = w

	go ew.loop()
	log.Printf("notify: watching %s for enrichment events", ew.dir)
	return nil
}

// Stop shuts down the watcher.
func (ew *EventWatcher) Stop() {
	if ew.watcher != nil {
		_ = ew.watcher.Close()
	}
	<-ew.done
}

func (ew *EventWatcher) loop() {
	defer close(ew.done)
	for {
		select {
		case evt, ok := <-ew.watcher.Events:
			if !ok {
				return
			}
			if evt.Op&fsnotify.Create != 0 && strings.HasSuffix(evt.Name, ".event") {
				ew.processFile(evt.Name)
			}
		case err, ok := <-ew.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("notify: watcher error: %v", err)
		}
	}
}

func (ew *EventWatcher) drainExisting() {
	entries, err := os.ReadDir(ew.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".event") {
			ew.processFile(filepath.Join(ew.dir, entry.Name()))
		}
	}
}

func (ew *EventWatcher) processFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // file already consumed by another process
	}
	_ = os.Remove(path)

	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("notify: invalid event file %s: %v", filepath.Base(path), err)
		return
	}

	if event.MemoryID != "" && ew.callback != nil {
		ew.callback(event.Type, event.MemoryID)
	}
}

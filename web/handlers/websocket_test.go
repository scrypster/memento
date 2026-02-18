package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scrypster/memento/web/handlers"
	"github.com/stretchr/testify/assert"
)

func TestWebSocketHub_ValidatesOrigin(t *testing.T) {
	hub := handlers.NewWebSocketHub()
	defer hub.Stop()

	// Test with invalid origin - should reject with 403
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Origin", "http://evil.com")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	w := httptest.NewRecorder()
	hub.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Forbidden")
}

func TestWebSocketHub_Broadcast(t *testing.T) {
	hub := handlers.NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	// Create mock client
	received := make(chan []byte, 1)
	mockClient := &handlers.MockClient{
		SendChan: received,
	}

	hub.Register(mockClient)

	// Give the hub time to register the client
	time.Sleep(10 * time.Millisecond)

	// Broadcast message
	message := map[string]interface{}{
		"type": "test",
		"data": "hello",
	}
	hub.Broadcast(message)

	// Wait for message
	select {
	case msg := <-received:
		assert.Contains(t, string(msg), "test")
		assert.Contains(t, string(msg), "hello")
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for broadcast message")
	}
}

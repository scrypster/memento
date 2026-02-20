package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket" //nolint:staticcheck // TODO: migrate to github.com/coder/websocket
)

// WebSocketHub manages WebSocket connections and broadcasts messages.
type WebSocketHub struct {
	clients    map[clientInterface]bool
	broadcast  chan interface{}
	register   chan clientInterface
	unregister chan clientInterface
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

// clientInterface allows for both real clients and mock clients.
type clientInterface interface {
	getSendChannel() chan []byte
	close()
}

// Client represents a WebSocket connection.
type Client struct {
	hub  *WebSocketHub
	conn *websocket.Conn //nolint:staticcheck // TODO: migrate to github.com/coder/websocket
	send chan []byte
}

func (c *Client) getSendChannel() chan []byte {
	return c.send
}

func (c *Client) close() {
	if c.conn != nil {
		_ = c.conn.Close(websocket.StatusNormalClosure, "") //nolint:staticcheck // TODO: migrate to github.com/coder/websocket
	}
}

// NewWebSocketHub creates a new WebSocket hub.
func NewWebSocketHub() *WebSocketHub {
	ctx, cancel := context.WithCancel(context.Background())
	return &WebSocketHub{
		clients:    make(map[clientInterface]bool),
		broadcast:  make(chan interface{}, 256),
		register:   make(chan clientInterface),
		unregister: make(chan clientInterface),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Run starts the hub's message processing loop.
func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("WebSocket client connected (total: %d)", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.getSendChannel())
			}
			count := len(h.clients)
			h.mu.Unlock()
			log.Printf("WebSocket client disconnected (total: %d)", count)

		case message := <-h.broadcast:
			// Use a full Lock because we may delete from the map in the default branch.
			h.mu.Lock()
			data, err := json.Marshal(message)
			if err != nil {
				log.Printf("ERROR: Failed to marshal WebSocket message: %v", err)
				h.mu.Unlock()
				continue
			}

			for client := range h.clients {
				sendChan := client.getSendChannel()
				select {
				case sendChan <- data:
				default:
					// Client's send channel is full, disconnect them
					close(sendChan)
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()

		case <-h.ctx.Done():
			log.Println("WebSocket hub stopping...")
			return
		}
	}
}

// Stop gracefully shuts down the hub.
func (h *WebSocketHub) Stop() {
	h.cancel()

	h.mu.Lock()
	for client := range h.clients {
		close(client.getSendChannel())
		client.close()
	}
	h.clients = make(map[clientInterface]bool)
	h.mu.Unlock()
}

// Broadcast sends a message to all connected clients.
func (h *WebSocketHub) Broadcast(message interface{}) {
	select {
	case h.broadcast <- message:
	default:
		log.Println("WARNING: WebSocket broadcast channel full, dropping message")
	}
}

// Register adds a client to the hub.
func (h *WebSocketHub) Register(client clientInterface) {
	h.register <- client
}

// Unregister removes a client from the hub.
func (h *WebSocketHub) Unregister(client clientInterface) {
	h.unregister <- client
}

// ServeHTTP handles WebSocket upgrade requests.
func (h *WebSocketHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Validate Origin header
	origin := r.Header.Get("Origin")
	if origin != "" {
		allowedOrigins := map[string]bool{
			"http://localhost:6363":  true,
			"http://127.0.0.1:6363": true,
		}
		if !allowedOrigins[origin] {
			http.Error(w, "Forbidden: invalid origin", http.StatusForbidden)
			return
		}
	}

	// Upgrade connection
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{ //nolint:staticcheck // TODO: migrate to github.com/coder/websocket
		OriginPatterns: []string{"localhost:6363", "127.0.0.1:6363"},
	})
	if err != nil {
		log.Printf("ERROR: WebSocket upgrade failed: %v", err)
		return
	}

	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}

	h.Register(client)

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

// writePump sends messages to the WebSocket connection.
func (c *Client) writePump() {
	defer func() {
		c.hub.Unregister(c)
		_ = c.conn.Close(websocket.StatusNormalClosure, "") //nolint:staticcheck // TODO: migrate to github.com/coder/websocket
	}()

	for message := range c.send {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := c.conn.Write(ctx, websocket.MessageText, message) //nolint:staticcheck // TODO: migrate to github.com/coder/websocket
		cancel()

		if err != nil {
			log.Printf("ERROR: WebSocket write failed: %v", err)
			return
		}
	}
}

// readPump reads messages from the WebSocket connection.
// Currently just drains messages to detect disconnections.
func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister(c)
		_ = c.conn.Close(websocket.StatusNormalClosure, "") //nolint:staticcheck // TODO: migrate to github.com/coder/websocket
	}()

	for {
		_, _, err := c.conn.Read(context.Background()) //nolint:staticcheck // TODO: migrate to github.com/coder/websocket
		if err != nil {
			// Connection closed
			return
		}
		// Currently we just drain messages
		// Future: handle client->server messages (subscriptions, etc.)
	}
}

// MockClient is a mock client for testing.
type MockClient struct {
	SendChan chan []byte
}

func (m *MockClient) getSendChannel() chan []byte {
	return m.SendChan
}

func (m *MockClient) close() {
	// No-op for mock client
}

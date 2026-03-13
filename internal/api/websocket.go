package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/clawtrade/clawtrade/internal/engine"
	"github.com/gorilla/websocket"
)

// WSMessage is the JSON message format sent over WebSocket connections.
type WSMessage struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// Client represents a single WebSocket connection with optional subscriptions.
type Client struct {
	hub           *Hub
	conn          *websocket.Conn
	send          chan []byte
	mu            sync.RWMutex
	subscriptions map[string]bool
}

// Hub manages a set of active WebSocket clients and broadcasts messages to them.
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	done       chan struct{}
}

// NewHub creates a new Hub ready for use.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		done:       make(chan struct{}),
	}
}

// Run starts the hub's main loop that processes register/unregister requests.
// It blocks until Stop is called.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		case <-h.done:
			return
		}
	}
}

// Stop shuts down the hub's main loop.
func (h *Hub) Stop() {
	close(h.done)
}

// Clients returns the current number of connected clients.
func (h *Hub) Clients() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Broadcast sends a WSMessage to every connected client.
func (h *Hub) Broadcast(msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("websocket: failed to marshal broadcast message: %v", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			// Client buffer full; drop message to avoid blocking.
		}
	}
}

// BroadcastToSubscribers sends a message only to clients subscribed to the given eventType.
func (h *Hub) BroadcastToSubscribers(eventType string, data any) {
	msg := WSMessage{Type: eventType, Data: data}
	raw, err := json.Marshal(msg)
	if err != nil {
		log.Printf("websocket: failed to marshal subscriber message: %v", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		client.mu.RLock()
		subscribed := len(client.subscriptions) == 0 || client.subscriptions[eventType]
		client.mu.RUnlock()
		if subscribed {
			select {
			case client.send <- raw:
			default:
			}
		}
	}
}

// SubscribeToEvents registers the hub as a listener on the EventBus for the
// specified event types, forwarding each event to subscribed WebSocket clients.
func (h *Hub) SubscribeToEvents(bus *engine.EventBus, eventTypes []string) {
	for _, et := range eventTypes {
		et := et // capture loop variable
		bus.Subscribe(et, func(e engine.Event) {
			h.BroadcastToSubscribers(e.Type, e.Data)
		})
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleWebSocket is an HTTP handler that upgrades the connection to a WebSocket
// and registers the client with the hub.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket: upgrade error: %v", err)
		return
	}

	client := &Client{
		hub:           h,
		conn:          conn,
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}
	h.register <- client

	go client.writePump()
	go client.readPump()
}

// readPump reads messages from the WebSocket connection. It handles subscribe
// messages that let clients choose which event types they receive.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}
		if msg.Type == "subscribe" {
			if eventType, ok := msg.Data.(string); ok {
				c.mu.Lock()
				c.subscriptions[eventType] = true
				c.mu.Unlock()
			}
		}
	}
}

// writePump pumps messages from the send channel to the WebSocket connection.
func (c *Client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}

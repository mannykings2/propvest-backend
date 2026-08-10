// Package realtime implements a WebSocket hub for pushing live events to users.
//
// WHY WEBSOCKETS?
// ---------------
// Plain HTTP is request/response: the client must ask before it gets anything.
// For things like "your deposit just cleared" or "you have a new notification",
// we want the SERVER to push the moment it happens. A WebSocket is a persistent,
// bidirectional connection that stays open so the server can send at any time.
//
// THE HUB PATTERN
// ---------------
// Managing many concurrent connections with raw goroutines and shared maps is a
// classic source of data races. The hub centralises that: a single goroutine
// (Run) owns the registry of clients and processes register/unregister/broadcast
// requests received over channels. Everyone else talks to the hub only through
// channels, so there are no locks around the client map and no races.
//
//	Client A ─┐                        ┌─► Client A.send
//	Client B ─┤── register/unregister ─┤
//	          │                        │
//	push()  ──┴────► hub.Run (owner) ──┴─► fan out to that user's clients
//
// We key clients by userID so we can target "push to user X" (their phone AND
// laptop both receive it).
package realtime

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mannykings2/propvest-backend/internal/logger"
)

// Client is one WebSocket connection belonging to a user.
type Client struct {
	userID string
	conn   *websocket.Conn
	send   chan []byte // buffered outbound queue for this connection
	hub    *Hub
}

// Hub owns all active clients and fans out messages to them.
type Hub struct {
	// clients maps userID -> set of that user's live connections.
	clients map[string]map[*Client]struct{}

	register   chan *Client
	unregister chan *Client
	broadcast  chan outbound

	mu sync.RWMutex // guards clients for the count/read helpers only
}

// outbound is a "send this JSON to this user" instruction.
type outbound struct {
	userID string
	data   []byte
}

// NewHub creates an empty hub. Call Run in a goroutine to start it.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan outbound, 256),
	}
}

// Run is the hub's event loop. It is the ONLY goroutine that mutates the clients
// map, which is what makes the whole thing race-free. Run it once: go hub.Run().
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			if h.clients[c.userID] == nil {
				h.clients[c.userID] = make(map[*Client]struct{})
			}
			h.clients[c.userID][c] = struct{}{}
			h.mu.Unlock()
			logger.Debug("ws client registered", "user_id", c.userID)

		case c := <-h.unregister:
			h.mu.Lock()
			if set, ok := h.clients[c.userID]; ok {
				if _, ok := set[c]; ok {
					delete(set, c)
					close(c.send)
					if len(set) == 0 {
						delete(h.clients, c.userID)
					}
				}
			}
			h.mu.Unlock()
			logger.Debug("ws client unregistered", "user_id", c.userID)

		case msg := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients[msg.userID] {
				select {
				case c.send <- msg.data:
				default:
					// Slow client: drop it rather than block the whole hub.
					close(c.send)
					delete(h.clients[msg.userID], c)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// PushToUser queues data for delivery to all of a user's connections. Non-
// blocking; if the hub buffer is full the message is dropped (realtime pushes
// are best-effort — the durable copy lives in the notifications table).
func (h *Hub) PushToUser(userID string, data []byte) {
	select {
	case h.broadcast <- outbound{userID: userID, data: data}:
	default:
		logger.Warn("realtime broadcast buffer full; dropping push", "user_id", userID)
	}
}

// OnlineUsers returns how many distinct users currently have a live socket.
func (h *Hub) OnlineUsers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ── Per-connection read/write pumps ────────────────────────────────────────

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMessage = 1024
)

// NewClient wires a raw WebSocket connection to the hub and starts its pumps.
// Called by the HTTP handler after a successful upgrade + auth.
func (h *Hub) NewClient(userID string, conn *websocket.Conn) {
	c := &Client{userID: userID, conn: conn, send: make(chan []byte, 32), hub: h}
	h.register <- c
	go c.writePump()
	go c.readPump()
}

// readPump drains inbound frames. We don't accept commands from the client yet,
// but we must read to process control frames (ping/pong/close) and detect
// disconnects. On any error the client is unregistered.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessage)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

// writePump sends queued messages and periodic pings to keep the connection alive.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok { // hub closed the channel
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

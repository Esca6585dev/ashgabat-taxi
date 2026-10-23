// Package ws is the realtime hub: every connected app (client / driver / admin)
// gets a Conn; the server pushes JSON events {"type": "...", "data": ...}.
package ws

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
)

// Event is a message sent to apps.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
	At   int64  `json:"at"`
}

// Conn wraps a websocket with a buffered outgoing channel.
type Conn struct {
	UserID uint
	Role   string
	ws     *websocket.Conn
	send   chan []byte
	once   sync.Once
}

// Hub tracks connections per user id.
type Hub struct {
	mu    sync.RWMutex
	conns map[uint]map[*Conn]struct{}

	// OnSend, when set, sees every per-user event (push notifications); it must not block.
	OnSend func(userID uint, typ string, data any)
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{conns: make(map[uint]map[*Conn]struct{})}
}

// Register adds a connection and starts its writer goroutine.
func (h *Hub) Register(userID uint, role string, ws *websocket.Conn) *Conn {
	c := &Conn{UserID: userID, Role: role, ws: ws, send: make(chan []byte, 64)}
	h.mu.Lock()
	if h.conns[userID] == nil {
		h.conns[userID] = make(map[*Conn]struct{})
	}
	h.conns[userID][c] = struct{}{}
	h.mu.Unlock()
	go c.writer()
	return c
}

// Unregister removes a connection.
func (h *Hub) Unregister(c *Conn) {
	h.mu.Lock()
	if set, ok := h.conns[c.UserID]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.conns, c.UserID)
		}
	}
	h.mu.Unlock()
	c.close()
}

func (c *Conn) close() {
	c.once.Do(func() { close(c.send) })
}

func (c *Conn) writer() {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				_ = c.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// IsOnline reports whether the user has at least one live socket.
func (h *Hub) IsOnline(userID uint) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns[userID]) > 0
}

// SendTo pushes an event to every socket of a user. Returns true if delivered to at least one.
func (h *Hub) SendTo(userID uint, typ string, data any) bool {
	if h.OnSend != nil {
		h.OnSend(userID, typ, data)
	}
	payload, err := json.Marshal(Event{Type: typ, Data: data, At: time.Now().Unix()})
	if err != nil {
		log.Printf("ws: marshal %s: %v", typ, err)
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	delivered := false
	for c := range h.conns[userID] {
		select {
		case c.send <- payload:
			delivered = true
		default: // slow consumer, drop
		}
	}
	return delivered
}

// SendToMany pushes an event to several users.
func (h *Hub) SendToMany(userIDs []uint, typ string, data any) {
	for _, id := range userIDs {
		h.SendTo(id, typ, data)
	}
}

// BroadcastRole pushes an event to every connected user with the given role.
func (h *Hub) BroadcastRole(role, typ string, data any) {
	payload, err := json.Marshal(Event{Type: typ, Data: data, At: time.Now().Unix()})
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, set := range h.conns {
		for c := range set {
			if c.Role == role {
				select {
				case c.send <- payload:
				default:
				}
			}
		}
	}
}

// ConnectedCount returns the number of distinct users online.
func (h *Hub) ConnectedCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

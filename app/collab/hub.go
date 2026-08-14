package collab

import (
	"sync"

	"github.com/gorilla/websocket"
)

// Client is a single WebSocket connection associated with a dag room.
type Client struct {
	DagId string
	Conn  *websocket.Conn
	Send  chan []byte
}

// Hub manages per-diagram WebSocket rooms.
type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[*Client]bool
}

func NewHub() *Hub {
	return &Hub{rooms: make(map[string]map[*Client]bool)}
}

// Join adds the client to the room for dagId.
func (h *Hub) Join(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[c.DagId] == nil {
		h.rooms[c.DagId] = make(map[*Client]bool)
	}
	h.rooms[c.DagId][c] = true
}

// Leave removes the client from its room and closes its send channel.
func (h *Hub) Leave(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[c.DagId]
	if room == nil {
		return
	}
	if _, ok := room[c]; ok {
		delete(room, c)
		close(c.Send)
	}
	if len(room) == 0 {
		delete(h.rooms, c.DagId)
	}
}

// Broadcast sends msg to every client in dagId's room except the one in except
// (pass nil to send to all). Slow clients are skipped — their message is dropped
// rather than blocking the broadcaster.
func (h *Hub) Broadcast(dagId string, msg []byte, except *Client) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.rooms[dagId] {
		if c == except {
			continue
		}
		select {
		case c.Send <- msg:
		default:
		}
	}
}

// WritePump pumps messages from the client's send channel to the WebSocket.
// Call it in its own goroutine; it returns when the channel is closed.
func (c *Client) WritePump() {
	defer c.Conn.Close()
	for msg := range c.Send {
		if err := c.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

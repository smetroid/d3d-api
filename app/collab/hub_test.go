package collab

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient creates a connected WebSocket client pair (server + client conn).
func newTestClient(t *testing.T, hub *Hub, dagId string) (*Client, *websocket.Conn) {
	t.Helper()

	var serverConn *websocket.Conn
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		require.NoError(t, err)
		serverConn = conn
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() { clientConn.Close() })

	// Give the handler goroutine time to set serverConn.
	time.Sleep(10 * time.Millisecond)

	client := &Client{
		DagId: dagId,
		Conn:  serverConn,
		Send:  make(chan []byte, 8),
	}
	hub.Join(client)
	t.Cleanup(func() { hub.Leave(client) })

	return client, clientConn
}

func TestHub_JoinCreatesRoom(t *testing.T) {
	hub := NewHub()
	client, _ := newTestClient(t, hub, "dag-1")

	hub.mu.RLock()
	_, ok := hub.rooms["dag-1"][client]
	hub.mu.RUnlock()

	assert.True(t, ok, "client should be in room dag-1 after Join")
}

func TestHub_LeaveRemovesClient(t *testing.T) {
	hub := NewHub()
	client, _ := newTestClient(t, hub, "dag-1")

	hub.Leave(client)

	hub.mu.RLock()
	_, ok := hub.rooms["dag-1"][client]
	hub.mu.RUnlock()

	assert.False(t, ok, "client should be removed from room after Leave")
}

func TestHub_LeaveDeletesEmptyRoom(t *testing.T) {
	hub := NewHub()
	client, _ := newTestClient(t, hub, "dag-1")
	// Override cleanup — we call Leave manually.
	hub.Leave(client)

	hub.mu.RLock()
	_, roomExists := hub.rooms["dag-1"]
	hub.mu.RUnlock()

	assert.False(t, roomExists, "empty room should be deleted after last client leaves")
}

func TestHub_BroadcastDeliversToOthers(t *testing.T) {
	hub := NewHub()
	sender, _ := newTestClient(t, hub, "dag-1")
	receiver, _ := newTestClient(t, hub, "dag-1")

	msg := []byte(`{"type":"diagram:updated"}`)
	hub.Broadcast("dag-1", msg, sender)

	select {
	case got := <-receiver.Send:
		assert.Equal(t, msg, got)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for broadcast message")
	}
}

func TestHub_BroadcastSkipsSender(t *testing.T) {
	hub := NewHub()
	sender, _ := newTestClient(t, hub, "dag-1")
	newTestClient(t, hub, "dag-1") // second client

	msg := []byte(`{"type":"diagram:updated"}`)
	hub.Broadcast("dag-1", msg, sender)

	// sender's Send channel should be empty
	select {
	case <-sender.Send:
		t.Fatal("sender should not receive its own broadcast")
	case <-time.After(30 * time.Millisecond):
		// expected
	}
}

func TestHub_BroadcastDifferentRoomIgnored(t *testing.T) {
	hub := NewHub()
	newTestClient(t, hub, "dag-1")
	otherRoom, _ := newTestClient(t, hub, "dag-2")

	msg := []byte(`{"type":"diagram:updated"}`)
	hub.Broadcast("dag-1", msg, nil)

	select {
	case <-otherRoom.Send:
		t.Fatal("client in dag-2 should not receive broadcast for dag-1")
	case <-time.After(30 * time.Millisecond):
		// expected
	}
}

func TestClient_WritePump(t *testing.T) {
	hub := NewHub()
	client, clientConn := newTestClient(t, hub, "dag-1")

	go client.WritePump()

	msg := []byte(`{"type":"presence"}`)
	client.Send <- msg

	clientConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, got, err := clientConn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, msg, got)
}

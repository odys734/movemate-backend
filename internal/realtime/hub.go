package realtime

import (
	"sync"

	"github.com/gorilla/websocket"
)

type Client struct {
	UserID    string
	BookingID string
	Conn      *websocket.Conn
	Send      chan []byte
	closeOnce sync.Once
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*Client]bool
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[*Client]bool),
	}
}

func (h *Hub) AddClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[client.BookingID] == nil {
		h.clients[client.BookingID] = make(map[*Client]bool)
	}

	h.clients[client.BookingID][client] = true
}

func (h *Hub) RemoveClient(client *Client) {
	client.closeOnce.Do(func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		clients := h.clients[client.BookingID]

		if clients != nil {
			delete(clients, client)

			if len(clients) == 0 {
				delete(h.clients, client.BookingID)
			}
		}

		close(client.Send)
	})
}

func (h *Hub) Broadcast(bookingID string, message []byte, exclude *Client) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients[bookingID] {
		if client == exclude {
			continue
		}

		select {
		case client.Send <- message:
		default:
			// Don't block the hub if a client is not reading.
		}
	}
}

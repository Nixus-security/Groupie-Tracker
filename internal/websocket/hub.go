package websocket

import (
	"encoding/json"
	"sync"
)

type Hub struct {
	rooms      map[string]map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan *RoomMessage
	mu         sync.RWMutex
	shutdown   chan struct{}
}

type RoomMessage struct {
	RoomCode string
	Data     []byte
}

func NewHub() *Hub {
	return &Hub{
		rooms:      make(map[string]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *RoomMessage, 256),
		shutdown:   make(chan struct{}),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.rooms[client.RoomCode] == nil {
				h.rooms[client.RoomCode] = make(map[*Client]bool)
			}
			h.rooms[client.RoomCode][client] = true
			h.mu.Unlock()

			h.BroadcastToRoom(client.RoomCode, map[string]interface{}{
				"type":    "user_joined",
				"user_id": client.UserID,
				"pseudo":  client.Pseudo,
				"count":   h.GetRoomClientCount(client.RoomCode),
			})

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.rooms[client.RoomCode]; ok {
				if _, ok := clients[client]; ok {
					delete(clients, client)
					close(client.Send)

					if len(clients) == 0 {
						delete(h.rooms, client.RoomCode)
					}
				}
			}
			h.mu.Unlock()

			h.BroadcastToRoom(client.RoomCode, map[string]interface{}{
				"type":    "user_left",
				"user_id": client.UserID,
				"pseudo":  client.Pseudo,
				"count":   h.GetRoomClientCount(client.RoomCode),
			})

		case message := <-h.broadcast:
			h.mu.RLock()
			clients := h.rooms[message.RoomCode]
			h.mu.RUnlock()

			for client := range clients {
				select {
				case client.Send <- message.Data:
				default:
					h.mu.Lock()
					delete(h.rooms[message.RoomCode], client)
					close(client.Send)
					h.mu.Unlock()
				}
			}

		case <-h.shutdown:
			h.mu.Lock()
			for roomCode, clients := range h.rooms {
				for client := range clients {
					close(client.Send)
					delete(clients, client)
				}
				delete(h.rooms, roomCode)
			}
			h.mu.Unlock()
			return
		}
	}
}

func (h *Hub) Shutdown() {
	close(h.shutdown)
}

func (h *Hub) BroadcastToRoom(roomCode string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}

	select {
	case h.broadcast <- &RoomMessage{RoomCode: roomCode, Data: jsonData}:
	default:
	}
}

func (h *Hub) SendToClient(client *Client, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}

	select {
	case client.Send <- jsonData:
	default:
	}
}

func (h *Hub) GetRoomClients(roomCode string) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var clients []*Client
	for client := range h.rooms[roomCode] {
		clients = append(clients, client)
	}
	return clients
}

func (h *Hub) GetRoomClientCount(roomCode string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.rooms[roomCode])
}

func (h *Hub) IsUserInRoom(roomCode string, userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.rooms[roomCode] {
		if client.UserID == userID {
			return true
		}
	}
	return false
}

func (h *Hub) GetClientByUserID(roomCode string, userID int64) *Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.rooms[roomCode] {
		if client.UserID == userID {
			return client
		}
	}
	return nil
}

func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

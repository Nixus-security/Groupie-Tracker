package websocket

import (
	"encoding/json"
	"log"
	"sync"

	"groupie-tracker/internal/models"
)

type Hub struct {
	rooms map[string]map[int64]*Client

	register   chan *Client
	unregister chan *Client
	broadcast  chan *BroadcastMessage

	mutex sync.RWMutex
}

type BroadcastMessage struct {
	RoomCode string
	Message  *models.WSMessage
	Exclude  int64
}

var (
	hubInstance *Hub
	hubOnce     sync.Once
)

func GetHub() *Hub {
	hubOnce.Do(func() {
		hubInstance = &Hub{
			rooms:      make(map[string]map[int64]*Client),
			register:   make(chan *Client),
			unregister: make(chan *Client),
			broadcast:  make(chan *BroadcastMessage, 256),
		}
		go hubInstance.run()
		log.Println("[Hub] ✅ WebSocket Hub démarré")
	})
	return hubInstance
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case msg := <-h.broadcast:
			h.broadcastToRoom(msg)
		}
	}
}

func (h *Hub) registerClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if _, exists := h.rooms[client.RoomCode]; !exists {
		h.rooms[client.RoomCode] = make(map[int64]*Client)
	}

	if oldClient, exists := h.rooms[client.RoomCode][client.UserID]; exists {
		log.Printf("[Hub] ⚠️ Remplacement connexion existante pour User %d", client.UserID)
		oldClient.Close()
	}

	h.rooms[client.RoomCode][client.UserID] = client
	log.Printf("[Hub] 🔌 Client connecté: User %d (%s) dans salle %s (total: %d)",
		client.UserID, client.Pseudo, client.RoomCode, len(h.rooms[client.RoomCode]))
}

func (h *Hub) unregisterClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if room, exists := h.rooms[client.RoomCode]; exists {
		if _, exists := room[client.UserID]; exists {
			delete(room, client.UserID)
			client.Close()
			log.Printf("[Hub] 🔌 Client déconnecté: User %d (%s) de salle %s (restant: %d)",
				client.UserID, client.Pseudo, client.RoomCode, len(room))

			if len(room) == 0 {
				delete(h.rooms, client.RoomCode)
				log.Printf("[Hub] 🗑️ Salle %s supprimée (vide)", client.RoomCode)
			}
		}
	}
}

func (h *Hub) broadcastToRoom(msg *BroadcastMessage) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	room, exists := h.rooms[msg.RoomCode]
	if !exists {
		log.Printf("[Hub] ⚠️ Broadcast: salle %s non trouvée", msg.RoomCode)
		return
	}

	data, err := json.Marshal(msg.Message)
	if err != nil {
		log.Printf("[Hub] ❌ Erreur marshal message: %v", err)
		return
	}

	recipientCount := len(room)
	if msg.Exclude != 0 {
		recipientCount--
	}
	log.Printf("[Hub] 📤 Broadcast: type=%s, room=%s, recipients=%d, exclude=%d",
		msg.Message.Type, msg.RoomCode, recipientCount, msg.Exclude)

	for userID, client := range room {
		if msg.Exclude != 0 && userID == msg.Exclude {
			continue
		}

		select {
		case client.send <- data:
		default:
			log.Printf("[Hub] ⚠️ Buffer plein pour User %d, déconnexion", userID)
			h.unregister <- client
		}
	}
}

func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

func (h *Hub) Broadcast(roomCode string, msg *models.WSMessage) {
	h.broadcast <- &BroadcastMessage{
		RoomCode: roomCode,
		Message:  msg,
		Exclude:  0,
	}
}

func (h *Hub) BroadcastExcept(roomCode string, msg *models.WSMessage, excludeUserID int64) {
	h.broadcast <- &BroadcastMessage{
		RoomCode: roomCode,
		Message:  msg,
		Exclude:  excludeUserID,
	}
}

func (h *Hub) SendToUser(roomCode string, userID int64, msg *models.WSMessage) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	room, exists := h.rooms[roomCode]
	if !exists {
		log.Printf("[Hub] ⚠️ SendToUser: salle %s non trouvée", roomCode)
		return
	}

	client, exists := room[userID]
	if !exists {
		log.Printf("[Hub] ⚠️ SendToUser: User %d non trouvé dans salle %s", userID, roomCode)
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[Hub] ❌ Erreur marshal message: %v", err)
		return
	}

	log.Printf("[Hub] 📤 SendToUser: type=%s, user=%d, room=%s", msg.Type, userID, roomCode)

	select {
	case client.send <- data:
	default:
		log.Printf("[Hub] ⚠️ Buffer plein pour User %d", userID)
	}
}

func (h *Hub) GetRoomClients(roomCode string) int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if room, exists := h.rooms[roomCode]; exists {
		return len(room)
	}
	return 0
}

func (h *Hub) GetConnectedUsers(roomCode string) []int64 {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	room, exists := h.rooms[roomCode]
	if !exists {
		return nil
	}

	users := make([]int64, 0, len(room))
	for userID := range room {
		users = append(users, userID)
	}
	return users
}

func (h *Hub) IsUserConnected(roomCode string, userID int64) bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	room, exists := h.rooms[roomCode]
	if !exists {
		return false
	}

	_, connected := room[userID]
	return connected
}
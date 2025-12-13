// Package websocket gère les connexions WebSocket temps réel
package websocket

import (
	"encoding/json"
	"log"
	"sync"

	"groupie-tracker/internal/models"
)

// Hub gère toutes les connexions WebSocket
type Hub struct {
	// Clients connectés par salle: roomCode -> userID -> Client
	rooms map[string]map[int64]*Client

	// Canal pour enregistrer un nouveau client
	register chan *Client

	// Canal pour désenregistrer un client
	unregister chan *Client

	// Canal pour diffuser un message à une salle
	broadcast chan *BroadcastMessage

	// Mutex pour l'accès concurrent
	mutex sync.RWMutex
}

// BroadcastMessage message à diffuser
type BroadcastMessage struct {
	RoomCode string
	Message  *models.WSMessage
	Exclude  int64 // UserID à exclure (0 = aucun)
}

// hubInstance singleton du hub
var (
	hubInstance *Hub
	hubOnce     sync.Once
)

// GetHub retourne l'instance singleton du hub
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

// run démarre la boucle principale du hub
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

// registerClient enregistre un nouveau client
func (h *Hub) registerClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Créer la salle si elle n'existe pas
	if _, exists := h.rooms[client.RoomCode]; !exists {
		h.rooms[client.RoomCode] = make(map[int64]*Client)
	}

	// Fermer l'ancienne connexion si elle existe
	if oldClient, exists := h.rooms[client.RoomCode][client.UserID]; exists {
		log.Printf("[Hub] ⚠️ Remplacement connexion existante pour User %d", client.UserID)
		oldClient.Close()
	}

	h.rooms[client.RoomCode][client.UserID] = client
	log.Printf("[Hub] 🔌 Client connecté: User %d (%s) dans salle %s (total: %d)",
		client.UserID, client.Pseudo, client.RoomCode, len(h.rooms[client.RoomCode]))
}

// unregisterClient désenregistre un client
func (h *Hub) unregisterClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if room, exists := h.rooms[client.RoomCode]; exists {
		if _, exists := room[client.UserID]; exists {
			delete(room, client.UserID)
			client.Close()
			log.Printf("[Hub] 🔌 Client déconnecté: User %d (%s) de salle %s (restant: %d)",
				client.UserID, client.Pseudo, client.RoomCode, len(room))

			// Supprimer la salle si vide
			if len(room) == 0 {
				delete(h.rooms, client.RoomCode)
				log.Printf("[Hub] 🗑️ Salle %s supprimée (vide)", client.RoomCode)
			}
		}
	}
}

// broadcastToRoom diffuse un message à tous les clients d'une salle
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

	// Log du broadcast
	recipientCount := len(room)
	if msg.Exclude != 0 {
		recipientCount--
	}
	log.Printf("[Hub] 📤 Broadcast: type=%s, room=%s, recipients=%d, exclude=%d",
		msg.Message.Type, msg.RoomCode, recipientCount, msg.Exclude)

	for userID, client := range room {
		// Exclure l'utilisateur spécifié si nécessaire
		if msg.Exclude != 0 && userID == msg.Exclude {
			continue
		}

		select {
		case client.send <- data:
			// Message envoyé
		default:
			// Buffer plein, fermer le client
			log.Printf("[Hub] ⚠️ Buffer plein pour User %d, déconnexion", userID)
			h.unregister <- client
		}
	}
}

// ============================================================================
// MÉTHODES PUBLIQUES
// ============================================================================

// Register enregistre un client
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister désenregistre un client
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Broadcast diffuse un message à une salle
func (h *Hub) Broadcast(roomCode string, msg *models.WSMessage) {
	h.broadcast <- &BroadcastMessage{
		RoomCode: roomCode,
		Message:  msg,
		Exclude:  0,
	}
}

// BroadcastExcept diffuse un message à une salle sauf à un utilisateur
func (h *Hub) BroadcastExcept(roomCode string, msg *models.WSMessage, excludeUserID int64) {
	h.broadcast <- &BroadcastMessage{
		RoomCode: roomCode,
		Message:  msg,
		Exclude:  excludeUserID,
	}
}

// SendToUser envoie un message à un utilisateur spécifique
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
		// OK
	default:
		log.Printf("[Hub] ⚠️ Buffer plein pour User %d", userID)
	}
}

// GetRoomClients retourne le nombre de clients dans une salle
func (h *Hub) GetRoomClients(roomCode string) int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if room, exists := h.rooms[roomCode]; exists {
		return len(room)
	}
	return 0
}

// GetConnectedUsers retourne les IDs des utilisateurs connectés dans une salle
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

// IsUserConnected vérifie si un utilisateur est connecté dans une salle
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
// Package websocket - handler.go
// Gère les connexions WebSocket et le routage des messages
package websocket

import (
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"groupie-tracker/internal/auth"
	"groupie-tracker/internal/models"
	"groupie-tracker/internal/rooms"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// BlindTestStarter interface pour démarrer une partie Blind Test
type BlindTestStarter interface {
	StartGame(roomCode string, genre string, rounds int) error
	HandleMessage(client *Client, msg *models.WSMessage)
}

// PetitBacStarter interface pour démarrer une partie Petit Bac
type PetitBacStarter interface {
	StartGame(roomCode string, categories []string, rounds int) error
	HandleMessage(client *Client, msg *models.WSMessage)
}

// Handler gère les connexions WebSocket
type Handler struct {
	hub         *Hub
	roomManager *rooms.Manager
	
	blindTestHandler BlindTestStarter
	petitBacHandler  PetitBacStarter
}

// NewHandler crée un nouveau handler WebSocket
func NewHandler() *Handler {
	return &Handler{
		hub:         GetHub(),
		roomManager: rooms.GetManager(),
	}
}

// SetBlindTestHandler définit le handler Blind Test
func (h *Handler) SetBlindTestHandler(handler BlindTestStarter) {
	h.blindTestHandler = handler
	log.Println("[WebSocket] Handler Blind Test configuré")
}

// SetPetitBacHandler définit le handler Petit Bac
func (h *Handler) SetPetitBacHandler(handler PetitBacStarter) {
	h.petitBacHandler = handler
	log.Println("[WebSocket] Handler Petit Bac configuré")
}

// HandleWebSocket gère les nouvelles connexions WebSocket
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	roomCode := r.URL.Query().Get("room")
	if roomCode == "" {
		path := strings.TrimPrefix(r.URL.Path, "/ws/room/")
		roomCode = strings.TrimSuffix(path, "/")
	}
	
	if roomCode == "" {
		http.Error(w, "Code de salle manquant", http.StatusBadRequest)
		return
	}

	room, err := h.roomManager.GetRoomByCode(roomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(roomCode)
		if err != nil {
			http.Error(w, "Salle non trouvée", http.StatusNotFound)
			return
		}
	}

	room.Mutex.RLock()
	_, isInRoom := room.Players[user.ID]
	room.Mutex.RUnlock()

	if !isInRoom {
		http.Error(w, "Vous n'êtes pas dans cette salle", http.StatusForbidden)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ Erreur upgrade WebSocket: %v", err)
		return
	}

	client := NewClient(h.hub, conn, user.ID, user.Pseudo, room.Code, h.handleMessage)

	h.hub.Register(client)

	room.Mutex.Lock()
	if player, exists := room.Players[user.ID]; exists {
		player.Connected = true
	}
	room.Mutex.Unlock()

	h.hub.BroadcastExcept(room.Code, &models.WSMessage{
		Type: models.WSTypePlayerJoined,
		Payload: map[string]interface{}{
			"user_id": user.ID,
			"pseudo":  user.Pseudo,
		},
	}, user.ID)

	h.sendRoomState(client, room)

	client.Start()
}

// handleMessage traite les messages reçus des clients
func (h *Handler) handleMessage(client *Client, msg *models.WSMessage) {
	room, err := h.roomManager.GetRoomByCode(client.RoomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(client.RoomCode)
		if err != nil {
			client.SendError("Salle non trouvée")
			return
		}
	}

	switch msg.Type {
	// Messages de salle
	case models.WSTypePlayerReady:
		h.handlePlayerReady(client, room, msg)
	case models.WSTypeLeaveRoom:
		h.handleLeaveRoom(client, room)
	case models.WSTypeStartGame:
		h.handleStartGame(client, room, msg)

	// Messages Blind Test
	case models.WSTypeBTAnswer:
		if h.blindTestHandler != nil {
			h.blindTestHandler.HandleMessage(client, msg)
		} else {
			client.SendError("Handler Blind Test non configuré")
		}

	// Messages Petit Bac
	case "submit_answers", "stop_round", "submit_votes":
		if h.petitBacHandler != nil {
			h.petitBacHandler.HandleMessage(client, msg)
		} else {
			client.SendError("Handler Petit Bac non configuré")
		}

	default:
		log.Printf("[WebSocket] Message non géré: %s", msg.Type)
	}
}

// handlePlayerReady gère le changement d'état "prêt"
func (h *Handler) handlePlayerReady(client *Client, room *models.Room, msg *models.WSMessage) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		client.SendError("Payload invalide")
		return
	}

	ready, _ := payload["ready"].(bool)

	err := h.roomManager.SetPlayerReady(room.ID, client.UserID, ready)
	if err != nil {
		client.SendError(err.Error())
		return
	}

	h.hub.Broadcast(room.Code, &models.WSMessage{
		Type: models.WSTypePlayerReady,
		Payload: map[string]interface{}{
			"user_id": client.UserID,
			"pseudo":  client.Pseudo,
			"ready":   ready,
		},
	})

	if models.IsRoomReady(room) {
		h.hub.Broadcast(room.Code, &models.WSMessage{
			Type: models.WSTypeRoomUpdate,
			Payload: map[string]interface{}{
				"is_ready": true,
			},
		})
	}
}

// handleLeaveRoom gère le départ d'un joueur
func (h *Handler) handleLeaveRoom(client *Client, room *models.Room) {
	err := h.roomManager.LeaveRoom(room.ID, client.UserID)
	if err != nil {
		client.SendError(err.Error())
		return
	}

	h.hub.BroadcastExcept(room.Code, &models.WSMessage{
		Type: models.WSTypePlayerLeft,
		Payload: map[string]interface{}{
			"user_id": client.UserID,
			"pseudo":  client.Pseudo,
		},
	}, client.UserID)

	h.hub.Unregister(client)
}

// handleStartGame gère le démarrage d'une partie
func (h *Handler) handleStartGame(client *Client, room *models.Room, msg *models.WSMessage) {
	if room.HostID != client.UserID {
		client.SendError("Seul l'hôte peut démarrer la partie")
		return
	}

	// Permettre le mode solo OU vérifier que tous sont prêts
	room.Mutex.RLock()
	playerCount := len(room.Players)
	room.Mutex.RUnlock()

	if playerCount > 1 && !models.IsRoomReady(room) {
		client.SendError("Tous les joueurs ne sont pas prêts")
		return
	}

	switch room.GameType {
	case models.GameTypeBlindTest:
		if h.blindTestHandler == nil {
			client.SendError("Handler Blind Test non configuré")
			return
		}

		genre := room.Config.Playlist
		if genre == "" {
			genre = "Pop"
		}
		rounds := 10

		err := h.blindTestHandler.StartGame(room.Code, genre, rounds)
		if err != nil {
			client.SendError("Impossible de démarrer: " + err.Error())
			return
		}

		h.roomManager.StartGame(room.ID)

		h.hub.Broadcast(room.Code, &models.WSMessage{
			Type: models.WSTypeStartGame,
			Payload: map[string]interface{}{
				"game_type": "blindtest",
				"genre":     genre,
				"rounds":    rounds,
			},
		})

	case models.GameTypePetitBac:
		if h.petitBacHandler == nil {
			client.SendError("Handler Petit Bac non configuré")
			return
		}

		categories := room.Config.Categories
		if len(categories) == 0 {
			categories = models.DefaultPetitBacCategories
		}
		rounds := room.Config.NbRounds
		if rounds <= 0 {
			rounds = models.NbrsManche
		}

		err := h.petitBacHandler.StartGame(room.Code, categories, rounds)
		if err != nil {
			client.SendError("Impossible de démarrer: " + err.Error())
			return
		}

		h.roomManager.StartGame(room.ID)

	default:
		client.SendError("Type de jeu inconnu")
	}
}

// sendRoomState envoie l'état actuel de la salle à un client
func (h *Handler) sendRoomState(client *Client, room *models.Room) {
	room.Mutex.RLock()
	defer room.Mutex.RUnlock()

	players := make([]map[string]interface{}, 0, len(room.Players))
	for _, p := range room.Players {
		players = append(players, map[string]interface{}{
			"user_id":   p.UserID,
			"pseudo":    p.Pseudo,
			"score":     p.Score,
			"is_host":   p.IsHost,
			"is_ready":  p.IsReady,
			"connected": p.Connected,
		})
	}

	client.Send(&models.WSMessage{
		Type: models.WSTypeRoomUpdate,
		Payload: map[string]interface{}{
			"room_id":   room.ID,
			"code":      room.Code,
			"name":      room.Name,
			"host_id":   room.HostID,
			"game_type": room.GameType,
			"status":    room.Status,
			"players":   players,
			"config":    room.Config,
			"is_ready":  models.IsRoomReady(room),
		},
	})
}

// GetHub retourne le hub pour utilisation externe
func (h *Handler) GetHub() *Hub {
	return h.hub
}
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

type BlindTestStarter interface {
	StartGame(roomCode string, genre string, rounds int) error
	HandleMessage(client *Client, msg *models.WSMessage)
}

type PetitBacStarter interface {
	StartGame(roomCode string, categories []string, rounds int) error
	HandleMessage(client *Client, msg *models.WSMessage)
}

type Handler struct {
	hub         *Hub
	roomManager *rooms.Manager

	blindTestHandler BlindTestStarter
	petitBacHandler  PetitBacStarter
}

func NewHandler() *Handler {
	return &Handler{
		hub:         GetHub(),
		roomManager: rooms.GetManager(),
	}
}

func (h *Handler) SetBlindTestHandler(handler BlindTestStarter) {
	h.blindTestHandler = handler
	log.Println("[WebSocket] ✅ Handler Blind Test configuré")
}

func (h *Handler) SetPetitBacHandler(handler PetitBacStarter) {
	h.petitBacHandler = handler
	log.Println("[WebSocket] ✅ Handler Petit Bac configuré")
}

func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		log.Println("[WebSocket] ❌ Utilisateur non authentifié")
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	roomCode := r.URL.Query().Get("room")
	if roomCode == "" {
		path := strings.TrimPrefix(r.URL.Path, "/ws/room/")
		roomCode = strings.TrimSuffix(path, "/")
	}

	if roomCode == "" {
		log.Println("[WebSocket] ❌ Code de salle manquant")
		http.Error(w, "Code de salle manquant", http.StatusBadRequest)
		return
	}

	room, err := h.roomManager.GetRoomByCode(roomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(roomCode)
		if err != nil {
			log.Printf("[WebSocket] ❌ Salle non trouvée: %s", roomCode)
			http.Error(w, "Salle non trouvée", http.StatusNotFound)
			return
		}
	}

	room.Mutex.RLock()
	_, isInRoom := room.Players[user.ID]
	room.Mutex.RUnlock()

	if !isInRoom {
		log.Printf("[WebSocket] ❌ User %d pas dans la salle %s", user.ID, roomCode)
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

	log.Printf("[WebSocket] ✅ Client connecté: User %d (%s) dans salle %s", user.ID, user.Pseudo, room.Code)

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

func (h *Handler) handleMessage(client *Client, msg *models.WSMessage) {
	log.Printf("[WebSocket] 📨 Message: type=%s, user=%d (%s), room=%s",
		msg.Type, client.UserID, client.Pseudo, client.RoomCode)

	room, err := h.roomManager.GetRoomByCode(client.RoomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(client.RoomCode)
		if err != nil {
			client.SendError("Salle non trouvée")
			return
		}
	}

	switch msg.Type {
	case models.WSTypePlayerReady:
		h.handlePlayerReady(client, room, msg)

	case models.WSTypeLeaveRoom:
		h.handleLeaveRoom(client, room)

	case models.WSTypeStartGame:
		h.handleStartGame(client, room, msg)

	case models.WSTypeBTAnswer:
		if h.blindTestHandler != nil {
			h.blindTestHandler.HandleMessage(client, msg)
		} else {
			log.Println("[WebSocket] ⚠️ Handler Blind Test non configuré")
			client.SendError("Handler Blind Test non configuré")
		}

	case models.WSTypePBSubmitAnswers, models.WSTypePBStopRound, models.WSTypePBSubmitVotes:
		if h.petitBacHandler != nil {
			h.petitBacHandler.HandleMessage(client, msg)
		} else {
			log.Println("[WebSocket] ⚠️ Handler Petit Bac non configuré")
			client.SendError("Handler Petit Bac non configuré")
		}

	default:
		log.Printf("[WebSocket] ⚠️ Message non géré: %s", msg.Type)
	}
}

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

	log.Printf("[WebSocket] 👤 Player %d (%s) ready=%v", client.UserID, client.Pseudo, ready)

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

func (h *Handler) handleLeaveRoom(client *Client, room *models.Room) {
	err := h.roomManager.LeaveRoom(room.ID, client.UserID)
	if err != nil {
		client.SendError(err.Error())
		return
	}

	log.Printf("[WebSocket] 👋 Player %d (%s) quitte la salle %s", client.UserID, client.Pseudo, room.Code)

	h.hub.BroadcastExcept(room.Code, &models.WSMessage{
		Type: models.WSTypePlayerLeft,
		Payload: map[string]interface{}{
			"user_id": client.UserID,
			"pseudo":  client.Pseudo,
		},
	}, client.UserID)

	h.hub.Unregister(client)
}

func (h *Handler) handleStartGame(client *Client, room *models.Room, msg *models.WSMessage) {
	log.Printf("[WebSocket] 🎮 Demande start_game de %d (%s) pour salle %s", client.UserID, client.Pseudo, room.Code)

	if room.HostID != client.UserID {
		client.SendError("Seul l'hôte peut démarrer la partie")
		return
	}

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

		if payload, ok := msg.Payload.(map[string]interface{}); ok {
			if g, ok := payload["genre"].(string); ok && g != "" {
				genre = g
			}
			if r, ok := payload["rounds"].(float64); ok && r > 0 {
				rounds = int(r)
			}
		}

		err := h.blindTestHandler.StartGame(room.Code, genre, rounds)
		if err != nil {
			log.Printf("[WebSocket] ❌ Erreur démarrage BlindTest: %v", err)
			client.SendError("Impossible de démarrer: " + err.Error())
			return
		}

		h.roomManager.StartGame(room.ID)

		log.Printf("[WebSocket] ✅ BlindTest démarré: genre=%s, rounds=%d", genre, rounds)

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
			log.Printf("[WebSocket] ❌ Erreur démarrage PetitBac: %v", err)
			client.SendError("Impossible de démarrer: " + err.Error())
			return
		}

		h.roomManager.StartGame(room.ID)

		log.Printf("[WebSocket] ✅ PetitBac démarré: categories=%v, rounds=%d", categories, rounds)

	default:
		client.SendError("Type de jeu inconnu: " + string(room.GameType))
	}
}

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

func (h *Handler) GetHub() *Hub {
	return h.hub
}
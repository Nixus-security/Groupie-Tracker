// Package websocket - handler.go
// Gère les connexions WebSocket et le routage des messages
package websocket

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"groupie-tracker/internal/auth"
	"groupie-tracker/internal/models"
	"groupie-tracker/internal/rooms"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// En production, vérifier l'origine
		return true
	},
}

// Handler gère les connexions WebSocket
type Handler struct {
	hub         *Hub
	roomManager *rooms.Manager
	
	// Handlers de jeu (injectés)
	blindTestHandler func(*Client, *models.WSMessage)
	petitBacHandler  func(*Client, *models.WSMessage)
}

// NewHandler crée un nouveau handler WebSocket
func NewHandler() *Handler {
	return &Handler{
		hub:         GetHub(),
		roomManager: rooms.GetManager(),
	}
}

// SetBlindTestHandler définit le handler Blind Test
func (h *Handler) SetBlindTestHandler(handler func(*Client, *models.WSMessage)) {
	h.blindTestHandler = handler
}

// SetPetitBacHandler définit le handler Petit Bac
func (h *Handler) SetPetitBacHandler(handler func(*Client, *models.WSMessage)) {
	h.petitBacHandler = handler
}

// RegisterRoutes enregistre la route WebSocket
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware) {
	// La route WebSocket nécessite une authentification
	mux.Handle("/ws", authMiddleware.RequireAuth(http.HandlerFunc(h.HandleWebSocket)))
}

// HandleWebSocket gère les nouvelles connexions WebSocket
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Récupérer l'utilisateur depuis le contexte
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Récupérer le code de la salle depuis les paramètres
	roomCode := r.URL.Query().Get("room")
	if roomCode == "" {
		http.Error(w, "Code de salle manquant", http.StatusBadRequest)
		return
	}

	// Vérifier que la salle existe
	room, err := h.roomManager.GetRoom(roomCode)
	if err != nil {
		http.Error(w, "Salle non trouvée", http.StatusNotFound)
		return
	}

	// Vérifier que l'utilisateur est dans la salle
	room.Mutex.RLock()
	_, isInRoom := room.Players[user.ID]
	room.Mutex.RUnlock()

	if !isInRoom {
		http.Error(w, "Vous n'êtes pas dans cette salle", http.StatusForbidden)
		return
	}

	// Upgrader la connexion HTTP vers WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ Erreur upgrade WebSocket: %v", err)
		return
	}

	// Créer le client
	client := NewClient(h.hub, conn, user.ID, user.Pseudo, roomCode, h.handleMessage)

	// Enregistrer le client
	h.hub.Register(client)

	// Mettre à jour le statut de connexion dans la salle
	room.Mutex.Lock()
	if player, exists := room.Players[user.ID]; exists {
		player.Connected = true
	}
	room.Mutex.Unlock()

	// Notifier les autres joueurs
	h.hub.BroadcastExcept(roomCode, &models.WSMessage{
		Type: models.WSTypePlayerJoined,
		Payload: map[string]interface{}{
			"user_id": user.ID,
			"pseudo":  user.Pseudo,
		},
	}, user.ID)

	// Envoyer l'état actuel de la salle au nouveau client
	h.sendRoomState(client, room)

	// Démarrer les pompes de lecture/écriture
	client.Start()
}

// handleMessage traite les messages reçus des clients
func (h *Handler) handleMessage(client *Client, msg *models.WSMessage) {
	room, err := h.roomManager.GetRoom(client.RoomCode)
	if err != nil {
		client.SendError("Salle non trouvée")
		return
	}

	switch msg.Type {
	// Messages de salle
	case models.WSTypePlayerReady:
		h.handlePlayerReady(client, room, msg)
	case models.WSTypeLeaveRoom:
		h.handleLeaveRoom(client, room)
	case models.WSTypeStartGame:
		h.handleStartGame(client, room)

	// Messages Blind Test
	case models.WSTypeBTAnswer:
		if h.blindTestHandler != nil {
			h.blindTestHandler(client, msg)
		}

	// Messages Petit Bac
	case models.WSTypePBAnswer, models.WSTypePBVote, models.WSTypePBStopRound:
		if h.petitBacHandler != nil {
			h.petitBacHandler(client, msg)
		}

	default:
		client.SendError("Type de message inconnu: " + string(msg.Type))
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

	err := h.roomManager.SetPlayerReady(client.RoomCode, client.UserID, ready)
	if err != nil {
		client.SendError(err.Error())
		return
	}

	// Notifier tous les joueurs
	h.hub.Broadcast(client.RoomCode, &models.WSMessage{
		Type: models.WSTypePlayerReady,
		Payload: map[string]interface{}{
			"user_id": client.UserID,
			"pseudo":  client.Pseudo,
			"ready":   ready,
		},
	})

	// Vérifier si la salle est prête
	if models.IsRoomReady(room) {
		h.hub.Broadcast(client.RoomCode, &models.WSMessage{
			Type: models.WSTypeRoomUpdate,
			Payload: map[string]interface{}{
				"is_ready": true,
			},
		})
	}
}

// handleLeaveRoom gère le départ d'un joueur
func (h *Handler) handleLeaveRoom(client *Client, _ *models.Room) {
	err := h.roomManager.LeaveRoom(client.RoomCode, client.UserID)
	if err != nil {
		client.SendError(err.Error())
		return
	}

	// Notifier les autres joueurs
	h.hub.BroadcastExcept(client.RoomCode, &models.WSMessage{
		Type: models.WSTypePlayerLeft,
		Payload: map[string]interface{}{
			"user_id": client.UserID,
			"pseudo":  client.Pseudo,
		},
	}, client.UserID)

	// Fermer la connexion
	h.hub.Unregister(client)
}

// handleStartGame gère le démarrage d'une partie
func (h *Handler) handleStartGame(client *Client, room *models.Room) {
	// Vérifier que c'est l'hôte
	if room.HostID != client.UserID {
		client.SendError("Seul l'hôte peut démarrer la partie")
		return
	}

	// Vérifier que la salle est prête
	if !models.IsRoomReady(room) {
		client.SendError("Tous les joueurs ne sont pas prêts")
		return
	}

	// Démarrer la partie
	err := h.roomManager.StartGame(client.RoomCode, client.UserID)
	if err != nil {
		client.SendError(err.Error())
		return
	}

	// Notifier tous les joueurs
	h.hub.Broadcast(client.RoomCode, &models.WSMessage{
		Type: models.WSTypeStartGame,
		Payload: map[string]interface{}{
			"game_type": room.GameType,
			"config":    room.Config,
		},
	})
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
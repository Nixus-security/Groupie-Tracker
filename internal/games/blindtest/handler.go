// Package blindtest gère la logique du jeu Blind Test
package blindtest

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"groupie-tracker/internal/models"
	"groupie-tracker/internal/rooms"
	"groupie-tracker/internal/websocket"
)

// Handler gère les messages WebSocket pour le Blind Test
type Handler struct {
	gameManager *GameManager
	roomManager *rooms.Manager
	hub         *websocket.Hub
	timers      map[string]*time.Timer // roomID -> timer
	timerMutex  sync.Mutex
}

var (
	handlerInstance *Handler
	handlerOnce     sync.Once
)

// GetHandler retourne l'instance singleton du handler
func GetHandler() *Handler {
	handlerOnce.Do(func() {
		handlerInstance = &Handler{
			gameManager: GetGameManager(),
			roomManager: rooms.GetManager(),
			hub:         websocket.GetHub(),
			timers:      make(map[string]*time.Timer),
		}
	})
	return handlerInstance
}

// HandleMessage traite les messages WebSocket du Blind Test
// Cette fonction est appelée par le handler WebSocket principal
func (h *Handler) HandleMessage(client *websocket.Client, msg *models.WSMessage) {
	switch msg.Type {
	case models.WSTypeBTAnswer:
		h.handleAnswer(client, msg)
	default:
		log.Printf("[BlindTest] Message non géré: %s", msg.Type)
	}
}

// StartGame démarre une partie de Blind Test (appelé par le handler WebSocket principal)
func (h *Handler) StartGame(roomCode string, genre string, rounds int) error {
	room, err := h.roomManager.GetRoomByCode(roomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(roomCode)
		if err != nil {
			return err
		}
	}

	// Démarrer la partie
	_, err = h.gameManager.StartGame(room.ID, genre, rounds)
	if err != nil {
		return err
	}

	log.Printf("[BlindTest] Partie démarrée dans la salle %s (genre: %s, manches: %d)", roomCode, genre, rounds)

	// Lancer la première manche après un court délai
	go func() {
		time.Sleep(2 * time.Second)
		h.startNextRound(room.ID, roomCode)
	}()

	return nil
}

// startNextRound démarre la prochaine manche
func (h *Handler) startNextRound(roomID, roomCode string) {
	roundInfo, err := h.gameManager.NextRound(roomID)
	if err != nil {
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type:  models.WSTypeError,
			Error: err.Error(),
		})
		return
	}

	// Jeu terminé ?
	if roundInfo == nil {
		h.endGame(roomID, roomCode)
		return
	}

	log.Printf("[BlindTest] Manche %d/%d - URL: %s", roundInfo.Round, roundInfo.Total, roundInfo.PreviewURL)

	// Envoyer les infos de la manche à tous les joueurs
	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type:    models.WSTypeBTNewRound,
		Payload: roundInfo,
	})

	// Démarrer le timer
	go h.runRoundTimer(roomID, roomCode, roundInfo.Duration)
}

// runRoundTimer gère le timer d'une manche
func (h *Handler) runRoundTimer(roomID, roomCode string, duration int) {
	state := h.gameManager.GetGameState(roomID)
	if state == nil {
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for i := duration; i >= 0; i-- {
		state.Mutex.Lock()
		state.TimeLeft = i
		state.Mutex.Unlock()

		// Envoyer le temps restant toutes les secondes
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type: "time_update",
			Payload: map[string]int{
				"time_left": i,
			},
		})

		if i > 0 {
			<-ticker.C
		}

		// Vérifier si le jeu existe toujours
		if h.gameManager.GetGameState(roomID) == nil {
			return
		}
	}

	// Temps écoulé - révéler la réponse
	h.revealAndContinue(roomID, roomCode)
}

// handleAnswer traite une réponse d'un joueur
func (h *Handler) handleAnswer(client *websocket.Client, msg *models.WSMessage) {
	// Parser le payload
	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		client.SendError("Payload invalide")
		return
	}

	var answer struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal(payloadBytes, &answer); err != nil {
		client.SendError("Format de réponse invalide")
		return
	}

	// Récupérer la salle
	room, err := h.roomManager.GetRoomByCode(client.RoomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(client.RoomCode)
		if err != nil {
			client.SendError("Salle non trouvée")
			return
		}
	}

	// Soumettre la réponse
	result, err := h.gameManager.SubmitAnswer(room.ID, client.UserID, answer.Answer)
	if err != nil {
		client.SendError(err.Error())
		return
	}

	// Envoyer le résultat au joueur
	client.Send(&models.WSMessage{
		Type:    models.WSTypeBTResult,
		Payload: result,
	})

	// Si la réponse est correcte, notifier tout le monde
	if result.IsCorrect && !result.AlreadyAnswered {
		h.hub.Broadcast(client.RoomCode, &models.WSMessage{
			Type: "player_found",
			Payload: map[string]interface{}{
				"user_id": client.UserID,
				"pseudo":  client.Pseudo,
				"points":  result.Points,
			},
		})

		// Mettre à jour les scores
		h.broadcastScores(room.ID, client.RoomCode)
	}
}

// broadcastScores envoie les scores à tous les joueurs
func (h *Handler) broadcastScores(roomID, roomCode string) {
	scores := h.gameManager.GetScores(roomID)
	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type:    models.WSTypeBTScores,
		Payload: scores,
	})
}

// revealAndContinue révèle la réponse et passe à la manche suivante
func (h *Handler) revealAndContinue(roomID, roomCode string) {
	// Révéler la réponse
	revealInfo := h.gameManager.RevealAnswer(roomID)
	if revealInfo != nil {
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type:    "bt_reveal",
			Payload: revealInfo,
		})
	}

	// Envoyer les scores actuels
	h.broadcastScores(roomID, roomCode)

	// Attendre avant la prochaine manche
	time.Sleep(5 * time.Second)

	// Vérifier si le jeu est terminé
	if h.gameManager.IsGameOver(roomID) {
		h.endGame(roomID, roomCode)
	} else {
		h.startNextRound(roomID, roomCode)
	}
}

// endGame termine la partie
func (h *Handler) endGame(roomID, roomCode string) {
	result := h.gameManager.EndGame(roomID)
	if result == nil {
		return
	}

	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type:    models.WSTypeBTGameEnd,
		Payload: result,
	})

	log.Printf("[BlindTest] Partie terminée - Gagnant: %s", result.Winner)
}
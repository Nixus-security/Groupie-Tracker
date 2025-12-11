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
	stopTimers  map[string]chan bool
	mutex       sync.Mutex
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
			stopTimers:  make(map[string]chan bool),
		}
	})
	return handlerInstance
}

// HandleMessage traite les messages WebSocket du Blind Test
func (h *Handler) HandleMessage(client *websocket.Client, msg *models.WSMessage) {
	switch msg.Type {
	case models.WSTypeBTAnswer:
		h.handleAnswer(client, msg)
	default:
		log.Printf("[BlindTest] Message non géré: %s", msg.Type)
	}
}

// StartGame démarre une partie de Blind Test
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

	log.Printf("[BlindTest] ✅ Partie démarrée dans la salle %s (genre: %s, manches: %d)", roomCode, genre, rounds)

	// Créer le canal pour stopper le timer
	h.mutex.Lock()
	h.stopTimers[room.ID] = make(chan bool, 1)
	h.mutex.Unlock()

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
		log.Printf("[BlindTest] ❌ Erreur NextRound: %v", err)
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type:  models.WSTypeError,
			Error: err.Error(),
		})
		return
	}

	// Jeu terminé ?
	if roundInfo == nil {
		log.Printf("[BlindTest] 🏁 Jeu terminé pour salle %s", roomCode)
		h.endGame(roomID, roomCode)
		return
	}

	log.Printf("[BlindTest] 🎵 Manche %d/%d - Preview: %s", roundInfo.Round, roundInfo.Total, roundInfo.PreviewURL)

	// Envoyer les infos de la manche à tous les joueurs
	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type:    models.WSTypeBTNewRound,
		Payload: roundInfo,
	})

	// Recréer le canal stop pour cette manche
	h.mutex.Lock()
	if _, exists := h.stopTimers[roomID]; !exists {
		h.stopTimers[roomID] = make(chan bool, 1)
	}
	h.mutex.Unlock()

	// Démarrer le timer
	go h.runRoundTimer(roomID, roomCode, roundInfo.Duration)
}

// runRoundTimer gère le timer d'une manche
func (h *Handler) runRoundTimer(roomID, roomCode string, duration int) {
	state := h.gameManager.GetGameState(roomID)
	if state == nil {
		log.Printf("[BlindTest] ❌ État du jeu non trouvé pour %s", roomID)
		return
	}

	h.mutex.Lock()
	stopChan := h.stopTimers[roomID]
	h.mutex.Unlock()

	if stopChan == nil {
		log.Printf("[BlindTest] ❌ Stop channel non trouvé")
		return
	}

	log.Printf("[BlindTest] ⏱️ Timer démarré: %d secondes", duration)

	for i := duration; i >= 0; i-- {
		// Vérifier si on doit arrêter
		select {
		case <-stopChan:
			log.Printf("[BlindTest] ⏹️ Timer interrompu")
			// Recréer le canal pour la prochaine manche
			h.mutex.Lock()
			h.stopTimers[roomID] = make(chan bool, 1)
			h.mutex.Unlock()
			return
		default:
		}

		state.Mutex.Lock()
		state.TimeLeft = i
		state.Mutex.Unlock()

		// Envoyer le temps restant
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type: "time_update",
			Payload: map[string]int{
				"time_left": i,
			},
		})

		if i > 0 {
			time.Sleep(1 * time.Second)
		}

		// Vérifier si le jeu existe toujours
		if h.gameManager.GetGameState(roomID) == nil {
			log.Printf("[BlindTest] Jeu terminé pendant le timer")
			return
		}
	}

	// Temps écoulé
	log.Printf("[BlindTest] ⏰ Temps écoulé pour salle %s", roomCode)
	h.revealAndContinue(roomID, roomCode)
}

// handleAnswer traite une réponse d'un joueur
func (h *Handler) handleAnswer(client *websocket.Client, msg *models.WSMessage) {
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

	log.Printf("[BlindTest] 📝 Réponse de %s: %s", client.Pseudo, answer.Answer)

	room, err := h.roomManager.GetRoomByCode(client.RoomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(client.RoomCode)
		if err != nil {
			client.SendError("Salle non trouvée")
			return
		}
	}

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

	if result.IsCorrect && !result.AlreadyAnswered {
		log.Printf("[BlindTest] ✅ Bonne réponse de %s ! +%d points", client.Pseudo, result.Points)
		
		h.hub.Broadcast(client.RoomCode, &models.WSMessage{
			Type: "player_found",
			Payload: map[string]interface{}{
				"user_id": client.UserID,
				"pseudo":  client.Pseudo,
				"points":  result.Points,
			},
		})

		h.broadcastScores(room.ID, client.RoomCode)

		// Vérifier si tous les joueurs ont trouvé
		if h.allPlayersAnsweredCorrectly(room.ID) {
			log.Printf("[BlindTest] 🎉 Tous les joueurs ont trouvé !")
			
			h.mutex.Lock()
			if stopChan, exists := h.stopTimers[room.ID]; exists {
				select {
				case stopChan <- true:
				default:
				}
			}
			h.mutex.Unlock()

			go func() {
				time.Sleep(1 * time.Second)
				h.revealAndContinue(room.ID, client.RoomCode)
			}()
		}
	} else if !result.IsCorrect {
		log.Printf("[BlindTest] ❌ Mauvaise réponse de %s", client.Pseudo)
	}
}

// allPlayersAnsweredCorrectly vérifie si tous les joueurs ont répondu correctement
func (h *Handler) allPlayersAnsweredCorrectly(roomID string) bool {
	state := h.gameManager.GetGameState(roomID)
	if state == nil {
		return false
	}

	room, err := h.roomManager.GetRoom(roomID)
	if err != nil {
		return false
	}

	room.Mutex.RLock()
	playerCount := len(room.Players)
	room.Mutex.RUnlock()

	state.Mutex.RLock()
	answeredCount := len(state.HasAnswered)
	state.Mutex.RUnlock()

	// Tous les joueurs ont répondu (correctement ou non, mais le timer s'arrête quand tous ont tenté)
	return answeredCount >= playerCount && playerCount > 0
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
	revealInfo := h.gameManager.RevealAnswer(roomID)
	if revealInfo != nil {
		log.Printf("[BlindTest] 🔓 Révélation: %s - %s", revealInfo.TrackName, revealInfo.ArtistName)
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type:    "bt_reveal",
			Payload: revealInfo,
		})
	}

	h.broadcastScores(roomID, roomCode)

	// Attendre avant la prochaine manche
	time.Sleep(4 * time.Second)

	if h.gameManager.IsGameOver(roomID) {
		log.Printf("[BlindTest] 🏁 Partie terminée pour salle %s", roomCode)
		h.endGame(roomID, roomCode)
	} else {
		log.Printf("[BlindTest] ➡️ Passage à la manche suivante")
		h.startNextRound(roomID, roomCode)
	}
}

// endGame termine la partie
func (h *Handler) endGame(roomID, roomCode string) {
	h.mutex.Lock()
	if stopChan, exists := h.stopTimers[roomID]; exists {
		close(stopChan)
		delete(h.stopTimers, roomID)
	}
	h.mutex.Unlock()

	result := h.gameManager.EndGame(roomID)
	if result == nil {
		return
	}

	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type:    models.WSTypeBTGameEnd,
		Payload: result,
	})

	log.Printf("[BlindTest] 🏆 Partie terminée - Gagnant: %s", result.Winner)
}
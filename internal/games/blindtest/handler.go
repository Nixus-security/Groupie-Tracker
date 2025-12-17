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

type Handler struct {
	gameManager *GameManager
	roomManager *rooms.Manager
	hub         *websocket.Hub
	stopTimers  map[string]chan bool
	roundLocks  map[string]*sync.Mutex
	mutex       sync.Mutex
}

var (
	handlerInstance *Handler
	handlerOnce     sync.Once
)

func GetHandler() *Handler {
	handlerOnce.Do(func() {
		handlerInstance = &Handler{
			gameManager: GetGameManager(),
			roomManager: rooms.GetManager(),
			hub:         websocket.GetHub(),
			stopTimers:  make(map[string]chan bool),
			roundLocks:  make(map[string]*sync.Mutex),
		}
	})
	return handlerInstance
}

func (h *Handler) HandleMessage(client *websocket.Client, msg *models.WSMessage) {
	log.Printf("[BlindTest] 📨 Message reçu: type=%s, user=%d", msg.Type, client.UserID)

	switch msg.Type {
	case models.WSTypeBTAnswer:
		h.handleAnswer(client, msg)
	default:
		log.Printf("[BlindTest] ⚠️ Message non géré: %s", msg.Type)
	}
}

func (h *Handler) StartGame(roomCode string, genre string, rounds int) error {
	room, err := h.roomManager.GetRoomByCode(roomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(roomCode)
		if err != nil {
			return err
		}
	}

	_, err = h.gameManager.StartGame(room.ID, genre, rounds)
	if err != nil {
		return err
	}

	log.Printf("[BlindTest] ✅ Partie démarrée dans la salle %s (genre: %s, manches: %d)", roomCode, genre, rounds)

	h.mutex.Lock()
	h.stopTimers[room.ID] = make(chan bool, 1)
	h.roundLocks[room.ID] = &sync.Mutex{}
	h.mutex.Unlock()

	go func() {
		time.Sleep(2 * time.Second)
		h.startNextRound(room.ID, roomCode)
	}()

	return nil
}

func (h *Handler) startNextRound(roomID, roomCode string) {
	h.mutex.Lock()
	roundLock, exists := h.roundLocks[roomID]
	h.mutex.Unlock()

	if !exists {
		log.Printf("[BlindTest] ❌ Round lock non trouvé pour %s", roomID)
		return
	}

	roundLock.Lock()
	defer roundLock.Unlock()

	roundInfo, err := h.gameManager.NextRound(roomID)
	if err != nil {
		log.Printf("[BlindTest] ❌ Erreur NextRound: %v", err)
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type:  models.WSTypeError,
			Error: err.Error(),
		})
		return
	}

	if roundInfo == nil {
		log.Printf("[BlindTest] 🏁 Jeu terminé pour salle %s", roomCode)
		h.endGame(roomID, roomCode)
		return
	}

	log.Printf("[BlindTest] 🎵 Manche %d/%d - Preview: %s", roundInfo.Round, roundInfo.Total, roundInfo.PreviewURL)

	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type: models.WSTypeBTPreload,
		Payload: map[string]interface{}{
			"preview_url": roundInfo.PreviewURL,
			"round":       roundInfo.Round,
			"total":       roundInfo.Total,
		},
	})

	time.Sleep(1500 * time.Millisecond)

	h.mutex.Lock()
	if oldChan, exists := h.stopTimers[roomID]; exists {
		select {
		case <-oldChan:
		default:
		}
	}
	h.stopTimers[roomID] = make(chan bool, 1)
	h.mutex.Unlock()

	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type:    models.WSTypeBTNewRound,
		Payload: roundInfo,
	})

	go h.runRoundTimer(roomID, roomCode, roundInfo.Duration)
}

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

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeLeft := duration

	for timeLeft >= 0 {
		select {
		case <-stopChan:
			log.Printf("[BlindTest] ⏹️ Timer interrompu")
			return
		default:
		}

		state.Mutex.Lock()
		state.TimeLeft = timeLeft
		state.Mutex.Unlock()

		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type: models.WSTypeTimeUpdate,
			Payload: map[string]int{
				"time_left": timeLeft,
			},
		})

		if h.gameManager.GetGameState(roomID) == nil {
			log.Printf("[BlindTest] Jeu terminé pendant le timer")
			return
		}

		if timeLeft == 0 {
			break
		}

		select {
		case <-stopChan:
			log.Printf("[BlindTest] ⏹️ Timer interrompu pendant l'attente")
			return
		case <-ticker.C:
			timeLeft--
		}
	}

	select {
	case <-stopChan:
		log.Printf("[BlindTest] ⏹️ Timer déjà interrompu, on ne révèle pas")
		return
	default:
	}

	log.Printf("[BlindTest] ⏰ Temps écoulé pour salle %s", roomCode)
	h.revealAndContinue(roomID, roomCode)
}

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

	log.Printf("[BlindTest] 🔍 Réponse de %s: %s", client.Pseudo, answer.Answer)

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

	client.Send(&models.WSMessage{
		Type:    models.WSTypeBTResult,
		Payload: result,
	})

	if result.IsCorrect && !result.AlreadyAnswered {
		log.Printf("[BlindTest] ✅ Bonne réponse de %s ! +%d points", client.Pseudo, result.Points)

		h.hub.Broadcast(client.RoomCode, &models.WSMessage{
			Type: models.WSTypePlayerFound,
			Payload: map[string]interface{}{
				"user_id": client.UserID,
				"pseudo":  client.Pseudo,
				"points":  result.Points,
			},
		})

		h.broadcastScores(room.ID, client.RoomCode)

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
	correctCount := 0
	for userID := range state.HasAnswered {
		answer := state.Answers[userID]
		if checkAnswer(answer, state.CurrentTrack.Name, state.CurrentTrack.Artist) {
			correctCount++
		}
	}
	state.Mutex.RUnlock()

	return correctCount >= playerCount && playerCount > 0
}

func (h *Handler) broadcastScores(roomID, roomCode string) {
	scores := h.gameManager.GetScores(roomID)
	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type:    models.WSTypeBTScores,
		Payload: scores,
	})
}

func (h *Handler) revealAndContinue(roomID, roomCode string) {
	state := h.gameManager.GetGameState(roomID)
	if state == nil {
		log.Printf("[BlindTest] ⚠️ État du jeu non trouvé pour révélation")
		return
	}

	state.Mutex.Lock()
	if state.IsRevealed {
		state.Mutex.Unlock()
		log.Printf("[BlindTest] ⚠️ Déjà révélé, on ignore")
		return
	}
	state.Mutex.Unlock()

	revealInfo := h.gameManager.RevealAnswer(roomID)
	if revealInfo != nil {
		log.Printf("[BlindTest] 🔔 Révélation: %s - %s", revealInfo.TrackName, revealInfo.ArtistName)
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type:    models.WSTypeBTReveal,
			Payload: revealInfo,
		})
	}

	h.broadcastScores(roomID, roomCode)

	time.Sleep(4 * time.Second)

	if h.gameManager.IsGameOver(roomID) {
		log.Printf("[BlindTest] 🏁 Partie terminée pour salle %s", roomCode)
		h.endGame(roomID, roomCode)
	} else {
		log.Printf("[BlindTest] ➡️ Passage à la manche suivante")
		h.startNextRound(roomID, roomCode)
	}
}

func (h *Handler) endGame(roomID, roomCode string) {
	h.mutex.Lock()
	if stopChan, exists := h.stopTimers[roomID]; exists {
		select {
		case <-stopChan:
		default:
			close(stopChan)
		}
		delete(h.stopTimers, roomID)
	}
	delete(h.roundLocks, roomID)
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
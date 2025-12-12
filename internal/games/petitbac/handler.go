// Package petitbac gère la logique du jeu Petit Bac Musical
package petitbac

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"groupie-tracker/internal/models"
	"groupie-tracker/internal/rooms"
	"groupie-tracker/internal/websocket"
)

// Handler gère les messages WebSocket pour le Petit Bac
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

// HandleMessage traite les messages WebSocket du Petit Bac
func (h *Handler) HandleMessage(client *websocket.Client, msg *models.WSMessage) {
	switch msg.Type {
	case "submit_answers":
		h.handleSubmitAnswers(client, msg)
	case "stop_round":
		h.handleStopRound(client, msg)
	case "submit_votes":
		h.handleSubmitVotes(client, msg)
	default:
		log.Printf("[PetitBac] Message non géré: %s", msg.Type)
	}
}

// StartGame démarre une partie de Petit Bac
func (h *Handler) StartGame(roomCode string, categories []string, rounds int) error {
	room, err := h.roomManager.GetRoomByCode(roomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(roomCode)
		if err != nil {
			return err
		}
	}

	// Démarrer la partie
	_, err = h.gameManager.StartGame(room.ID, categories, rounds)
	if err != nil {
		return err
	}

	log.Printf("[PetitBac] ✅ Partie démarrée dans la salle %s (%d manches)", roomCode, rounds)

	// Créer le canal pour stopper le timer
	h.mutex.Lock()
	h.stopTimers[room.ID] = make(chan bool, 1)
	h.mutex.Unlock()

	// Notifier tous les joueurs
	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type: "game_start",
		Payload: map[string]interface{}{
			"game_type":  "petitbac",
			"categories": categories,
			"rounds":     rounds,
		},
	})

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
		log.Printf("[PetitBac] ❌ Erreur NextRound: %v", err)
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type:  models.WSTypeError,
			Error: err.Error(),
		})
		return
	}

	// Jeu terminé ?
	if roundInfo == nil {
		log.Printf("[PetitBac] 🏁 Jeu terminé pour salle %s", roomCode)
		h.endGame(roomID, roomCode)
		return
	}

	log.Printf("[PetitBac] 📝 Manche %d/%d - Lettre: %s", roundInfo.Round, roundInfo.Total, roundInfo.Letter)

	// Recréer le canal stop pour cette manche
	h.mutex.Lock()
	if _, exists := h.stopTimers[roomID]; !exists {
		h.stopTimers[roomID] = make(chan bool, 1)
	}
	h.mutex.Unlock()

	// Envoyer les infos de la manche à tous les joueurs
	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type:    "new_round",
		Payload: roundInfo,
	})

	// Démarrer le timer
	go h.runRoundTimer(roomID, roomCode, roundInfo.Duration)
}

// runRoundTimer gère le timer d'une manche
func (h *Handler) runRoundTimer(roomID, roomCode string, duration int) {
	state := h.gameManager.GetGameState(roomID)
	if state == nil {
		log.Printf("[PetitBac] ❌ État du jeu non trouvé pour %s", roomID)
		return
	}

	h.mutex.Lock()
	stopChan := h.stopTimers[roomID]
	h.mutex.Unlock()

	if stopChan == nil {
		log.Printf("[PetitBac] ❌ Stop channel non trouvé")
		return
	}

	log.Printf("[PetitBac] ⏱️ Timer démarré: %d secondes", duration)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeLeft := duration

	for timeLeft >= 0 {
		// Vérifier si on doit arrêter (STOP! appuyé)
		select {
		case <-stopChan:
			log.Printf("[PetitBac] ⏹️ Timer interrompu (STOP!)")
			h.mutex.Lock()
			h.stopTimers[roomID] = make(chan bool, 1)
			h.mutex.Unlock()
			h.startVotingPhase(roomID, roomCode)
			return
		default:
		}

		state.Mutex.Lock()
		state.TimeLeft = timeLeft
		state.Mutex.Unlock()

		// Envoyer le temps restant
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type: "time_update",
			Payload: map[string]int{
				"time_left": timeLeft,
			},
		})

		// Vérifier si le jeu existe toujours
		if h.gameManager.GetGameState(roomID) == nil {
			log.Printf("[PetitBac] Jeu terminé pendant le timer")
			return
		}

		// Vérifier si tous les joueurs ont soumis
		if h.gameManager.AllPlayersSubmitted(roomID) {
			log.Printf("[PetitBac] ✅ Tous les joueurs ont soumis leurs réponses")
			h.startVotingPhase(roomID, roomCode)
			return
		}

		if timeLeft == 0 {
			break
		}

		select {
		case <-stopChan:
			log.Printf("[PetitBac] ⏹️ Timer interrompu pendant l'attente")
			h.mutex.Lock()
			h.stopTimers[roomID] = make(chan bool, 1)
			h.mutex.Unlock()
			h.startVotingPhase(roomID, roomCode)
			return
		case <-ticker.C:
			timeLeft--
		}
	}

	log.Printf("[PetitBac] ⏰ Temps écoulé pour salle %s", roomCode)
	h.startVotingPhase(roomID, roomCode)
}

// handleSubmitAnswers traite la soumission des réponses
func (h *Handler) handleSubmitAnswers(client *websocket.Client, msg *models.WSMessage) {
	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		client.SendError("Payload invalide")
		return
	}

	var data struct {
		Answers map[string]string `json:"answers"`
	}
	if err := json.Unmarshal(payloadBytes, &data); err != nil {
		client.SendError("Format de réponse invalide")
		return
	}

	log.Printf("[PetitBac] 📝 Réponses de %s: %+v", client.Pseudo, data.Answers)

	room, err := h.roomManager.GetRoomByCode(client.RoomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(client.RoomCode)
		if err != nil {
			client.SendError("Salle non trouvée")
			return
		}
	}

	err = h.gameManager.SubmitAnswers(room.ID, client.UserID, data.Answers)
	if err != nil {
		client.SendError(err.Error())
		return
	}

	client.Send(&models.WSMessage{
		Type: "answers_submitted",
		Payload: map[string]bool{
			"success": true,
		},
	})

	h.hub.Broadcast(client.RoomCode, &models.WSMessage{
		Type: "player_submitted",
		Payload: map[string]interface{}{
			"user_id": client.UserID,
			"pseudo":  client.Pseudo,
		},
	})
}

// handleStopRound traite le STOP d'un joueur
func (h *Handler) handleStopRound(client *websocket.Client, msg *models.WSMessage) {
	room, err := h.roomManager.GetRoomByCode(client.RoomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(client.RoomCode)
		if err != nil {
			client.SendError("Salle non trouvée")
			return
		}
	}

	err = h.gameManager.StopRound(room.ID, client.UserID)
	if err != nil {
		client.SendError(err.Error())
		return
	}

	log.Printf("[PetitBac] 🛑 %s a appuyé sur STOP!", client.Pseudo)

	h.hub.Broadcast(client.RoomCode, &models.WSMessage{
		Type: "round_stop",
		Payload: map[string]interface{}{
			"stopped_by": client.Pseudo,
		},
	})

	h.mutex.Lock()
	if stopChan, exists := h.stopTimers[room.ID]; exists {
		select {
		case stopChan <- true:
		default:
		}
	}
	h.mutex.Unlock()
}

// startVotingPhase démarre la phase de vote
func (h *Handler) startVotingPhase(roomID, roomCode string) {
	state := h.gameManager.GetGameState(roomID)
	if state == nil {
		return
	}

	room, err := h.roomManager.GetRoom(roomID)
	if err != nil {
		return
	}

	room.Mutex.RLock()
	playerCount := len(room.Players)
	room.Mutex.RUnlock()

	// En mode solo, pas besoin de phase de vote
	if playerCount == 1 {
		log.Printf("[PetitBac] 🎯 Mode solo: pas de phase de vote")
		h.skipVotingAndCalculateScores(roomID, roomCode)
		return
	}

	votingInfo := h.gameManager.StartVoting(roomID)
	if votingInfo == nil {
		return
	}

	var answers []map[string]interface{}
	for category, userAnswers := range votingInfo.Answers {
		for userID, answer := range userAnswers {
			player, _ := h.roomManager.GetPlayer(roomID, userID)
			pseudo := "Inconnu"
			if player != nil {
				pseudo = player.Pseudo
			}

			answers = append(answers, map[string]interface{}{
				"user_id":  userID,
				"pseudo":   pseudo,
				"category": category,
				"answer":   answer,
			})
		}
	}

	log.Printf("[PetitBac] 🗳️ Phase de vote démarrée avec %d réponses", len(answers))

	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type: "voting_start",
		Payload: map[string]interface{}{
			"answers":    answers,
			"duration":   votingInfo.Duration,
			"categories": votingInfo.Categories,
		},
	})

	go h.runVotingTimer(roomID, roomCode, votingInfo.Duration)
}

// skipVotingAndCalculateScores pour le mode solo
func (h *Handler) skipVotingAndCalculateScores(roomID, roomCode string) {
	state := h.gameManager.GetGameState(roomID)
	if state != nil {
		state.Mutex.Lock()
		state.Phase = PhaseResults
		state.Mutex.Unlock()
	}

	roundScores := h.gameManager.CalculateRoundScores(roomID)
	if roundScores == nil {
		return
	}

	var results []map[string]interface{}
	for userID, score := range roundScores.Scores {
		player, _ := h.roomManager.GetPlayer(roomID, userID)
		pseudo := "Inconnu"
		if player != nil {
			pseudo = player.Pseudo
		}
		results = append(results, map[string]interface{}{
			"user_id": userID,
			"pseudo":  pseudo,
			"points":  score,
		})
	}

	scores := h.gameManager.GetScores(roomID)
	scoresMap := make(map[int64]map[string]interface{})
	for _, s := range scores {
		scoresMap[s.UserID] = map[string]interface{}{
			"pseudo": s.Pseudo,
			"score":  s.Score,
		}
	}

	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type: "round_result",
		Payload: map[string]interface{}{
			"results": results,
			"details": roundScores.Details,
			"scores":  scoresMap,
		},
	})

	time.Sleep(4 * time.Second)

	if h.gameManager.IsGameOver(roomID) {
		log.Printf("[PetitBac] 🏁 Partie terminée pour salle %s", roomCode)
		h.endGame(roomID, roomCode)
	} else {
		log.Printf("[PetitBac] ➡️ Passage à la manche suivante")
		h.startNextRound(roomID, roomCode)
	}
}

// runVotingTimer gère le timer de vote
func (h *Handler) runVotingTimer(roomID, roomCode string, duration int) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for timeLeft := duration; timeLeft >= 0; timeLeft-- {
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type: "vote_time_update",
			Payload: map[string]int{
				"time_left": timeLeft,
			},
		})

		if timeLeft == 0 {
			break
		}
		<-ticker.C
	}

	h.calculateAndShowResults(roomID, roomCode)
}

// handleSubmitVotes traite la soumission des votes
func (h *Handler) handleSubmitVotes(client *websocket.Client, msg *models.WSMessage) {
	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		client.SendError("Payload invalide")
		return
	}

	var data struct {
		Votes map[string]bool `json:"votes"`
	}
	if err := json.Unmarshal(payloadBytes, &data); err != nil {
		client.SendError("Format de vote invalide")
		return
	}

	room, err := h.roomManager.GetRoomByCode(client.RoomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(client.RoomCode)
		if err != nil {
			client.SendError("Salle non trouvée")
			return
		}
	}

	for key, accept := range data.Votes {
		parts := strings.Split(key, "_")
		if len(parts) >= 2 {
			targetUserID, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				continue
			}
			category := strings.Join(parts[1:], "_")

			h.gameManager.SubmitVote(room.ID, client.UserID, targetUserID, category, !accept)
		}
	}

	client.Send(&models.WSMessage{
		Type: "votes_submitted",
		Payload: map[string]bool{
			"success": true,
		},
	})
}

// calculateAndShowResults calcule et affiche les résultats
func (h *Handler) calculateAndShowResults(roomID, roomCode string) {
	roundScores := h.gameManager.CalculateRoundScores(roomID)
	if roundScores == nil {
		return
	}

	var results []map[string]interface{}
	for userID, score := range roundScores.Scores {
		player, _ := h.roomManager.GetPlayer(roomID, userID)
		pseudo := "Inconnu"
		if player != nil {
			pseudo = player.Pseudo
		}
		results = append(results, map[string]interface{}{
			"user_id": userID,
			"pseudo":  pseudo,
			"points":  score,
		})
	}

	scores := h.gameManager.GetScores(roomID)
	scoresMap := make(map[int64]map[string]interface{})
	for _, s := range scores {
		scoresMap[s.UserID] = map[string]interface{}{
			"pseudo": s.Pseudo,
			"score":  s.Score,
		}
	}

	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type: "round_result",
		Payload: map[string]interface{}{
			"results": results,
			"details": roundScores.Details,
			"scores":  scoresMap,
		},
	})

	time.Sleep(5 * time.Second)

	if h.gameManager.IsGameOver(roomID) {
		log.Printf("[PetitBac] 🏁 Partie terminée pour salle %s", roomCode)
		h.endGame(roomID, roomCode)
	} else {
		log.Printf("[PetitBac] ➡️ Passage à la manche suivante")
		h.startNextRound(roomID, roomCode)
	}
}

// endGame termine la partie
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
	h.mutex.Unlock()

	result := h.gameManager.EndGame(roomID)
	if result == nil {
		return
	}

	var rankings []map[string]interface{}
	for _, score := range result.Scores {
		rankings = append(rankings, map[string]interface{}{
			"user_id": score.UserID,
			"pseudo":  score.Pseudo,
			"score":   score.Score,
		})
	}

	var winner map[string]interface{}
	if len(result.Scores) > 0 {
		winner = map[string]interface{}{
			"pseudo": result.Scores[0].Pseudo,
			"score":  result.Scores[0].Score,
		}
	}

	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type: "game_end",
		Payload: map[string]interface{}{
			"winner":   winner,
			"rankings": rankings,
		},
	})

	log.Printf("[PetitBac] 🏆 Partie terminée - Gagnant: %s", result.Winner)
}
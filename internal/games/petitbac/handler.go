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

func (h *Handler) HandleMessage(client *websocket.Client, msg *models.WSMessage) {
	log.Printf("[PetitBac] 📨 HandleMessage: type=%s, user=%d", msg.Type, client.UserID)

	switch msg.Type {
	case models.WSTypePBSubmitAnswers:
		h.handleSubmitAnswers(client, msg)
	case models.WSTypePBStopRound:
		h.handleStopRound(client, msg)
	case models.WSTypePBSubmitVotes:
		h.handleSubmitVotes(client, msg)
	default:
		log.Printf("[PetitBac] ⚠️ Message non géré: %s", msg.Type)
	}
}

func (h *Handler) StartGame(roomCode string, categories []string, rounds int) error {
	room, err := h.roomManager.GetRoomByCode(roomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(roomCode)
		if err != nil {
			return h.StartGameWithDuration(roomCode, categories, rounds, DefaultAnswerTime)
		}
	}

	room.Mutex.RLock()
	configCategories := room.Config.Categories
	configRounds := room.Config.NbRounds
	configDuration := room.Config.TimePerRound
	room.Mutex.RUnlock()

	if len(configCategories) > 0 {
		categories = configCategories
	}
	if configRounds > 0 {
		rounds = configRounds
	}
	if configDuration <= 0 {
		configDuration = DefaultAnswerTime
	}

	log.Printf("[PetitBac] Config chargée: %d catégories, %d manches, %ds/manche", len(categories), rounds, configDuration)

	return h.StartGameWithDuration(roomCode, categories, rounds, configDuration)
}

func (h *Handler) StartGameWithDuration(roomCode string, categories []string, rounds int, duration int) error {
	log.Printf("[PetitBac] 🎮 StartGame appelé - roomCode=%s, categories=%v, rounds=%d, duration=%d", roomCode, categories, rounds, duration)

	room, err := h.roomManager.GetRoomByCode(roomCode)
	if err != nil {
		log.Printf("[PetitBac] GetRoomByCode échoué, essai GetRoom...")
		room, err = h.roomManager.GetRoom(roomCode)
		if err != nil {
			log.Printf("[PetitBac] ❌ Salle non trouvée: %s - %v", roomCode, err)
			return err
		}
	}

	log.Printf("[PetitBac] ✅ Salle trouvée - ID=%s, Code=%s", room.ID, room.Code)

	_, err = h.gameManager.StartGameWithDuration(room.ID, categories, rounds, duration)
	if err != nil {
		log.Printf("[PetitBac] ❌ Erreur gameManager.StartGame: %v", err)
		return err
	}

	log.Printf("[PetitBac] ✅ Partie initialisée - RoomID=%s, RoomCode=%s, Manches=%d, Durée=%ds", room.ID, room.Code, rounds, duration)

	h.mutex.Lock()
	h.stopTimers[room.ID] = make(chan bool, 1)
	h.mutex.Unlock()

	log.Printf("[PetitBac] 📤 Broadcast game_start vers roomCode=%s", room.Code)

	h.hub.Broadcast(room.Code, &models.WSMessage{
		Type: "game_start",
		Payload: map[string]interface{}{
			"game_type":  "petitbac",
			"categories": categories,
			"rounds":     rounds,
			"duration":   duration,
		},
	})

	go func() {
		log.Printf("[PetitBac] ⏳ Attente 2s avant première manche...")
		time.Sleep(2 * time.Second)
		log.Printf("[PetitBac] 🚀 Lancement première manche - RoomID=%s, RoomCode=%s", room.ID, room.Code)
		h.startNextRound(room.ID, room.Code)
	}()

	return nil
}

func (h *Handler) startNextRound(roomID, roomCode string) {
	log.Printf("[PetitBac] 🔄 startNextRound - RoomID=%s, RoomCode=%s", roomID, roomCode)

	roundInfo, err := h.gameManager.NextRound(roomID)
	if err != nil {
		log.Printf("[PetitBac] ❌ Erreur NextRound: %v", err)
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type:  models.WSTypeError,
			Error: err.Error(),
		})
		return
	}

	if roundInfo == nil {
		log.Printf("[PetitBac] 🏁 Jeu terminé pour salle %s (roundInfo nil)", roomCode)
		h.endGame(roomID, roomCode)
		return
	}

	log.Printf("[PetitBac] 📝 Nouvelle manche - Round=%d/%d, Lettre=%s", roundInfo.Round, roundInfo.Total, roundInfo.Letter)

	h.mutex.Lock()
	if _, exists := h.stopTimers[roomID]; !exists {
		h.stopTimers[roomID] = make(chan bool, 1)
	}
	h.mutex.Unlock()

	payload := map[string]interface{}{
		"round":      roundInfo.Round,
		"total":      roundInfo.Total,
		"letter":     roundInfo.Letter,
		"categories": roundInfo.Categories,
		"duration":   roundInfo.Duration,
	}

	log.Printf("[PetitBac] 📤 Broadcast new_round vers roomCode=%s - payload=%+v", roomCode, payload)

	h.hub.Broadcast(roomCode, &models.WSMessage{
		Type:    "new_round",
		Payload: payload,
	})

	go h.runRoundTimer(roomID, roomCode, roundInfo.Duration)
}

func (h *Handler) runRoundTimer(roomID, roomCode string, duration int) {
	log.Printf("[PetitBac] ⏱️ Timer démarré - %d secondes, RoomID=%s", duration, roomID)

	state := h.gameManager.GetGameState(roomID)
	if state == nil {
		log.Printf("[PetitBac] ❌ État du jeu non trouvé pour RoomID=%s", roomID)
		return
	}

	h.mutex.Lock()
	stopChan := h.stopTimers[roomID]
	h.mutex.Unlock()

	if stopChan == nil {
		log.Printf("[PetitBac] ❌ Stop channel non trouvé pour RoomID=%s", roomID)
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeLeft := duration

	for timeLeft >= 0 {
		select {
		case <-stopChan:
			log.Printf("[PetitBac] ⏹️ Timer interrompu par STOP!")
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

		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type: "time_update",
			Payload: map[string]interface{}{
				"time_left": timeLeft,
			},
		})

		if h.gameManager.GetGameState(roomID) == nil {
			log.Printf("[PetitBac] ⚠️ Jeu terminé pendant le timer")
			return
		}

		if h.gameManager.AllPlayersSubmitted(roomID) {
			log.Printf("[PetitBac] ✅ Tous les joueurs ont soumis")
			h.startVotingPhase(roomID, roomCode)
			return
		}

		if filled, userID := h.gameManager.AnyPlayerFilledAll(roomID); filled {
			log.Printf("[PetitBac] ✅ Joueur %d a rempli toutes les catégories - arrêt du tour", userID)
			
			player, _ := h.roomManager.GetPlayer(roomID, userID)
			pseudo := "Un joueur"
			if player != nil {
				pseudo = player.Pseudo
			}
			
			h.hub.Broadcast(roomCode, &models.WSMessage{
				Type: "round_stop",
				Payload: map[string]interface{}{
					"stopped_by": pseudo,
					"reason":     "all_filled",
				},
			})
			
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

func (h *Handler) handleSubmitAnswers(client *websocket.Client, msg *models.WSMessage) {
	log.Printf("[PetitBac] 📝 handleSubmitAnswers de %s (ID=%d)", client.Pseudo, client.UserID)

	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		log.Printf("[PetitBac] ❌ Erreur marshal payload: %v", err)
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

	log.Printf("[PetitBac] 📝 Réponses reçues: %+v", data.Answers)

	room, err := h.roomManager.GetRoomByCode(client.RoomCode)
	if err != nil {
		room, err = h.roomManager.GetRoom(client.RoomCode)
		if err != nil {
			log.Printf("[PetitBac] ❌ Salle non trouvée: %s", client.RoomCode)
			client.SendError("Salle non trouvée")
			return
		}
	}

	err = h.gameManager.SubmitAnswers(room.ID, client.UserID, data.Answers)
	if err != nil {
		log.Printf("[PetitBac] ❌ Erreur SubmitAnswers: %v", err)
		client.SendError(err.Error())
		return
	}

	log.Printf("[PetitBac] ✅ Réponses de %s enregistrées", client.Pseudo)

	client.Send(&models.WSMessage{
		Type: "answers_submitted",
		Payload: map[string]interface{}{
			"success": true,
		},
	})

	h.hub.Broadcast(room.Code, &models.WSMessage{
		Type: "player_submitted",
		Payload: map[string]interface{}{
			"user_id": client.UserID,
			"pseudo":  client.Pseudo,
		},
	})
}

func (h *Handler) handleStopRound(client *websocket.Client, msg *models.WSMessage) {
	log.Printf("[PetitBac] 🛑 handleStopRound de %s", client.Pseudo)

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

	h.hub.Broadcast(room.Code, &models.WSMessage{
		Type: "round_stop",
		Payload: map[string]interface{}{
			"stopped_by": client.Pseudo,
			"reason":     "manual",
		},
	})

	h.mutex.Lock()
	if stopChan, exists := h.stopTimers[room.ID]; exists {
		select {
		case stopChan <- true:
			log.Printf("[PetitBac] ✅ Signal STOP envoyé au timer")
		default:
			log.Printf("[PetitBac] ⚠️ Canal STOP plein")
		}
	} else {
		log.Printf("[PetitBac] ⚠️ Pas de canal STOP pour room %s", room.ID)
	}
	h.mutex.Unlock()
}

func (h *Handler) startVotingPhase(roomID, roomCode string) {
	log.Printf("[PetitBac] 🗳️ startVotingPhase - RoomID=%s, RoomCode=%s", roomID, roomCode)

	state := h.gameManager.GetGameState(roomID)
	if state == nil {
		log.Printf("[PetitBac] ❌ État du jeu non trouvé")
		return
	}

	votingInfo := h.gameManager.StartVoting(roomID)
	if votingInfo == nil {
		log.Printf("[PetitBac] ❌ VotingInfo nil")
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

	log.Printf("[PetitBac] 🗳️ Phase de vote - %d réponses à valider", len(answers))

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

func (h *Handler) runVotingTimer(roomID, roomCode string, duration int) {
	log.Printf("[PetitBac] ⏱️ Timer vote démarré - %d secondes", duration)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for timeLeft := duration; timeLeft >= 0; timeLeft-- {
		h.hub.Broadcast(roomCode, &models.WSMessage{
			Type: "vote_time_update",
			Payload: map[string]interface{}{
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

func (h *Handler) handleSubmitVotes(client *websocket.Client, msg *models.WSMessage) {
	log.Printf("[PetitBac] 🗳️ handleSubmitVotes de %s", client.Pseudo)

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
		Payload: map[string]interface{}{
			"success": true,
		},
	})
}

func (h *Handler) calculateAndShowResults(roomID, roomCode string) {
	log.Printf("[PetitBac] 📊 calculateAndShowResults")

	roundScores := h.gameManager.CalculateRoundScores(roomID)
	if roundScores == nil {
		log.Printf("[PetitBac] ❌ roundScores nil")
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
		log.Printf("[PetitBac] 🏁 Partie terminée")
		h.endGame(roomID, roomCode)
	} else {
		log.Printf("[PetitBac] ➡️ Manche suivante")
		h.startNextRound(roomID, roomCode)
	}
}

func (h *Handler) endGame(roomID, roomCode string) {
	log.Printf("[PetitBac] 🏁 endGame - RoomID=%s, RoomCode=%s", roomID, roomCode)

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
		log.Printf("[PetitBac] ❌ result nil")
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
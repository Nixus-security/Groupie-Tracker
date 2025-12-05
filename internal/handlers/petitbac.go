package handlers

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"music-platform/internal/database"
	"music-platform/internal/middleware"
	"music-platform/internal/models"
)

func PetitBac(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)

	code := strings.TrimPrefix(r.URL.Path, "/petitbac/")
	if code == "" {
		http.Redirect(w, r, "/lobby", http.StatusSeeOther)
		return
	}

	room, err := database.GetRoomByCode(code)
	if err != nil || room == nil {
		http.Redirect(w, r, "/lobby?error=room_not_found", http.StatusSeeOther)
		return
	}

	if room.GameType != "petitbac" {
		http.Redirect(w, r, "/lobby", http.StatusSeeOther)
		return
	}

	inRoom, _ := database.IsPlayerInRoom(room.ID, user.ID)
	if !inRoom {
		http.Redirect(w, r, "/lobby?error=not_in_room", http.StatusSeeOther)
		return
	}

	game, _ := database.GetActiveGameByRoomID(room.ID)
	players, _ := database.GetRoomPlayers(room.ID)
	categories, _ := database.GetAllCategories()

	var cfg models.PetitBacConfig
	json.Unmarshal([]byte(room.Config), &cfg)

	data := map[string]interface{}{
		"User":       user,
		"Room":       room,
		"Game":       game,
		"Players":    players,
		"Categories": categories,
		"Config":     cfg,
		"NbrsManche": models.GetNbrsManche(),
	}

	renderTemplate(w, "petitbac.html", data)
}

func PetitBacSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUser(r)

	var req struct {
		GameID  int64            `json:"game_id"`
		Answers map[string]string `json:"answers"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Données invalides", http.StatusBadRequest)
		return
	}

	game, err := database.GetGameByID(req.GameID)
	if err != nil || game == nil {
		http.Error(w, "Partie non trouvée", http.StatusNotFound)
		return
	}

	if game.Status != "active" {
		http.Error(w, "La partie n'est pas en cours", http.StatusBadRequest)
		return
	}

	currentLetter := getCurrentLetter(game.ID)

	for categoryIDStr, answer := range req.Answers {
		var categoryID int64
		json.Unmarshal([]byte(categoryIDStr), &categoryID)

		answer = strings.TrimSpace(answer)
		if answer != "" && !strings.HasPrefix(strings.ToUpper(answer), currentLetter) {
			answer = ""
		}

		database.SavePetitBacAnswer(game.ID, user.ID, game.CurrentRound, categoryID, currentLetter, answer)
	}

	room, _ := database.GetRoomByID(game.RoomID)
	if room != nil {
		hub.BroadcastToRoom(room.Code, map[string]interface{}{
			"type":    "answers_submitted",
			"user_id": user.ID,
			"pseudo":  user.Pseudo,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

func PetitBacVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUser(r)

	var req struct {
		AnswerID int64 `json:"answer_id"`
		IsValid  bool  `json:"is_valid"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Données invalides", http.StatusBadRequest)
		return
	}

	answers, err := database.GetRoundAnswers(0, 0)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	var targetAnswer *database.PetitBacAnswer
	for _, a := range answers {
		if a.ID == req.AnswerID {
			targetAnswer = a
			break
		}
	}

	if targetAnswer == nil {
		http.Error(w, "Réponse non trouvée", http.StatusNotFound)
		return
	}

	if targetAnswer.UserID == user.ID {
		http.Error(w, "Vous ne pouvez pas voter pour votre propre réponse", http.StatusBadRequest)
		return
	}

	votesValid := targetAnswer.VotesValid
	votesInvalid := targetAnswer.VotesInvalid

	if req.IsValid {
		votesValid++
	} else {
		votesInvalid++
	}

	game, _ := database.GetGameByID(targetAnswer.GameID)
	room, _ := database.GetRoomByID(game.RoomID)
	playerCount, _ := database.GetPlayerCount(room.ID)

	isValidated := models.IsAnswerValidated(votesValid, votesInvalid, playerCount)

	database.UpdateAnswerVotes(req.AnswerID, votesValid, votesInvalid, isValidated)

	hub.BroadcastToRoom(room.Code, map[string]interface{}{
		"type":         "vote_update",
		"answer_id":    req.AnswerID,
		"votes_valid":  votesValid,
		"votes_invalid": votesInvalid,
		"is_validated": isValidated,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"is_validated": isValidated,
	})
}

func StartPetitBac(roomCode string, gameID int64) {
	game, _ := database.GetGameByID(gameID)
	if game == nil {
		return
	}

	room, _ := database.GetRoomByCode(roomCode)
	if room == nil {
		return
	}

	var cfg models.PetitBacConfig
	json.Unmarshal([]byte(room.Config), &cfg)

	database.UpdateGameStatus(gameID, "active")
	database.UpdateRoomStatus(room.ID, "playing")

	players, _ := database.GetRoomPlayers(room.ID)
	models.InitGameScoreboard(gameID)
	for _, p := range players {
		models.SetPlayerScore(gameID, p.ID, 0)
	}

	go runPetitBacRounds(roomCode, gameID, cfg)
}

func runPetitBacRounds(roomCode string, gameID int64, cfg models.PetitBacConfig) {
	game, _ := database.GetGameByID(gameID)
	usedLetters := game.UsedLetters

	for round := 1; round <= cfg.TotalRounds; round++ {
		database.UpdateGameRound(gameID, round)

		letter := pickRandomLetter(usedLetters)
		usedLetters += letter
		database.UpdateGameUsedLetters(gameID, usedLetters)

		setCurrentLetter(gameID, letter)

		hub.BroadcastToRoom(roomCode, map[string]interface{}{
			"type":         "round_start",
			"round":        round,
			"total_rounds": cfg.TotalRounds,
			"letter":       letter,
			"time":         cfg.TimePerRound,
			"categories":   cfg.Categories,
		})

		time.Sleep(time.Duration(cfg.TimePerRound) * time.Second)

		database.UpdateGameStatus(gameID, "voting")

		hub.BroadcastToRoom(roomCode, map[string]interface{}{
			"type":  "voting_start",
			"round": round,
		})

		answers, _ := database.GetRoundAnswers(gameID, round)

		hub.BroadcastToRoom(roomCode, map[string]interface{}{
			"type":    "show_answers",
			"answers": answers,
		})

		time.Sleep(30 * time.Second)

		calculateRoundPoints(gameID, round)

		database.UpdateGameStatus(gameID, "active")

		hub.BroadcastToRoom(roomCode, map[string]interface{}{
			"type":   "round_end",
			"round":  round,
			"scores": models.GetGameScores(gameID),
		})

		time.Sleep(5 * time.Second)
	}

	endPetitBac(roomCode, gameID)
}

func endPetitBac(roomCode string, gameID int64) {
	database.UpdateGameStatus(gameID, "finished")

	room, _ := database.GetRoomByCode(roomCode)
	if room != nil {
		database.UpdateRoomStatus(room.ID, "finished")
	}

	scoreboard, _ := database.GetScoreboard(gameID)

	hub.BroadcastToRoom(roomCode, map[string]interface{}{
		"type":       "game_end",
		"scoreboard": scoreboard,
	})

	models.ClearGameScoreboard(gameID)
}

func calculateRoundPoints(gameID int64, round int) {
	answers, _ := database.GetRoundAnswers(gameID, round)

	var modelAnswers []models.PetitBacAnswer
	for _, a := range answers {
		modelAnswers = append(modelAnswers, models.PetitBacAnswer{
			ID:          a.ID,
			GameID:      a.GameID,
			UserID:      a.UserID,
			RoundNumber: a.RoundNumber,
			CategoryID:  a.CategoryID,
			Letter:      a.Letter,
			Answer:      a.Answer,
			IsValidated: a.IsValidated,
		})
	}

	for _, answer := range modelAnswers {
		points := models.CalculatePetitBacPoints(answer, modelAnswers)
		database.UpdateAnswerPoints(answer.ID, points)
		database.AddScore(gameID, answer.UserID, points, round)
		models.UpdatePlayerScore(gameID, answer.UserID, points)
	}
}

func pickRandomLetter(usedLetters string) string {
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	var available []string

	for _, letter := range alphabet {
		if !strings.Contains(usedLetters, string(letter)) {
			available = append(available, string(letter))
		}
	}

	if len(available) == 0 {
		return "A"
	}

	rand.Seed(time.Now().UnixNano())
	return available[rand.Intn(len(available))]
}

var currentLetters = make(map[int64]string)

func setCurrentLetter(gameID int64, letter string) {
	currentLetters[gameID] = letter
}

func getCurrentLetter(gameID int64) string {
	return currentLetters[gameID]
}

package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"music-platform/internal/database"
	"music-platform/internal/middleware"
	"music-platform/internal/models"
	"music-platform/internal/spotify"
)

func BlindTest(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)

	code := strings.TrimPrefix(r.URL.Path, "/blindtest/")
	if code == "" {
		http.Redirect(w, r, "/lobby", http.StatusSeeOther)
		return
	}

	room, err := database.GetRoomByCode(code)
	if err != nil || room == nil {
		http.Redirect(w, r, "/lobby?error=room_not_found", http.StatusSeeOther)
		return
	}

	if room.GameType != "blindtest" {
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

	var cfg models.BlindTestConfig
	json.Unmarshal([]byte(room.Config), &cfg)

	data := map[string]interface{}{
		"User":        user,
		"Room":        room,
		"Game":        game,
		"Players":     players,
		"Config":      cfg,
		"Playlists":   []string{"Rock", "Rap", "Pop"},
		"Description": "Choisis une des playlist !",
	}

	renderTemplate(w, "blindtest.html", data)
}

func BlindTestAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUser(r)

	var req struct {
		GameID       int64  `json:"game_id"`
		Answer       string `json:"answer"`
		ResponseTime int64  `json:"response_time"`
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

	room, _ := database.GetRoomByCode(getCodeFromRoomID(game.RoomID))
	if room == nil {
		http.Error(w, "Salle non trouvée", http.StatusNotFound)
		return
	}

	currentTrack := spotify.GetCurrentTrack(game.ID)
	if currentTrack == nil {
		http.Error(w, "Pas de piste en cours", http.StatusBadRequest)
		return
	}

	isCorrect := checkAnswer(req.Answer, currentTrack.Name, currentTrack.Artist)
	points := calculateBlindTestPoints(isCorrect, req.ResponseTime)

	if isCorrect {
		database.AddScore(game.ID, user.ID, points, game.CurrentRound)
		models.UpdatePlayerScore(game.ID, user.ID, points)
	}

	hub.BroadcastToRoom(room.Code, map[string]interface{}{
		"type":          "answer_submitted",
		"user_id":       user.ID,
		"pseudo":        user.Pseudo,
		"is_correct":    isCorrect,
		"points":        points,
		"response_time": req.ResponseTime,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"is_correct": isCorrect,
		"points":     points,
		"track": map[string]string{
			"name":   currentTrack.Name,
			"artist": currentTrack.Artist,
		},
	})
}

func StartBlindTest(roomCode string, gameID int64) {
	game, _ := database.GetGameByID(gameID)
	if game == nil {
		return
	}

	room, _ := database.GetRoomByCode(roomCode)
	if room == nil {
		return
	}

	var cfg models.BlindTestConfig
	json.Unmarshal([]byte(room.Config), &cfg)

	database.UpdateGameStatus(gameID, "active")
	database.UpdateRoomStatus(room.ID, "playing")

	players, _ := database.GetRoomPlayers(room.ID)
	models.InitGameScoreboard(gameID)
	for _, p := range players {
		models.SetPlayerScore(gameID, p.ID, 0)
	}

	go runBlindTestRounds(roomCode, gameID, cfg)
}

func runBlindTestRounds(roomCode string, gameID int64, cfg models.BlindTestConfig) {
	for round := 1; round <= cfg.TotalRounds; round++ {
		database.UpdateGameRound(gameID, round)

		track, err := spotify.GetRandomTrack(cfg.Playlist)
		if err != nil {
			continue
		}

		spotify.SetCurrentTrack(gameID, track)

		hub.BroadcastToRoom(roomCode, map[string]interface{}{
			"type":         "round_start",
			"round":        round,
			"total_rounds": cfg.TotalRounds,
			"preview_url":  track.PreviewURL,
			"time":         cfg.TimePerRound,
		})

		time.Sleep(time.Duration(cfg.TimePerRound) * time.Second)

		hub.BroadcastToRoom(roomCode, map[string]interface{}{
			"type":   "round_end",
			"round":  round,
			"track":  track,
			"scores": models.GetGameScores(gameID),
		})

		time.Sleep(5 * time.Second)
	}

	endBlindTest(roomCode, gameID)
}

func endBlindTest(roomCode string, gameID int64) {
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

func checkAnswer(answer, trackName, artist string) bool {
	answer = normalizeString(answer)
	trackName = normalizeString(trackName)
	artist = normalizeString(artist)

	if strings.Contains(trackName, answer) && len(answer) >= 3 {
		return true
	}

	if strings.Contains(artist, answer) && len(answer) >= 3 {
		return true
	}

	if similarity(answer, trackName) > 0.8 {
		return true
	}

	return false
}

func calculateBlindTestPoints(isCorrect bool, responseTime int64) int {
	if !isCorrect {
		return 0
	}

	basePoints := 100

	if responseTime < 5000 {
		return basePoints + 50
	} else if responseTime < 10000 {
		return basePoints + 30
	} else if responseTime < 20000 {
		return basePoints + 10
	}

	return basePoints
}

func normalizeString(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	return s
}

func similarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	matches := 0
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			matches++
		}
	}

	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	return float64(matches) / float64(maxLen)
}

func getCodeFromRoomID(roomID int64) string {
	room, _ := database.GetRoomByID(roomID)
	if room != nil {
		return room.Code
	}
	return ""
}

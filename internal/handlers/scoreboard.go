package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"music-platform/internal/database"
	"music-platform/internal/middleware"
	"music-platform/internal/models"
)

func Scoreboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)

	path := strings.TrimPrefix(r.URL.Path, "/scoreboard/")
	gameIDStr := strings.Split(path, "/")[0]

	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/lobby", http.StatusSeeOther)
		return
	}

	game, err := database.GetGameByID(gameID)
	if err != nil || game == nil {
		http.Redirect(w, r, "/lobby?error=game_not_found", http.StatusSeeOther)
		return
	}

	room, _ := database.GetRoomByID(game.RoomID)
	if room == nil {
		http.Redirect(w, r, "/lobby", http.StatusSeeOther)
		return
	}

	inRoom, _ := database.IsPlayerInRoom(room.ID, user.ID)
	if !inRoom {
		http.Redirect(w, r, "/lobby?error=not_in_room", http.StatusSeeOther)
		return
	}

	scoreboard, _ := database.GetScoreboard(gameID)
	players, _ := database.GetRoomPlayers(room.ID)

	var playerModels []models.Player
	for _, p := range players {
		playerModels = append(playerModels, models.Player{
			ID:     p.ID,
			Pseudo: p.Pseudo,
		})
	}

	liveScoreboard := models.BuildScoreboard(gameID, playerModels, game.GameType, game.Status)

	var finalScoreboard interface{}
	if game.Status == "finished" {
		finalScoreboard = scoreboard
	} else {
		finalScoreboard = liveScoreboard
	}

	data := map[string]interface{}{
		"User":       user,
		"Game":       game,
		"Room":       room,
		"Scoreboard": finalScoreboard,
		"IsFinished": game.Status == "finished",
	}

	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(finalScoreboard)
		return
	}

	renderTemplate(w, "scoreboard.html", data)
}

func APIScoreboard(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/scoreboard/")
	gameIDStr := strings.Split(path, "/")[0]

	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		http.Error(w, "ID invalide", http.StatusBadRequest)
		return
	}

	game, err := database.GetGameByID(gameID)
	if err != nil || game == nil {
		http.Error(w, "Partie non trouvée", http.StatusNotFound)
		return
	}

	room, _ := database.GetRoomByID(game.RoomID)
	players, _ := database.GetRoomPlayers(room.ID)

	var playerModels []models.Player
	for _, p := range players {
		playerModels = append(playerModels, models.Player{
			ID:     p.ID,
			Pseudo: p.Pseudo,
		})
	}

	scoreboard := models.BuildScoreboard(gameID, playerModels, game.GameType, game.Status)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(scoreboard)
}

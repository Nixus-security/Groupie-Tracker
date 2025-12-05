
package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"music-platform/internal/database"
	"music-platform/internal/middleware"
	"music-platform/internal/models"
)

func Lobby(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)

	rooms, err := database.GetActiveRooms()
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	gameFilter := r.URL.Query().Get("game")

	var filteredRooms []*database.Room
	for _, room := range rooms {
		if gameFilter == "" || room.GameType == gameFilter {
			filteredRooms = append(filteredRooms, room)
		}
	}

	for _, room := range filteredRooms {
		players, _ := database.GetRoomPlayers(room.ID)
		room.Config = formatPlayerCount(len(players))
	}

	data := map[string]interface{}{
		"User":       user,
		"Rooms":      filteredRooms,
		"GameFilter": gameFilter,
	}

	renderTemplate(w, "lobby.html", data)
}

func CreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUser(r)

	gameType := r.FormValue("game_type")
	if gameType != "blindtest" && gameType != "petitbac" {
		http.Error(w, "Type de jeu invalide", http.StatusBadRequest)
		return
	}

	code, err := generateRoomCode()
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	var configJSON string
	if gameType == "blindtest" {
		cfg := models.DefaultBlindTestConfig()
		if playlist := r.FormValue("playlist"); playlist != "" {
			cfg.Playlist = playlist
		}
		data, _ := json.Marshal(cfg)
		configJSON = string(data)
	} else {
		cfg := models.DefaultPetitBacConfig()
		data, _ := json.Marshal(cfg)
		configJSON = string(data)
	}

	roomID, err := database.CreateRoom(code, user.ID, gameType, configJSON)
	if err != nil {
		http.Error(w, "Erreur création salle", http.StatusInternalServerError)
		return
	}

	if err := database.AddPlayerToRoom(roomID, user.ID); err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/room/"+code, http.StatusSeeOther)
}

func JoinRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUser(r)

	code := strings.ToUpper(strings.TrimSpace(r.FormValue("code")))
	if code == "" {
		http.Redirect(w, r, "/lobby?error=code_required", http.StatusSeeOther)
		return
	}

	room, err := database.GetRoomByCode(code)
	if err != nil || room == nil {
		http.Redirect(w, r, "/lobby?error=room_not_found", http.StatusSeeOther)
		return
	}

	if room.Status != "waiting" {
		http.Redirect(w, r, "/lobby?error=game_started", http.StatusSeeOther)
		return
	}

	if err := database.AddPlayerToRoom(room.ID, user.ID); err != nil {
		http.Redirect(w, r, "/lobby?error=join_failed", http.StatusSeeOther)
		return
	}

	hub.BroadcastToRoom(code, map[string]interface{}{
		"type":   "player_joined",
		"pseudo": user.Pseudo,
	})

	http.Redirect(w, r, "/room/"+code, http.StatusSeeOther)
}

func Room(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)

	code := strings.TrimPrefix(r.URL.Path, "/room/")
	if code == "" {
		http.Redirect(w, r, "/lobby", http.StatusSeeOther)
		return
	}

	room, err := database.GetRoomByCode(code)
	if err != nil || room == nil {
		http.Redirect(w, r, "/lobby?error=room_not_found", http.StatusSeeOther)
		return
	}

	inRoom, _ := database.IsPlayerInRoom(room.ID, user.ID)
	if !inRoom {
		if room.Status != "waiting" {
			http.Redirect(w, r, "/lobby?error=game_started", http.StatusSeeOther)
			return
		}
		database.AddPlayerToRoom(room.ID, user.ID)
	}

	players, _ := database.GetRoomPlayers(room.ID)

	creator, _ := database.GetUserByID(room.CreatorID)

	modelRoom := models.Room{
		ID:        room.ID,
		Code:      room.Code,
		CreatorID: room.CreatorID,
		GameType:  room.GameType,
		Status:    room.Status,
		Config:    room.Config,
	}
	for _, p := range players {
		modelRoom.Players = append(modelRoom.Players, models.Player{
			ID:     p.ID,
			Pseudo: p.Pseudo,
		})
	}

	isReady := models.IsRoomReady(modelRoom)
	isCreator := user.ID == room.CreatorID

	data := map[string]interface{}{
		"User":      user,
		"Room":      room,
		"Players":   players,
		"Creator":   creator,
		"IsCreator": isCreator,
		"IsReady":   isReady,
	}

	if room.GameType == "blindtest" {
		var cfg models.BlindTestConfig
		json.Unmarshal([]byte(room.Config), &cfg)
		data["Config"] = cfg
	} else {
		var cfg models.PetitBacConfig
		json.Unmarshal([]byte(room.Config), &cfg)
		data["Config"] = cfg
		categories, _ := database.GetAllCategories()
		data["Categories"] = categories
	}

	renderTemplate(w, "room.html", data)
}

func generateRoomCode() (string, error) {
	for i := 0; i < 10; i++ {
		bytes := make([]byte, 3)
		if _, err := rand.Read(bytes); err != nil {
			return "", err
		}
		code := strings.ToUpper(hex.EncodeToString(bytes))

		exists, err := database.RoomCodeExists(code)
		if err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", nil
}

func formatPlayerCount(count int) string {
	if count == 1 {
		return "1 joueur"
	}
	return string(rune('0'+count)) + " joueurs"
}
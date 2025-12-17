package rooms

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"groupie-tracker/internal/auth"
	"groupie-tracker/internal/models"
)

type Handler struct {
	templateDir string
}

func NewHandler(templateDir string) *Handler {
	return &Handler{
		templateDir: templateDir,
	}
}

func (h *Handler) HandleLobby(w http.ResponseWriter, r *http.Request) {
	sessionManager := auth.NewSessionManager()
	user, err := sessionManager.GetUserFromRequest(r)
	if err != nil {
		http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusSeeOther)
		return
	}

	manager := GetManager()
	rooms := manager.GetAllRooms()

	data := map[string]interface{}{
		"Title": "Salles de jeu",
		"User":  user,
		"Rooms": rooms,
	}

	tmplPath := filepath.Join(h.templateDir, "rooms.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Printf("[ROOMS] Erreur chargement template rooms.html: %v", err)
		http.ServeFile(w, r, tmplPath)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ROOMS] Erreur exécution template: %v", err)
		http.Error(w, "Erreur interne", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) HandleRoom(w http.ResponseWriter, r *http.Request) {
	sessionManager := auth.NewSessionManager()
	user, err := sessionManager.GetUserFromRequest(r)
	if err != nil {
		http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusSeeOther)
		return
	}

	code := strings.TrimPrefix(r.URL.Path, "/room/")
	if code == "" || code == "create" || code == "join" {
		http.Redirect(w, r, "/rooms", http.StatusSeeOther)
		return
	}

	manager := GetManager()
	room, err := manager.GetRoomByCode(code)
	if err != nil {
		room, err = manager.GetRoom(code)
		if err != nil || room == nil {
			http.Error(w, "Salle introuvable", http.StatusNotFound)
			return
		}
	}

	var player *models.Player
	room.Mutex.RLock()
	if p, exists := room.Players[user.ID]; exists {
		player = p
	}
	room.Mutex.RUnlock()

	if player == nil {
		http.Redirect(w, r, "/rooms?error=not_in_room", http.StatusSeeOther)
		return
	}

	var tmplFile string
	switch room.GameType {
	case models.GameTypeBlindTest:
		tmplFile = "room_blindtest.html"
	case models.GameTypePetitBac:
		tmplFile = "room_petitbac.html"
	default:
		tmplFile = "room.html"
	}

	data := map[string]interface{}{
		"Title":  room.Name,
		"User":   user,
		"Room":   room,
		"Player": player,
	}

	tmplPath := filepath.Join(h.templateDir, tmplFile)
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Printf("[ROOMS] Erreur chargement template %s: %v", tmplFile, err)
		http.Error(w, "Erreur interne", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ROOMS] Erreur exécution template: %v", err)
		http.Error(w, "Erreur interne", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) HandleGetRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	manager := GetManager()
	rooms := manager.GetAllRooms()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"rooms":   rooms,
	})
}

func (h *Handler) HandleCreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	sessionManager := auth.NewSessionManager()
	user, err := sessionManager.GetUserFromRequest(r)
	if err != nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Erreur parsing formulaire", http.StatusBadRequest)
		return
	}

	roomName := r.FormValue("room_name")
	gameTypeStr := r.FormValue("game_type")

	if roomName == "" {
		http.Error(w, "Le nom de la salle est requis", http.StatusBadRequest)
		return
	}

	var gameType models.GameType
	switch gameTypeStr {
	case "blindtest":
		gameType = models.GameTypeBlindTest
	case "petitbac":
		gameType = models.GameTypePetitBac
	default:
		http.Error(w, "Type de jeu invalide", http.StatusBadRequest)
		return
	}

	manager := GetManager()
	room, err := manager.CreateRoom(roomName, user.ID, user.Pseudo, gameType)
	if err != nil {
		log.Printf("[ROOMS] Erreur création salle: %v", err)
		http.Error(w, "Erreur création salle", http.StatusInternalServerError)
		return
	}

	if gameType == models.GameTypePetitBac {
		room.Mutex.Lock()

		categories := r.Form["categories"]
		if len(categories) >= 3 {
			room.Config.Categories = categories
			log.Printf("[ROOMS] Catégories personnalisées: %v", categories)
		} else {
			room.Config.Categories = models.DefaultPetitBacCategories
		}

		roundTimeStr := r.FormValue("round_time")
		if roundTimeStr != "" {
			if roundTime, err := strconv.Atoi(roundTimeStr); err == nil && roundTime >= 30 && roundTime <= 120 {
				room.Config.TimePerRound = roundTime
				log.Printf("[ROOMS] Temps par manche: %ds", roundTime)
			} else {
				room.Config.TimePerRound = 60
			}
		} else {
			room.Config.TimePerRound = 60
		}

		roundCountStr := r.FormValue("round_count")
		if roundCountStr != "" {
			if roundCount, err := strconv.Atoi(roundCountStr); err == nil && roundCount >= 3 && roundCount <= 15 {
				room.Config.NbRounds = roundCount
				log.Printf("[ROOMS] Nombre de manches: %d", roundCount)
			} else {
				room.Config.NbRounds = models.NbrsManche
			}
		} else {
			room.Config.NbRounds = models.NbrsManche
		}

		room.Mutex.Unlock()

		log.Printf("[ROOMS] Config Petit Bac: %d catégories, %ds/manche, %d manches",
			len(room.Config.Categories), room.Config.TimePerRound, room.Config.NbRounds)
	}

	log.Printf("[ROOMS] Salle créée: %s (%s) par %s", room.Code, room.Name, user.Pseudo)

	http.Redirect(w, r, "/room/"+room.Code, http.StatusSeeOther)
}

func (h *Handler) HandleJoinRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	sessionManager := auth.NewSessionManager()
	user, err := sessionManager.GetUserFromRequest(r)
	if err != nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Erreur parsing formulaire", http.StatusBadRequest)
		return
	}

	code := strings.ToUpper(strings.TrimSpace(r.FormValue("code")))
	if code == "" {
		http.Redirect(w, r, "/room/join?error=Code+requis", http.StatusSeeOther)
		return
	}

	manager := GetManager()
	room, err := manager.GetRoomByCode(code)
	if err != nil {
		room, err = manager.GetRoom(code)
		if err != nil || room == nil {
			http.Redirect(w, r, "/room/join?error=Salle+introuvable", http.StatusSeeOther)
			return
		}
	}

	player := &models.Player{
		UserID:  user.ID,
		Pseudo:  user.Pseudo,
		Score:   0,
		IsReady: false,
	}

	room.Mutex.Lock()

	if _, exists := room.Players[user.ID]; exists {
		room.Mutex.Unlock()
		http.Redirect(w, r, "/room/"+room.Code, http.StatusSeeOther)
		return
	}

	if len(room.Players) >= 8 {
		room.Mutex.Unlock()
		http.Redirect(w, r, "/room/join?error=Salle+pleine", http.StatusSeeOther)
		return
	}

	room.Players[user.ID] = player
	room.Mutex.Unlock()

	log.Printf("[ROOMS] %s a rejoint la salle %s", user.Pseudo, room.Code)

	http.Redirect(w, r, "/room/"+room.Code, http.StatusSeeOther)
}

func (h *Handler) HandleLeaveRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	sessionManager := auth.NewSessionManager()
	user, err := sessionManager.GetUserFromRequest(r)
	if err != nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	var req struct {
		RoomCode string `json:"room_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Erreur parsing JSON", http.StatusBadRequest)
		return
	}

	manager := GetManager()
	room, err := manager.GetRoomByCode(req.RoomCode)
	if err != nil {
		room, err = manager.GetRoom(req.RoomCode)
		if err != nil || room == nil {
			http.Error(w, "Salle introuvable", http.StatusNotFound)
			return
		}
	}

	room.Mutex.Lock()
	delete(room.Players, user.ID)
	room.Mutex.Unlock()

	log.Printf("[ROOMS] %s a quitté la salle %s", user.Pseudo, room.Code)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Vous avez quitté la salle",
	})
}

func (h *Handler) HandleRestartRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	sessionManager := auth.NewSessionManager()
	user, err := sessionManager.GetUserFromRequest(r)
	if err != nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/rooms/")
	code := strings.TrimSuffix(path, "/restart")

	manager := GetManager()
	room, err := manager.GetRoomByCode(code)
	if err != nil {
		room, err = manager.GetRoom(code)
		if err != nil || room == nil {
			http.Error(w, "Salle introuvable", http.StatusNotFound)
			return
		}
	}

	if room.HostID != user.ID {
		http.Error(w, "Seul l'hôte peut redémarrer la partie", http.StatusForbidden)
		return
	}

	room.Mutex.Lock()
	room.Status = models.RoomStatusWaiting
	for _, player := range room.Players {
		player.Score = 0
		player.IsReady = false
	}
	if room.GameType == models.GameTypePetitBac {
		room.Config.UsedLetters = []string{}
	}
	room.Mutex.Unlock()

	log.Printf("[ROOMS] Partie redémarrée dans la salle %s par %s", room.Code, user.Pseudo)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Partie redémarrée",
	})
}
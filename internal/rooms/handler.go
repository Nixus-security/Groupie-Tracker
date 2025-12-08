// Package rooms - handler.go
// Handlers HTTP pour la gestion des salles
package rooms

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"path/filepath"

	"groupie-tracker/internal/auth"
	"groupie-tracker/internal/models"
)

// Handler gère les requêtes HTTP des salles
type Handler struct {
	manager   *Manager
	service   *Service
	templates *template.Template
}

// NewHandler crée une nouvelle instance du handler
func NewHandler(templatesDir string) *Handler {
	// Fonctions personnalisées pour les templates
	funcMap := template.FuncMap{
		"slice": func(s string, start, end int) string {
			if start >= len(s) {
				return ""
			}
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
		"eq": func(a, b interface{}) bool {
			return a == b
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseGlob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		log.Printf("⚠️ Erreur chargement templates rooms: %v", err)
	}

	return &Handler{
		manager:   GetManager(),
		service:   NewService(),
		templates: tmpl,
	}
}

// RegisterRoutes enregistre les routes des salles
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware) {
	// Pages (nécessitent authentification)
	mux.Handle("/rooms", authMiddleware.RequireAuth(http.HandlerFunc(h.RoomsListPage)))
	mux.Handle("/room/create", authMiddleware.RequireAuth(http.HandlerFunc(h.CreateRoomPage)))
	mux.Handle("/room/join", authMiddleware.RequireAuth(http.HandlerFunc(h.JoinRoomPage)))
	mux.Handle("/room/", authMiddleware.RequireAuth(http.HandlerFunc(h.RoomPage)))

	// API (nécessitent authentification)
	mux.Handle("/api/rooms", authMiddleware.RequireAuthAPI(http.HandlerFunc(h.APIListRooms)))
	mux.Handle("/api/rooms/create", authMiddleware.RequireAuthAPI(http.HandlerFunc(h.APICreateRoom)))
	mux.Handle("/api/rooms/join", authMiddleware.RequireAuthAPI(http.HandlerFunc(h.APIJoinRoom)))
	mux.Handle("/api/rooms/leave", authMiddleware.RequireAuthAPI(http.HandlerFunc(h.APILeaveRoom)))
	mux.Handle("/api/rooms/ready", authMiddleware.RequireAuthAPI(http.HandlerFunc(h.APISetReady)))
	mux.Handle("/api/rooms/start", authMiddleware.RequireAuthAPI(http.HandlerFunc(h.APIStartGame)))
}

// ============================================================================
// PAGES HTML
// ============================================================================

// RoomsListPage affiche la liste des salles
func (h *Handler) RoomsListPage(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	
	data := map[string]interface{}{
		"Title":  "Salles de jeu",
		"User":   user,
		"Rooms":  h.manager.GetAllRooms(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if h.templates != nil {
		if err := h.templates.ExecuteTemplate(w, "rooms.html", data); err != nil {
			log.Printf("❌ Erreur template rooms: %v", err)
			h.renderBasicRoomsPage(w, data)
		}
	} else {
		h.renderBasicRoomsPage(w, data)
	}
}

// CreateRoomPage affiche le formulaire de création de salle
func (h *Handler) CreateRoomPage(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())

	if r.Method == http.MethodPost {
		h.handleCreateRoomForm(w, r, user)
		return
	}

	data := map[string]interface{}{
		"Title": "Créer une salle",
		"User":  user,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if h.templates != nil {
		if err := h.templates.ExecuteTemplate(w, "create_room.html", data); err != nil {
			log.Printf("❌ Erreur template create_room: %v", err)
			h.renderBasicCreateRoomPage(w, data)
		}
	} else {
		h.renderBasicCreateRoomPage(w, data)
	}
}

// JoinRoomPage affiche le formulaire pour rejoindre une salle
func (h *Handler) JoinRoomPage(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())

	if r.Method == http.MethodPost {
		code := r.FormValue("code")
		_, err := h.manager.JoinRoom(code, user.ID, user.Pseudo)
		if err != nil {
			http.Redirect(w, r, "/room/join?error="+err.Error(), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/room/"+code, http.StatusSeeOther)
		return
	}

	data := map[string]interface{}{
		"Title": "Rejoindre une salle",
		"User":  user,
		"Error": r.URL.Query().Get("error"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if h.templates != nil {
		if err := h.templates.ExecuteTemplate(w, "join_room.html", data); err != nil {
			log.Printf("❌ Erreur template join_room: %v", err)
			h.renderBasicJoinRoomPage(w, data)
		}
	} else {
		h.renderBasicJoinRoomPage(w, data)
	}
}

// RoomPage affiche une salle spécifique
func (h *Handler) RoomPage(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	
	// Extraire le code de la salle depuis l'URL (/room/XXXXXX)
	code := r.URL.Path[len("/room/"):]
	if code == "" {
		http.Redirect(w, r, "/rooms", http.StatusSeeOther)
		return
	}

	room, err := h.manager.GetRoom(code)
	if err != nil {
		http.Redirect(w, r, "/rooms?error=Salle+non+trouvée", http.StatusSeeOther)
		return
	}

	// Vérifier si le joueur est dans la salle
	room.Mutex.RLock()
	_, isInRoom := room.Players[user.ID]
	room.Mutex.RUnlock()

	if !isInRoom {
		// Tenter de rejoindre automatiquement
		_, err = h.manager.JoinRoom(code, user.ID, user.Pseudo)
		if err != nil {
			http.Redirect(w, r, "/rooms?error="+err.Error(), http.StatusSeeOther)
			return
		}
	}

	data := map[string]interface{}{
		"Title":  room.Name,
		"User":   user,
		"Room":   room,
		"IsHost": room.HostID == user.ID,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if h.templates != nil {
		if err := h.templates.ExecuteTemplate(w, "room.html", data); err != nil {
			log.Printf("❌ Erreur template room: %v", err)
			h.renderBasicRoomPage(w, data)
		}
	} else {
		h.renderBasicRoomPage(w, data)
	}
}

func (h *Handler) handleCreateRoomForm(w http.ResponseWriter, r *http.Request, user *models.User) {
	name := r.FormValue("name")
	gameType := models.GameType(r.FormValue("game_type"))

	room, err := h.manager.CreateRoom(user.ID, user.Pseudo, name, gameType)
	if err != nil {
		http.Redirect(w, r, "/room/create?error="+err.Error(), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/room/"+room.Code, http.StatusSeeOther)
}

// ============================================================================
// API JSON
// ============================================================================

// CreateRoomRequest requête de création de salle
type CreateRoomRequest struct {
	Name     string          `json:"name"`
	GameType models.GameType `json:"game_type"`
}

// JoinRoomRequest requête pour rejoindre une salle
type JoinRoomRequest struct {
	Code string `json:"code"`
}

// SetReadyRequest requête pour définir l'état prêt
type SetReadyRequest struct {
	Code  string `json:"code"`
	Ready bool   `json:"ready"`
}

// RoomResponse réponse contenant une salle
type RoomResponse struct {
	Success bool     `json:"success"`
	Room    *RoomDTO `json:"room,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// RoomDTO structure de salle pour l'API
type RoomDTO struct {
	ID        string             `json:"id"`
	Code      string             `json:"code"`
	Name      string             `json:"name"`
	HostID    int64              `json:"host_id"`
	GameType  models.GameType    `json:"game_type"`
	Status    models.RoomStatus  `json:"status"`
	Players   []PlayerDTO        `json:"players"`
	Config    models.GameConfig  `json:"config"`
	IsReady   bool               `json:"is_ready"`
}

// PlayerDTO structure de joueur pour l'API
type PlayerDTO struct {
	UserID    int64  `json:"user_id"`
	Pseudo    string `json:"pseudo"`
	Score     int    `json:"score"`
	IsHost    bool   `json:"is_host"`
	IsReady   bool   `json:"is_ready"`
	Connected bool   `json:"connected"`
}

// APIListRooms liste toutes les salles disponibles
func (h *Handler) APIListRooms(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rooms := h.manager.GetAllRooms()
	var roomDTOs []RoomDTO

	for _, room := range rooms {
		if room.Status == models.RoomStatusWaiting {
			roomDTOs = append(roomDTOs, h.roomToDTO(room))
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"rooms":   roomDTOs,
	})
}

// APICreateRoom crée une nouvelle salle via API
func (h *Handler) APICreateRoom(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: "Méthode non autorisée"})
		return
	}

	user := auth.GetUserFromContext(r.Context())

	var req CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: "JSON invalide"})
		return
	}

	room, err := h.manager.CreateRoom(user.ID, user.Pseudo, req.Name, req.GameType)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: err.Error()})
		return
	}

	dto := h.roomToDTO(room)
	json.NewEncoder(w).Encode(RoomResponse{
		Success: true,
		Room:    &dto,
	})
}

// APIJoinRoom rejoint une salle via API
func (h *Handler) APIJoinRoom(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: "Méthode non autorisée"})
		return
	}

	user := auth.GetUserFromContext(r.Context())

	var req JoinRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: "JSON invalide"})
		return
	}

	room, err := h.manager.JoinRoom(req.Code, user.ID, user.Pseudo)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: err.Error()})
		return
	}

	dto := h.roomToDTO(room)
	json.NewEncoder(w).Encode(RoomResponse{
		Success: true,
		Room:    &dto,
	})
}

// APILeaveRoom quitte une salle via API
func (h *Handler) APILeaveRoom(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: "Méthode non autorisée"})
		return
	}

	user := auth.GetUserFromContext(r.Context())

	var req JoinRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: "JSON invalide"})
		return
	}

	err := h.manager.LeaveRoom(req.Code, user.ID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(RoomResponse{Success: true})
}

// APISetReady définit l'état prêt d'un joueur
func (h *Handler) APISetReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: "Méthode non autorisée"})
		return
	}

	user := auth.GetUserFromContext(r.Context())

	var req SetReadyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: "JSON invalide"})
		return
	}

	err := h.manager.SetPlayerReady(req.Code, user.ID, req.Ready)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: err.Error()})
		return
	}

	room, _ := h.manager.GetRoom(req.Code)
	dto := h.roomToDTO(room)
	json.NewEncoder(w).Encode(RoomResponse{
		Success: true,
		Room:    &dto,
	})
}

// APIStartGame démarre la partie
func (h *Handler) APIStartGame(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: "Méthode non autorisée"})
		return
	}

	user := auth.GetUserFromContext(r.Context())

	var req JoinRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: "JSON invalide"})
		return
	}

	err := h.manager.StartGame(req.Code, user.ID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RoomResponse{Success: false, Error: err.Error()})
		return
	}

	room, _ := h.manager.GetRoom(req.Code)
	dto := h.roomToDTO(room)
	json.NewEncoder(w).Encode(RoomResponse{
		Success: true,
		Room:    &dto,
	})
}

// ============================================================================
// HELPERS
// ============================================================================

func (h *Handler) roomToDTO(room *models.Room) RoomDTO {
	room.Mutex.RLock()
	defer room.Mutex.RUnlock()

	players := make([]PlayerDTO, 0, len(room.Players))
	for _, p := range room.Players {
		players = append(players, PlayerDTO{
			UserID:    p.UserID,
			Pseudo:    p.Pseudo,
			Score:     p.Score,
			IsHost:    p.IsHost,
			IsReady:   p.IsReady,
			Connected: p.Connected,
		})
	}

	return RoomDTO{
		ID:       room.ID,
		Code:     room.Code,
		Name:     room.Name,
		HostID:   room.HostID,
		GameType: room.GameType,
		Status:   room.Status,
		Players:  players,
		Config:   room.Config,
		IsReady:  models.IsRoomReady(room),
	}
}

// ============================================================================
// TEMPLATES DE SECOURS
// ============================================================================

func (h *Handler) renderBasicRoomsPage(w http.ResponseWriter, data map[string]interface{}) {
	html := `<!DOCTYPE html>
<html lang="fr">
<head><meta charset="UTF-8"><title>{{.Title}}</title><link rel="stylesheet" href="/static/css/style.css"></head>
<body>
<nav class="navbar"><a href="/" class="navbar-brand">🎵 Groupie Tracker</a>
<ul class="navbar-nav"><li><a href="/">Accueil</a></li><li><a href="/rooms">Salles</a></li></ul>
<div class="navbar-user"><span>{{.User.Pseudo}}</span><a href="/logout" class="btn btn-sm">Déconnexion</a></div></nav>
<div class="container"><h1>Salles de jeu</h1>
<div class="d-flex gap-md mb-lg"><a href="/room/create" class="btn btn-success">Créer une salle</a><a href="/room/join" class="btn btn-secondary">Rejoindre avec code</a></div>
<div class="rooms-grid">{{range .Rooms}}<div class="card room-card"><h3>{{.Name}}</h3><p>Code: {{.Code}}</p><p>Type: {{.GameType}}</p><a href="/room/{{.Code}}" class="btn btn-primary">Rejoindre</a></div>{{else}}<p>Aucune salle disponible</p>{{end}}</div>
</div></body></html>`
	tmpl, _ := template.New("rooms").Parse(html)
	tmpl.Execute(w, data)
}

func (h *Handler) renderBasicCreateRoomPage(w http.ResponseWriter, data map[string]interface{}) {
	html := `<!DOCTYPE html>
<html lang="fr">
<head><meta charset="UTF-8"><title>{{.Title}}</title><link rel="stylesheet" href="/static/css/style.css"></head>
<body>
<nav class="navbar"><a href="/" class="navbar-brand">🎵 Groupie Tracker</a></nav>
<div class="container container-sm"><div class="card"><h1>Créer une salle</h1>
<form method="POST"><div class="form-group"><label>Nom de la salle</label><input type="text" name="name" class="form-control" required></div>
<div class="form-group"><label>Type de jeu</label><select name="game_type" class="form-control"><option value="blindtest">🎧 Blind Test</option><option value="petitbac">🔤 Petit Bac</option></select></div>
<button type="submit" class="btn btn-primary btn-lg btn-block">Créer</button></form></div></div></body></html>`
	tmpl, _ := template.New("create_room").Parse(html)
	tmpl.Execute(w, data)
}

func (h *Handler) renderBasicJoinRoomPage(w http.ResponseWriter, data map[string]interface{}) {
	html := `<!DOCTYPE html>
<html lang="fr">
<head><meta charset="UTF-8"><title>{{.Title}}</title><link rel="stylesheet" href="/static/css/style.css"></head>
<body>
<nav class="navbar"><a href="/" class="navbar-brand">🎵 Groupie Tracker</a></nav>
<div class="container container-sm"><div class="card"><h1>Rejoindre une salle</h1>
{{if .Error}}<div class="alert alert-danger">{{.Error}}</div>{{end}}
<form method="POST"><div class="form-group"><label>Code de la salle</label><input type="text" name="code" class="form-control" placeholder="XXXXXX" maxlength="6" required style="text-transform:uppercase;"></div>
<button type="submit" class="btn btn-primary btn-lg btn-block">Rejoindre</button></form></div></div></body></html>`
	tmpl, _ := template.New("join_room").Parse(html)
	tmpl.Execute(w, data)
}

func (h *Handler) renderBasicRoomPage(w http.ResponseWriter, data map[string]interface{}) {
	html := `<!DOCTYPE html>
<html lang="fr">
<head><meta charset="UTF-8"><title>{{.Title}}</title><link rel="stylesheet" href="/static/css/style.css"></head>
<body>
<nav class="navbar"><a href="/" class="navbar-brand">🎵 Groupie Tracker</a></nav>
<div class="container"><h1>{{.Room.Name}}</h1><p>Code: <strong>{{.Room.Code}}</strong></p>
<div id="game-area"></div></div>
<script>const ROOM_CODE="{{.Room.Code}}";const GAME_TYPE="{{.Room.GameType}}";const USER_ID={{.User.ID}};const USER_PSEUDO="{{.User.Pseudo}}";const IS_HOST={{.IsHost}};</script>
<script src="/static/js/websocket.js"></script>
{{if eq .Room.GameType "blindtest"}}<script src="/static/js/blindtest.js"></script>{{else}}<script src="/static/js/petitbac.js"></script>{{end}}
</body></html>`
	tmpl, _ := template.New("room").Parse(html)
	tmpl.Execute(w, data)
}

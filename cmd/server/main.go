package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"groupie-tracker/internal/auth"
	"groupie-tracker/internal/database"
	"groupie-tracker/internal/games/blindtest"
	"groupie-tracker/internal/games/petitbac"
	"groupie-tracker/internal/rooms"
	"groupie-tracker/internal/spotify"
	"groupie-tracker/internal/websocket"
)

type Config struct {
	Port            string
	DatabasePath    string
	TemplateDir     string
	StaticDir       string
	SpotifyClientID string
	SpotifySecret   string
}

func main() {
	log.Println("=== Groupie-Tracker Server ===")

	config := Config{
		Port:            getEnv("PORT", "8080"),
		DatabasePath:    getEnv("DB_PATH", "./data/groupie.db"),
		TemplateDir:     getEnv("TEMPLATE_DIR", "./web/templates"),
		StaticDir:       getEnv("STATIC_DIR", "./web/static"),
		SpotifyClientID: getEnv("SPOTIFY_CLIENT_ID", ""),
		SpotifySecret:   getEnv("SPOTIFY_CLIENT_SECRET", ""),
	}

	if err := os.MkdirAll("./data", 0755); err != nil {
		log.Printf("[WARN] Impossible de créer le dossier data: %v", err)
	}

	if err := database.Init(config.DatabasePath); err != nil {
		log.Fatalf("[FATAL] Erreur initialisation DB: %v", err)
	}
	defer database.Close()
	log.Println("[OK] Base de données initialisée")

	spotifyClient := spotify.NewClient(spotify.Config{})
	if err := spotifyClient.Authenticate(); err != nil {
		log.Printf("[WARN] Erreur init Deezer: %v", err)
	} else {
		log.Println("[OK] Client Deezer initialisé")
	}

	roomManager := rooms.GetManager()
	blindtestMgr := blindtest.GetGameManager()
	petitbacMgr := petitbac.GetGameManager()
	log.Println("[OK] Managers de jeu initialisés")

	_ = roomManager
	_ = blindtestMgr
	_ = petitbacMgr

	wsHandler := websocket.NewHandler()
	log.Println("[OK] Handler WebSocket initialisé")

	blindtestHandler := blindtest.GetHandler()
	wsHandler.SetBlindTestHandler(blindtestHandler)
	log.Println("[OK] Handler Blind Test connecté")

	petitbacHandler := petitbac.GetHandler()
	wsHandler.SetPetitBacHandler(petitbacHandler)
	log.Println("[OK] Handler Petit Bac connecté")

	authHandler := auth.NewHandler(config.TemplateDir)
	roomHandler := rooms.NewHandler(config.TemplateDir)

	authMiddleware := auth.NewMiddleware()

	mux := http.NewServeMux()

	fileServer := http.FileServer(http.Dir(config.StaticDir))
	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		sessionManager := auth.NewSessionManager()
		user, _ := sessionManager.GetUserFromRequest(r)

		data := map[string]interface{}{
			"Title": "Accueil - Groupie Tracker",
			"User":  user,
		}

		tmpl, err := template.ParseFiles(filepath.Join(config.TemplateDir, "index.html"))
		if err != nil {
			log.Printf("[HTTP] Erreur chargement index.html: %v", err)
			http.Error(w, "Erreur interne", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("[HTTP] Erreur exécution template index: %v", err)
			http.Error(w, "Erreur interne", http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/login", authHandler.HandleLogin)
	mux.HandleFunc("/register", authHandler.HandleRegister)
	mux.HandleFunc("/logout", authHandler.HandleLogout)

	mux.HandleFunc("/accueil", roomHandler.HandleLobby)
	mux.HandleFunc("/lobby", roomHandler.HandleLobby)
	mux.HandleFunc("/rooms", roomHandler.HandleLobby)
	mux.HandleFunc("/room/", roomHandler.HandleRoom)

	mux.HandleFunc("/room/create", func(w http.ResponseWriter, r *http.Request) {
		sessionManager := auth.NewSessionManager()
		user, err := sessionManager.GetUserFromRequest(r)
		if err != nil {
			http.Redirect(w, r, "/login?redirect=/room/create", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodPost {
			roomHandler.HandleCreateRoom(w, r)
			return
		}

		data := map[string]interface{}{
			"Title": "Créer une salle",
			"User":  user,
		}

		tmpl, err := template.ParseFiles(filepath.Join(config.TemplateDir, "create_room.html"))
		if err != nil {
			log.Printf("[HTTP] Erreur chargement create_room.html: %v", err)
			http.Error(w, "Erreur interne", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("[HTTP] Erreur exécution template create_room: %v", err)
			http.Error(w, "Erreur interne", http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/room/join", func(w http.ResponseWriter, r *http.Request) {
		sessionManager := auth.NewSessionManager()
		user, err := sessionManager.GetUserFromRequest(r)
		if err != nil {
			http.Redirect(w, r, "/login?redirect=/room/join", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodPost {
			roomHandler.HandleJoinRoom(w, r)
			return
		}

		data := map[string]interface{}{
			"Title": "Rejoindre une salle",
			"User":  user,
			"Error": r.URL.Query().Get("error"),
		}

		tmpl, err := template.ParseFiles(filepath.Join(config.TemplateDir, "join_room.html"))
		if err != nil {
			log.Printf("[HTTP] Erreur chargement join_room.html: %v", err)
			http.Error(w, "Erreur interne", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("[HTTP] Erreur exécution template join_room: %v", err)
			http.Error(w, "Erreur interne", http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/api/rooms", roomHandler.HandleGetRooms)
	mux.HandleFunc("/api/rooms/create", roomHandler.HandleCreateRoom)
	mux.HandleFunc("/api/rooms/join", roomHandler.HandleJoinRoom)
	mux.HandleFunc("/api/rooms/leave", roomHandler.HandleLeaveRoom)
	mux.Handle("/api/rooms/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			path := r.URL.Path
			if len(path) > 8 && path[len(path)-8:] == "/restart" {
				roomHandler.HandleRestartRoom(w, r)
				return
			}
		}
		http.NotFound(w, r)
	}))

	mux.Handle("/ws/room/", authMiddleware.RequireAuth(http.HandlerFunc(wsHandler.HandleWebSocket)))

	handler := loggingMiddleware(securityHeadersMiddleware(mux))

	server := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("[SERVER] Démarrage sur http://localhost:%s", config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Erreur serveur: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[SERVER] Arrêt en cours...")
	database.Close()
	log.Println("[SERVER] Arrêté proprement")
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[HTTP] %s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
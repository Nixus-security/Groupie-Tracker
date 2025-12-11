// Package main - Point d'entrée du serveur Groupie-Tracker
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

// Config configuration du serveur
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

	// Configuration (variables d'environnement ou valeurs par défaut)
	config := Config{
		Port:            getEnv("PORT", "8080"),
		DatabasePath:    getEnv("DB_PATH", "./data/groupie.db"),
		TemplateDir:     getEnv("TEMPLATE_DIR", "./web/templates"),
		StaticDir:       getEnv("STATIC_DIR", "./web/static"),
		SpotifyClientID: getEnv("SPOTIFY_CLIENT_ID", ""),
		SpotifySecret:   getEnv("SPOTIFY_CLIENT_SECRET", ""),
	}

	// Créer le dossier data si nécessaire
	if err := os.MkdirAll("./data", 0755); err != nil {
		log.Printf("[WARN] Impossible de créer le dossier data: %v", err)
	}

	// Initialiser la base de données
	if err := database.Init(config.DatabasePath); err != nil {
		log.Fatalf("[FATAL] Erreur initialisation DB: %v", err)
	}
	defer database.Close()
	log.Println("[OK] Base de données initialisée")

	// Initialiser le client Spotify si les credentials sont fournis
	if config.SpotifyClientID != "" && config.SpotifySecret != "" {
		spotifyClient := spotify.NewClient(spotify.Config{
			ClientID:     config.SpotifyClientID,
			ClientSecret: config.SpotifySecret,
		})
		if err := spotifyClient.Authenticate(); err != nil {
			log.Printf("[WARN] Erreur auth Spotify: %v", err)
		} else {
			log.Println("[OK] Client Spotify authentifié")
		}
	} else {
		log.Println("[WARN] Credentials Spotify non configurés - Blind Test limité")
	}

	// Initialiser les managers
	roomManager := rooms.GetManager()
	blindtestMgr := blindtest.GetGameManager()
	petitbacMgr := petitbac.GetGameManager()
	log.Println("[OK] Managers de jeu initialisés")

	// Utiliser les managers (éviter les erreurs "unused")
	_ = roomManager
	_ = blindtestMgr
	_ = petitbacMgr

	// Initialiser le handler WebSocket (pas le hub directement)
	wsHandler := websocket.NewHandler()
	log.Println("[OK] Handler WebSocket initialisé")

	// >>> AJOUTER CES LIGNES <<<
	// Connecter le handler Blind Test au WebSocket
	blindtestHandler := blindtest.GetHandler()
	wsHandler.SetBlindTestHandler(blindtestHandler)
	log.Println("[OK] Handler Blind Test connecté")



	// Initialiser les handlers
	authHandler := auth.NewHandler(config.TemplateDir)
	roomHandler := rooms.NewHandler(config.TemplateDir)

	// Initialiser le middleware d'authentification
	authMiddleware := auth.NewMiddleware()

	// Créer le routeur
	mux := http.NewServeMux()

	// ============================================================================
	// FICHIERS STATIQUES
	// ============================================================================
	fileServer := http.FileServer(http.Dir(config.StaticDir))
	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))

	// ============================================================================
	// PAGES HTML
	// ============================================================================

	// Page d'accueil (index.html)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		// Récupérer l'utilisateur s'il est connecté (optionnel)
		sessionManager := auth.NewSessionManager()
		user, _ := sessionManager.GetUserFromRequest(r)

		data := map[string]interface{}{
			"Title": "Accueil - Groupie Tracker",
			"User":  user,
		}

		// Charger le template
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

	// Authentification
	mux.HandleFunc("/login", authHandler.HandleLogin)
	mux.HandleFunc("/register", authHandler.HandleRegister)
	mux.HandleFunc("/logout", authHandler.HandleLogout)

	// Lobby et salles (nécessite authentification)
	mux.HandleFunc("/accueil", roomHandler.HandleLobby)
	mux.HandleFunc("/lobby", roomHandler.HandleLobby)
	mux.HandleFunc("/rooms", roomHandler.HandleLobby)
	mux.HandleFunc("/room/", roomHandler.HandleRoom)

	// Page de création de salle
	mux.HandleFunc("/room/create", func(w http.ResponseWriter, r *http.Request) {
		// Vérifier l'authentification
		sessionManager := auth.NewSessionManager()
		user, err := sessionManager.GetUserFromRequest(r)
		if err != nil {
			http.Redirect(w, r, "/login?redirect=/room/create", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodPost {
			// Traiter la création de salle
			roomHandler.HandleCreateRoom(w, r)
			return
		}

		// Afficher le formulaire
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

	// Page de jonction avec code
	mux.HandleFunc("/room/join", func(w http.ResponseWriter, r *http.Request) {
		// Vérifier l'authentification
		sessionManager := auth.NewSessionManager()
		user, err := sessionManager.GetUserFromRequest(r)
		if err != nil {
			http.Redirect(w, r, "/login?redirect=/room/join", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodPost {
			// Traiter la jonction
			roomHandler.HandleJoinRoom(w, r)
			return
		}

		// Afficher le formulaire
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

	// ============================================================================
	// API REST
	// ============================================================================

	// API Salles
	mux.HandleFunc("/api/rooms", roomHandler.HandleGetRooms)
	mux.HandleFunc("/api/rooms/create", roomHandler.HandleCreateRoom)
	mux.HandleFunc("/api/rooms/join", roomHandler.HandleJoinRoom)
	mux.HandleFunc("/api/rooms/leave", roomHandler.HandleLeaveRoom)
	mux.Handle("/api/rooms/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Router pour /api/rooms/{id}/restart
		if r.Method == http.MethodPost {
			path := r.URL.Path
			if len(path) > 8 && path[len(path)-8:] == "/restart" {
				roomHandler.HandleRestartRoom(w, r)
				return
			}
		}
		http.NotFound(w, r)
	}))

	// ============================================================================
	// WEBSOCKET (avec middleware d'authentification)
	// ============================================================================
	mux.Handle("/ws/room/", authMiddleware.RequireAuth(http.HandlerFunc(wsHandler.HandleWebSocket)))

	// ============================================================================
	// MIDDLEWARE & SERVEUR
	// ============================================================================

	// Wrapper avec middlewares
	handler := loggingMiddleware(securityHeadersMiddleware(mux))

	server := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Démarrage du serveur en goroutine
	go func() {
		log.Printf("[SERVER] Démarrage sur http://localhost:%s", config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Erreur serveur: %v", err)
		}
	}()

	// Gestion du shutdown gracieux
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[SERVER] Arrêt en cours...")
	database.Close()
	log.Println("[SERVER] Arrêté proprement")
}

// ============================================================================
// MIDDLEWARES
// ============================================================================

// loggingMiddleware log les requêtes HTTP
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[HTTP] %s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// securityHeadersMiddleware ajoute les headers de sécurité
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// ============================================================================
// UTILITAIRES
// ============================================================================

// getEnv récupère une variable d'environnement ou retourne la valeur par défaut
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
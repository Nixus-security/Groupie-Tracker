// Package main - Point d'entrée du serveur Groupie-Tracker
package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
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

	// Initialiser le hub WebSocket
	wsHub := websocket.GetHub()
	log.Println("[OK] Hub WebSocket initialisé")

	// Initialiser les handlers
	authHandler := auth.NewHandler(config.TemplateDir)
	roomHandler := rooms.NewHandler(config.TemplateDir)

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

	// Page d'accueil
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/lobby", http.StatusSeeOther)
	})

	// Authentification
	mux.HandleFunc("/login", authHandler.HandleLogin)
	mux.HandleFunc("/register", authHandler.HandleRegister)
	mux.HandleFunc("/logout", authHandler.HandleLogout)

	// Lobby et salles
	mux.HandleFunc("/lobby", roomHandler.HandleLobby)
	mux.HandleFunc("/room/", roomHandler.HandleRoom)

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
	// WEBSOCKET
	// ============================================================================
	mux.HandleFunc("/ws/room/", wsHub.HandleConnection)

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
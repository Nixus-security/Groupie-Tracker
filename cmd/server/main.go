package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"music-platform/internal/config"
	"music-platform/internal/database"
	"music-platform/internal/handlers"
	"music-platform/internal/middleware"
	"music-platform/internal/websocket"
)

var templates *template.Template

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg := config.Load()

	if err := database.Init(cfg.DatabasePath); err != nil {
		log.Fatal("Erreur base de données:", err)
	}
	defer database.Close()

	if err := database.RunMigrations(); err != nil {
		log.Fatal("Erreur migrations:", err)
	}

	var err error
	templates, err = loadTemplates("./web/templates")
	if err != nil {
		log.Fatal("Erreur templates:", err)
	}

	hub := websocket.NewHub()
	go hub.Run()

	handlers.Setup(templates, hub, cfg)

	mux := setupRoutes(hub)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Serveur démarré sur http://localhost:%s", cfg.Port)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal("Erreur serveur:", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Arrêt du serveur...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hub.Shutdown()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Erreur arrêt serveur: %v", err)
	}

	log.Println("Serveur arrêté")
}

func loadTemplates(basePath string) (*template.Template, error) {
	tmpl := template.New("").Funcs(template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"add":      func(a, b int) int { return a + b },
		"sub":      func(a, b int) int { return a - b },
	})

	patterns := []string{
		filepath.Join(basePath, "layouts", "*.html"),
		filepath.Join(basePath, "pages", "*.html"),
		filepath.Join(basePath, "components", "*.html"),
	}

	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if _, err := tmpl.ParseFiles(f); err != nil {
				return nil, err
			}
		}
	}

	return tmpl, nil
}

func setupRoutes(hub *websocket.Hub) *http.ServeMux {
	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir("./web/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("/", handlers.Home)
	mux.HandleFunc("/login", handlers.Login)
	mux.HandleFunc("/register", handlers.Register)
	mux.HandleFunc("/logout", handlers.Logout)

	mux.HandleFunc("/lobby", middleware.Auth(handlers.Lobby))
	mux.HandleFunc("/rooms/create", middleware.Auth(handlers.CreateRoom))
	mux.HandleFunc("/rooms/join", middleware.Auth(handlers.JoinRoom))
	mux.HandleFunc("/room/", middleware.Auth(handlers.Room))

	mux.HandleFunc("/blindtest/", middleware.Auth(handlers.BlindTest))
	mux.HandleFunc("/blindtest/answer", middleware.Auth(handlers.BlindTestAnswer))

	mux.HandleFunc("/petitbac/", middleware.Auth(handlers.PetitBac))
	mux.HandleFunc("/petitbac/submit", middleware.Auth(handlers.PetitBacSubmit))
	mux.HandleFunc("/petitbac/vote", middleware.Auth(handlers.PetitBacVote))

	mux.HandleFunc("/scoreboard/", middleware.Auth(handlers.Scoreboard))
	mux.HandleFunc("/api/scoreboard/", middleware.Auth(handlers.APIScoreboard))

	mux.HandleFunc("/ws/", middleware.Auth(handlers.WebSocket))

	return mux
}
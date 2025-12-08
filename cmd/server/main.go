// Package main est le point d'entrée de l'application
package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"groupie-tracker/internal/auth"
	"groupie-tracker/internal/database"
	"groupie-tracker/internal/games/blindtest"
	"groupie-tracker/internal/games/petitbac"
	"groupie-tracker/internal/models"
	"groupie-tracker/internal/rooms"
	"groupie-tracker/internal/spotify"
	"groupie-tracker/internal/websocket"
)

// templates contient tous les templates chargés
var templates *template.Template

func main() {
	// Configuration du logger
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("🚀 Démarrage de Groupie-Tracker...")

	// Initialiser la base de données
	dbPath := getEnvOrDefault("DB_PATH", "groupie_tracker.db")
	if err := database.Init(dbPath); err != nil {
		log.Fatalf("❌ Erreur initialisation DB: %v", err)
	}
	defer database.Close()
	log.Println("✅ Base de données initialisée")

	// Charger les templates avec les fonctions personnalisées
	templatesDir := "web/templates"
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
	
	var err error
	templates, err = template.New("").Funcs(funcMap).ParseGlob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		log.Printf("⚠️ Erreur chargement templates: %v", err)
	} else {
		log.Println("✅ Templates chargés")
	}

	// Configuration Spotify (à remplacer par vos identifiants)
	spotifyClientID := getEnvOrDefault("SPOTIFY_CLIENT_ID", "")
	spotifyClientSecret := getEnvOrDefault("SPOTIFY_CLIENT_SECRET", "")

	if spotifyClientID != "" && spotifyClientSecret != "" {
		spotifyClient := spotify.NewClient(spotify.Config{
			ClientID:     spotifyClientID,
			ClientSecret: spotifyClientSecret,
		})
		if err := spotifyClient.Authenticate(); err != nil {
			log.Printf("⚠️ Avertissement Spotify: %v", err)
		} else {
			log.Println("✅ Client Spotify initialisé")
		}
	} else {
		log.Println("⚠️ Variables SPOTIFY_CLIENT_ID et SPOTIFY_CLIENT_SECRET non définies")
		log.Println("   Le Blind Test ne fonctionnera pas sans les identifiants Spotify")
	}

	// Initialiser les managers
	_ = rooms.GetManager()
	log.Println("✅ Room Manager initialisé")

	_ = websocket.GetHub()
	log.Println("✅ WebSocket Hub initialisé")

	_ = blindtest.GetManager()
	log.Println("✅ Blind Test Manager initialisé")

	_ = petitbac.GetManager()
	log.Println("✅ Petit Bac Manager initialisé")

	// Créer le routeur
	mux := http.NewServeMux()

	// Servir les fichiers statiques
	fs := http.FileServer(http.Dir("web/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Créer le middleware d'authentification
	authMiddleware := auth.NewMiddleware()

	// Routes d'authentification (utilisent leur propre méthode RegisterRoutes)
	authHandler := auth.NewHandler(templatesDir)
	authHandler.RegisterRoutes(mux, authMiddleware)

	// Routes des salles (utilisent leur propre méthode RegisterRoutes)
	roomHandler := rooms.NewHandler(templatesDir)
	roomHandler.RegisterRoutes(mux, authMiddleware)

	// Routes Petit Bac catégories (CRUD)
	petitbacHandler := petitbac.NewHandler()
	mux.Handle("/api/petitbac/categories", authMiddleware.RequireAuthAPI(http.HandlerFunc(petitbacHandler.CategoriesAPI)))
	mux.Handle("/api/petitbac/categories/", authMiddleware.RequireAuthAPI(http.HandlerFunc(petitbacHandler.CategoryAPI)))

	// WebSocket avec injection des handlers de jeu
	wsHandler := websocket.NewHandler()
	
	// Injecter le handler Blind Test
	btManager := blindtest.GetManager()
	wsHandler.SetBlindTestHandler(func(client *websocket.Client, msg *models.WSMessage) {
		if msg.Type == models.WSTypeBTAnswer {
			if payload, ok := msg.Payload.(map[string]interface{}); ok {
				if answer, ok := payload["answer"].(string); ok {
					btManager.SubmitAnswer(client.RoomCode, client.UserID, client.Pseudo, answer)
				}
			}
		}
	})
	
	// Injecter le handler Petit Bac
	pbManager := petitbac.GetManager()
	wsHandler.SetPetitBacHandler(func(client *websocket.Client, msg *models.WSMessage) {
		switch msg.Type {
		case models.WSTypePBAnswer:
			if payload, ok := msg.Payload.(map[string]interface{}); ok {
				if answersRaw, ok := payload["answers"].(map[string]interface{}); ok {
					answers := make(map[string]string)
					for k, v := range answersRaw {
						if s, ok := v.(string); ok {
							answers[k] = s
						}
					}
					pbManager.SubmitAnswers(client.RoomCode, client.UserID, answers)
				}
			}
		case models.WSTypePBVote:
			if payload, ok := msg.Payload.(map[string]interface{}); ok {
				targetID, _ := payload["target_user_id"].(float64)
				category, _ := payload["category"].(string)
				isValid, _ := payload["is_valid"].(bool)
				pbManager.SubmitVote(client.RoomCode, client.UserID, int64(targetID), category, isValid)
			}
		case models.WSTypePBStopRound:
			pbManager.StopRound(client.RoomCode, client.UserID)
		}
	})
	
	// Route WebSocket avec authentification
	mux.Handle("/ws", authMiddleware.RequireAuthAPI(http.HandlerFunc(wsHandler.HandleWebSocket)))

	// Page d'accueil (avec authentification optionnelle)
	mux.Handle("/", authMiddleware.OptionalAuth(http.HandlerFunc(handleHome)))

	// Configuration du serveur
	port := getEnvOrDefault("PORT", "8080")
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("🎮 Serveur démarré sur http://localhost:%s", port)
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("Endpoints disponibles:")
	log.Println("  GET  /              - Page d'accueil")
	log.Println("  GET  /register      - Inscription")
	log.Println("  GET  /login         - Connexion")
	log.Println("  GET  /rooms         - Liste des salles")
	log.Println("  GET  /room/create   - Créer une salle")
	log.Println("  GET  /room/join     - Rejoindre une salle")
	log.Println("  GET  /room/{code}   - Salle de jeu")
	log.Println("  WS   /ws?room={code}- WebSocket")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("❌ Erreur serveur: %v", err)
	}
}

// handleHome gère la page d'accueil
func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Vérifier si l'utilisateur est connecté
	user := auth.GetUserFromContext(r.Context())
	
	data := map[string]interface{}{
		"Title": "Accueil",
		"User":  user,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	if templates != nil {
		if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
			log.Printf("❌ Erreur template index: %v", err)
			// Fallback HTML simple
			renderFallbackHome(w, user)
		}
	} else {
		renderFallbackHome(w, user)
	}
}

// renderFallbackHome affiche une page d'accueil simple si les templates ne sont pas chargés
func renderFallbackHome(w http.ResponseWriter, user *models.User) {
	html := `<!DOCTYPE html>
<html lang="fr">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Groupie-Tracker</title>
    <link rel="stylesheet" href="/static/css/style.css">
</head>
<body>
    <div class="container">
        <header class="text-center" style="padding: 64px 0;">
            <h1>🎵 Groupie-Tracker</h1>
            <p class="text-muted">Plateforme de jeux musicaux multijoueur</p>
        </header>
        
        <main>
            <div class="d-grid gap-lg" style="grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));">
                <div class="card">
                    <div class="game-icon">🎧</div>
                    <h2>Blind Test</h2>
                    <p class="text-muted">Devinez le titre de la chanson le plus vite possible !</p>
                </div>
                
                <div class="card">
                    <div class="game-icon">🔤</div>
                    <h2>Petit Bac Musical</h2>
                    <p class="text-muted">Trouvez des réponses musicales pour chaque lettre !</p>
                </div>
            </div>
            
            <div class="text-center mt-lg">`

	if user != nil {
		html += `
                <p>Bienvenue, <strong>` + user.Pseudo + `</strong> !</p>
                <div class="d-flex gap-md justify-center" style="flex-wrap: wrap;">
                    <a href="/rooms" class="btn btn-primary">Voir les salles</a>
                    <a href="/room/create" class="btn btn-success">Créer une salle</a>
                    <a href="/room/join" class="btn btn-secondary">Rejoindre</a>
                    <a href="/logout" class="btn btn-danger">Déconnexion</a>
                </div>`
	} else {
		html += `
                <p class="text-muted">Connectez-vous pour jouer !</p>
                <div class="d-flex gap-md justify-center">
                    <a href="/login" class="btn btn-primary">Connexion</a>
                    <a href="/register" class="btn btn-secondary">Inscription</a>
                </div>`
	}

	html += `
            </div>
        </main>
        
        <footer class="text-center mt-lg text-muted">
            <p>© 2024 Groupie-Tracker - Projet Go</p>
        </footer>
    </div>
</body>
</html>`

	w.Write([]byte(html))
}

// getEnvOrDefault retourne la valeur de la variable d'environnement ou une valeur par défaut
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
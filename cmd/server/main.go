// Package main est le point d'entrée de l'application
package main

import (
	"log"
	"net/http"
	"os"
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

func main() {
	// Configuration du logger
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("🚀 Démarrage de Groupie-Tracker...")

	// Initialiser la base de données
	if err := database.Initialize(); err != nil {
		log.Fatalf("❌ Erreur initialisation DB: %v", err)
	}
	defer database.Close()
	log.Println("✅ Base de données initialisée")

	// Configuration Spotify (à remplacer par vos identifiants)
	spotifyClientID := getEnvOrDefault("SPOTIFY_CLIENT_ID", "")
	spotifyClientSecret := getEnvOrDefault("SPOTIFY_CLIENT_SECRET", "")

	if spotifyClientID != "" && spotifyClientSecret != "" {
		if err := spotify.Initialize(spotifyClientID, spotifyClientSecret); err != nil {
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

	// Routes d'authentification
	authHandler := auth.NewHandler()
	mux.HandleFunc("/register", authHandler.RegisterPage)
	mux.HandleFunc("/login", authHandler.LoginPage)
	mux.HandleFunc("/logout", authHandler.Logout)
	mux.HandleFunc("/api/auth/register", authHandler.RegisterAPI)
	mux.HandleFunc("/api/auth/login", authHandler.LoginAPI)
	mux.HandleFunc("/api/auth/logout", authHandler.LogoutAPI)
	mux.HandleFunc("/api/auth/me", authHandler.MeAPI)

	// Routes des salles
	roomHandler := rooms.NewHandler()
	mux.HandleFunc("/rooms", auth.RequireAuth(roomHandler.ListRooms))
	mux.HandleFunc("/room/create", auth.RequireAuth(roomHandler.CreateRoomPage))
	mux.HandleFunc("/room/join", auth.RequireAuth(roomHandler.JoinRoomPage))
	mux.HandleFunc("/room/", auth.RequireAuth(roomHandler.RoomPage))
	mux.HandleFunc("/api/rooms", auth.RequireAuthAPI(roomHandler.ListRoomsAPI))
	mux.HandleFunc("/api/rooms/create", auth.RequireAuthAPI(roomHandler.CreateRoomAPI))
	mux.HandleFunc("/api/rooms/join", auth.RequireAuthAPI(roomHandler.JoinRoomAPI))
	mux.HandleFunc("/api/rooms/leave", auth.RequireAuthAPI(roomHandler.LeaveRoomAPI))
	mux.HandleFunc("/api/rooms/ready", auth.RequireAuthAPI(roomHandler.SetReadyAPI))
	mux.HandleFunc("/api/rooms/config", auth.RequireAuthAPI(roomHandler.UpdateConfigAPI))
	mux.HandleFunc("/api/rooms/start", auth.RequireAuthAPI(roomHandler.StartGameAPI))
	mux.HandleFunc("/api/rooms/info", auth.RequireAuthAPI(roomHandler.RoomInfoAPI))

	// Routes Petit Bac catégories (CRUD)
	petitbacHandler := petitbac.NewHandler()
	mux.HandleFunc("/api/petitbac/categories", auth.RequireAuthAPI(petitbacHandler.CategoriesAPI))
	mux.HandleFunc("/api/petitbac/categories/", auth.RequireAuthAPI(petitbacHandler.CategoryAPI))

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
	
	mux.HandleFunc("/ws", auth.RequireAuthAPI(wsHandler.HandleWebSocket))

	// Page d'accueil et jeux
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/blindtest/", auth.RequireAuth(handleBlindTest))
	mux.HandleFunc("/petitbac/", auth.RequireAuth(handlePetitBac))

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
	
	html := `<!DOCTYPE html>
<html lang="fr">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Groupie-Tracker - Jeux Musicaux Multijoueur</title>
    <link rel="stylesheet" href="/static/css/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>🎵 Groupie-Tracker</h1>
            <p>Plateforme de jeux musicaux multijoueur</p>
        </header>
        
        <main>
            <section class="games">
                <div class="game-card">
                    <h2>🎧 Blind Test</h2>
                    <p>Devinez le titre de la chanson le plus vite possible !</p>
                    <ul>
                        <li>Playlists: Rock, Rap, Pop</li>
                        <li>37 secondes par manche</li>
                        <li>Points selon la rapidité</li>
                    </ul>
                </div>
                
                <div class="game-card">
                    <h2>🎼 Petit Bac Musical</h2>
                    <p>Trouvez des réponses musicales pour chaque lettre !</p>
                    <ul>
                        <li>Catégories: Artiste, Album, Groupe...</li>
                        <li>9 manches</li>
                        <li>Validation collective</li>
                    </ul>
                </div>
            </section>
            
            <section class="actions">`

	if user != nil {
		html += `
                <p>Bienvenue, <strong>` + user.Pseudo + `</strong> !</p>
                <a href="/rooms" class="btn btn-primary">Voir les salles</a>
                <a href="/room/create" class="btn btn-success">Créer une salle</a>
                <a href="/room/join" class="btn btn-secondary">Rejoindre une salle</a>
                <a href="/logout" class="btn btn-danger">Déconnexion</a>`
	} else {
		html += `
                <p>Connectez-vous pour jouer !</p>
                <a href="/login" class="btn btn-primary">Connexion</a>
                <a href="/register" class="btn btn-secondary">Inscription</a>`
	}

	html += `
            </section>
        </main>
        
        <footer>
            <p>© 2024 Groupie-Tracker - Projet Go</p>
        </footer>
    </div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// handleBlindTest gère la page du Blind Test
func handleBlindTest(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="fr">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Blind Test - Groupie-Tracker</title>
    <link rel="stylesheet" href="/static/css/style.css">
</head>
<body>
    <div class="container">
        <h1>🎧 Blind Test</h1>
        <div id="game-container">
            <div id="round-info">
                <span id="round-number">Manche 0/10</span>
                <span id="timer">37s</span>
            </div>
            
            <div id="audio-player">
                <audio id="preview-audio" controls></audio>
            </div>
            
            <div id="answer-form">
                <input type="text" id="answer-input" placeholder="Votre réponse..." autocomplete="off">
                <button id="submit-answer" class="btn btn-primary">Envoyer</button>
            </div>
            
            <div id="players-list">
                <!-- Liste des joueurs avec leurs scores -->
            </div>
            
            <div id="results" style="display: none;">
                <!-- Résultats de la manche -->
            </div>
        </div>
    </div>
    <script src="/static/js/websocket.js"></script>
    <script src="/static/js/blindtest.js"></script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// handlePetitBac gère la page du Petit Bac
func handlePetitBac(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="fr">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Petit Bac Musical - Groupie-Tracker</title>
    <link rel="stylesheet" href="/static/css/style.css">
</head>
<body>
    <div class="container">
        <h1>🎼 Petit Bac Musical</h1>
        <div id="game-container">
            <div id="round-info">
                <span id="round-number">Manche 0/9</span>
                <span id="current-letter" class="big-letter">?</span>
            </div>
            
            <div id="categories-form">
                <!-- Formulaire dynamique avec les catégories -->
            </div>
            
            <div id="actions">
                <button id="submit-answers" class="btn btn-primary">Soumettre</button>
                <button id="stop-round" class="btn btn-danger">Stop !</button>
            </div>
            
            <div id="voting-section" style="display: none;">
                <!-- Section de vote -->
            </div>
            
            <div id="players-scores">
                <!-- Scores des joueurs -->
            </div>
        </div>
    </div>
    <script src="/static/js/websocket.js"></script>
    <script src="/static/js/petitbac.js"></script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// getEnvOrDefault retourne la valeur de la variable d'environnement ou une valeur par défaut
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
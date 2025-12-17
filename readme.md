🎵 Groupie-Tracker

📋 Table des matières

Fonctionnalités
Technologies
Architecture
Installation
Utilisation
Structure du projet
API & WebSocket
Contributeurs


✨ Fonctionnalités

🎮 Deux jeux disponibles :


🎧 Blind Test

Devinez le titre ou l'artiste à partir d'extraits de 30 secondes
Musiques issues de l'API Deezer (Top charts)
Points selon la rapidité de réponse (37 secondes par manche)
Visualiseur audio en temps réel
10 manches par partie

🔤 Petit Bac Musical

Trouvez des mots du monde musical commençant par une lettre aléatoire
5 catégories : Artiste, Album, Groupe, Instrument, Featuring
Validation collective par vote
9 manches personnalisables
Temps par manche configurable (30-120s)

👥 Système multijoueur

Salles privées avec code à 6 caractères
2 à 8 joueurs par salle
Mode solo disponible pour l'entraînement
WebSocket pour une expérience temps réel fluide
Système de prêt/hôte

🔐 Authentification sécurisée

Inscription avec validation CNIL (12 caractères min, majuscules, chiffres, caractères spéciaux)
Connexion par pseudo ou email
Sessions avec cookies sécurisés (24h)
Hashage bcrypt des mots de passe

🎨 Interface moderne

Design responsive avec système de design tokens
Animations fluides et feedback visuel
Icônes personnalisées
Thème sombre avec accents néon


🛠 Technologies
Backend

Go 1.21+ - Langage principal
SQLite - Base de données embarquée
Gorilla WebSocket - Communication temps réel
bcrypt - Sécurité des mots de passe

Frontend

HTML5/CSS3 - Interface utilisateur
JavaScript Vanilla - Logique client
WebSocket API - Temps réel
Web Audio API - Visualiseur audio

API Externe

Deezer API - Catalogue musical avec previews 30s


🏗 Architecture
groupie-tracker/
├── cmd/
│   └── server/
│       └── main.go              # Point d'entrée
├── internal/
│   ├── auth/                    # Authentification
│   │   ├── handler.go           # Routes auth
│   │   ├── service.go           # Logique métier
│   │   ├── session.go           # Gestion sessions
│   │   └── middleware.go        # Middlewares auth
│   ├── database/                # Base de données
│   │   ├── database.go          # Connexion SQLite
│   │   └── migrations.go        # Migrations
│   ├── games/                   # Logique des jeux
│   │   ├── blindtest/
│   │   │   ├── game.go          # Logique Blind Test
│   │   │   └── handler.go       # WebSocket Blind Test
│   │   └── petitbac/
│   │       ├── game.go          # Logique Petit Bac
│   │       └── handler.go       # WebSocket Petit Bac
│   ├── rooms/                   # Gestion des salles
│   │   ├── manager.go           # Manager singleton
│   │   ├── handler.go           # Routes HTTP
│   │   └── service.go           # Persistance
│   ├── spotify/                 # Intégration Deezer
│   │   └── client.go            # Client API Deezer
│   ├── websocket/               # WebSocket
│   │   ├── hub.go               # Hub central
│   │   ├── client.go            # Client WebSocket
│   │   └── handler.go           # Routage messages
│   └── models/
│       └── models.go            # Structures de données
├── web/
│   ├── static/
│   │   ├── style.css            # Styles principaux
│   │   ├── icons.css            # Icônes
│   │   ├── room.js              # Gestion salles
│   │   ├── websocket.js         # Client WebSocket
│   │   ├── blindtest.js         # UI Blind Test
│   │   ├── petitbac.js          # UI Petit Bac
│   │   └── audio-sphere-visualizer.js  # Visualiseur
│   └── templates/
│       ├── index.html           # Page d'accueil
│       ├── login.html           # Connexion
│       ├── register.html        # Inscription
│       ├── rooms.html           # Liste salles
│       ├── create_room.html     # Création salle
│       ├── join_room.html       # Rejoindre salle
│       ├── room_blindtest.html  # Salle Blind Test
│       └── room_petitbac.html   # Salle Petit Bac
├── data/
│   └── groupie.db               # Base SQLite (auto-créée)
├── go.mod
├── go.sum
└── README.md

🚀 Installation
Prérequis

Go 1.21+ installé (télécharger)
Git installé

Étapes:

Cloner le repository:

bashgit clone https://github.com/votre-username/groupie-tracker.git
cd groupie-tracker

Installer les dépendances:

-go mod download

Lancer le serveur:

-go run cmd/server/main.go

Le serveur démarre sur http://localhost:8080

Variables d'environnement (optionnel):

bashexport PORT=8080                    # Port du serveur (défaut: 8080)
export DB_PATH=./data/groupie.db    # Chemin base de données
export TEMPLATE_DIR=./web/templates # Dossier templates
export STATIC_DIR=./web/static      # Dossier statiques

🎯 Utilisation
1. Créer un compte

Accédez à /register
Pseudo : 3-30 caractères avec au moins une majuscule
Mot de passe : 12 caractères min (majuscule, minuscule, chiffre, caractère spécial)

2. Créer une salle

Cliquez sur "Créer une salle"
Choisissez le type de jeu (Blind Test ou Petit Bac)
Nommez votre salle
Pour Petit Bac : configurez les catégories, temps et nombre de manches

3. Inviter des joueurs

Partagez le code à 6 caractères affiché en haut de la salle
Les joueurs peuvent rejoindre via "Rejoindre avec code"
Maximum 8 joueurs par salle

4. Lancer la partie

Tous les joueurs cliquent sur "Prêt"
L'hôte lance la partie avec "Démarrer"
Mode solo possible (1 seul joueur)

5. Jouer !
Blind Test

Écoutez l'extrait de 30 secondes
Tapez le titre ou l'artiste dans le chat
Points selon la rapidité (100-150 pts)
10 manches au total

Petit Bac Musical

Une lettre est tirée au sort
Trouvez un mot pour chaque catégorie commençant par cette lettre
Soumettez vos réponses avant la fin du temps
Votez pour valider les réponses des autres
Points : 0 (rejeté), 5 (validé avec doublons), 10 (unique)

6. Résultats

Classement final avec scores
L'hôte peut relancer une nouvelle partie


📡 API & WebSocket
Routes HTTP
Authentification
POST   /login              # Connexion
POST   /register           # Inscription
GET    /logout             # Déconnexion
Salles
GET    /rooms              # Liste des salles
GET    /room/create        # Formulaire création
POST   /api/rooms/create   # Créer une salle
POST   /room/join          # Rejoindre avec code
GET    /room/{code}        # Afficher salle
POST   /api/rooms/{id}/restart  # Redémarrer (hôte)
Messages WebSocket
Messages généraux
javascript{type: "join_room", payload: {room_id: "..."}}
{type: "player_ready", payload: {ready: true}}
{type: "start_game"}
{type: "leave_room"}
Blind Test
javascript// Client → Serveur
{type: "bt_answer", payload: {answer: "Titre ou Artiste"}}

// Serveur → Client
{type: "bt_preload", payload: {preview_url: "...", round: 1, total: 10}}
{type: "bt_new_round", payload: {round: 1, total: 10, preview_url: "...", duration: 37}}
{type: "bt_result", payload: {is_correct: true, points: 120}}
{type: "bt_reveal", payload: {track_name: "...", artist_name: "..."}}
{type: "player_found", payload: {user_id: 1, pseudo: "Player", points: 120}}
{type: "bt_scores", payload: [{user_id: 1, pseudo: "Player", score: 350}, ...]}
{type: "bt_game_end", payload: {winner: "Player", scores: [...]}}
Petit Bac
javascript// Client → Serveur
{type: "submit_answers", payload: {answers: {artiste: "Adele", album: "21", ...}}}
{type: "stop_round"}  // Tous ont fini
{type: "submit_votes", payload: {votes: {1: {artiste: "accept", ...}, ...}}}

// Serveur → Client
{type: "pb_new_round", payload: {round: 1, total: 9, letter: "A", categories: [...], duration: 60}}
{type: "pb_vote_result", payload: {answers: {...}, votes_needed: true}}
{type: "pb_scores", payload: [{user_id: 1, pseudo: "Player", score: 45}, ...]}
{type: "pb_game_end", payload: {winner: "Player", scores: [...]}}
Base de données
Tables principales
sqlusers               # Utilisateurs (id, pseudo, email, password_hash)
sessions            # Sessions actives (id, user_id, expires_at)
rooms               # Salles (id, code, name, host_id, game_type, status)
room_players        # Joueurs dans salles (room_id, user_id, score)
game_scores         # Historique scores (room_id, user_id, score, round_scores)

🎨 Design System
Couleurs principales
css--primary: #6366f1     /* Indigo */
--success: #10b981     /* Vert */
--danger: #ef4444      /* Rouge */
--warning: #f59e0b     /* Orange */
--info: #3b82f6        /* Bleu */
Typographie
css--font-sans: 'Inter', sans-serif
--font-mono: 'Courier New', monospace

🤝 Contributeurs
Ce projet a été réalisé dans le cadre d'un projet pédagogique Go.
Fonctionnalités principales développées

✅ Authentification sécurisée (CNIL)
✅ WebSocket temps réel avec hub
✅ Intégration API Deezer
✅ Blind Test avec visualiseur audio
✅ Petit Bac Musical avec système de votes
✅ Gestion des salles multijoueurs
✅ Interface responsive moderne


🐛 Bugs connus & Améliorations futures
Bugs connus

Le visualiseur audio peut avoir des problèmes sur Safari
Déconnexions WebSocket nécessitent un rafraîchissement manuel

Améliorations prévues

 Classement global persistant
 Historique des parties jouées
 Plus de playlists Spotify/Deezer
 Catégories personnalisées pour Petit Bac
 Mode tournoi
 Chat en salle
 Sons et effets sonores


📞 Support
Pour toute question ou problème :

Vérifiez que Go 1.21+ est installé
Consultez les logs du serveur ([TAG] Message)
Ouvrez une issue sur GitHub
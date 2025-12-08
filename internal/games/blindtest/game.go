// Package blindtest gère la logique du jeu Blind Test
package blindtest

import (
	"log"
	"strings"
	"sync"
	"time"
	"unicode"

	"groupie-tracker/internal/models"
	"groupie-tracker/internal/rooms"
	"groupie-tracker/internal/spotify"
	"groupie-tracker/internal/websocket"
)

// GameManager gère toutes les parties de Blind Test en cours
type GameManager struct {
	games   map[string]*Game // roomCode -> Game
	mutex   sync.RWMutex
	hub     *websocket.Hub
	rooms   *rooms.Manager
	spotify *spotify.Client
}

// Game représente une partie de Blind Test
type Game struct {
	RoomCode     string
	Tracks       []*models.SpotifyTrack
	CurrentRound int
	TotalRounds  int
	TimePerRound int
	Scores       map[int64]int       // userID -> score total
	RoundScores  map[int64][]int     // userID -> scores par manche
	Answers      map[int64]*Answer   // Réponses de la manche en cours
	CurrentTrack *models.SpotifyTrack
	RoundStart   time.Time
	Status       string // "waiting", "playing", "revealing", "finished"
	Timer        *time.Timer
	Mutex        sync.RWMutex
}

// Answer représente une réponse d'un joueur
type Answer struct {
	UserID    int64
	Pseudo    string
	Answer    string
	Timestamp time.Time
	Correct   bool
	Points    int
}

// Points selon la rapidité
const (
	PointsFirst  = 5
	PointsSecond = 3
	PointsThird  = 2
	PointsOther  = 1
)

var (
	managerInstance *GameManager
	managerOnce     sync.Once
)

// GetManager retourne l'instance singleton du GameManager
func GetManager() *GameManager {
	managerOnce.Do(func() {
		managerInstance = &GameManager{
			games:   make(map[string]*Game),
			hub:     websocket.GetHub(),
			rooms:   rooms.GetManager(),
			spotify: spotify.GetClient(),
		}
	})
	return managerInstance
}

// StartGame démarre une nouvelle partie de Blind Test
func (gm *GameManager) StartGame(roomCode string) error {
	room, err := gm.rooms.GetRoom(roomCode)
	if err != nil {
		return err
	}

	// Récupérer les pistes depuis Spotify
	tracks, err := gm.spotify.GetRandomTracksForBlindTest(room.Config.Playlist, 10)
	if err != nil {
		return err
	}

	game := &Game{
		RoomCode:     roomCode,
		Tracks:       tracks,
		CurrentRound: 0,
		TotalRounds:  len(tracks),
		TimePerRound: room.Config.TimePerRound,
		Scores:       make(map[int64]int),
		RoundScores:  make(map[int64][]int),
		Answers:      make(map[int64]*Answer),
		Status:       "playing",
	}

	// Initialiser les scores pour chaque joueur
	room.Mutex.RLock()
	for userID := range room.Players {
		game.Scores[userID] = 0
		game.RoundScores[userID] = make([]int, 0)
	}
	room.Mutex.RUnlock()

	gm.mutex.Lock()
	gm.games[roomCode] = game
	gm.mutex.Unlock()

	// Démarrer la première manche
	gm.startRound(game)

	return nil
}

// startRound démarre une nouvelle manche
func (gm *GameManager) startRound(game *Game) {
	game.Mutex.Lock()
	
	game.CurrentRound++
	if game.CurrentRound > game.TotalRounds {
		game.Mutex.Unlock()
		gm.endGame(game)
		return
	}

	game.CurrentTrack = game.Tracks[game.CurrentRound-1]
	game.Answers = make(map[int64]*Answer)
	game.RoundStart = time.Now()
	game.Status = "playing"

	roundInfo := map[string]interface{}{
		"round":        game.CurrentRound,
		"total_rounds": game.TotalRounds,
		"preview_url":  game.CurrentTrack.PreviewURL,
		"image_url":    game.CurrentTrack.ImageURL,
		"duration":     game.TimePerRound,
	}
	roomCode := game.RoomCode
	timePerRound := game.TimePerRound
	
	game.Mutex.Unlock()

	// Notifier les joueurs de la nouvelle manche
	gm.hub.Broadcast(roomCode, &models.WSMessage{
		Type:    models.WSTypeBTNewRound,
		Payload: roundInfo,
	})

	log.Printf("🎵 Blind Test %s: Manche %d - %s", roomCode, game.CurrentRound, game.CurrentTrack.Name)

	// Timer pour la fin de manche
	game.Timer = time.AfterFunc(time.Duration(timePerRound)*time.Second, func() {
		gm.endRound(game)
	})
}

// endRound termine la manche en cours
func (gm *GameManager) endRound(game *Game) {
	game.Mutex.Lock()
	defer game.Mutex.Unlock()

	if game.Status != "playing" {
		return
	}
	game.Status = "revealing"

	if game.Timer != nil {
		game.Timer.Stop()
	}

	// Calculer les scores
	gm.calculateRoundScores(game)

	// Construire le classement de la manche
	results := make([]map[string]interface{}, 0)
	for _, answer := range game.Answers {
		results = append(results, map[string]interface{}{
			"user_id": answer.UserID,
			"pseudo":  answer.Pseudo,
			"answer":  answer.Answer,
			"correct": answer.Correct,
			"points":  answer.Points,
		})
	}

	// Envoyer les résultats
	gm.hub.Broadcast(game.RoomCode, &models.WSMessage{
		Type: models.WSTypeBTResult,
		Payload: map[string]interface{}{
			"round":    game.CurrentRound,
			"track":    game.CurrentTrack,
			"results":  results,
			"scores":   game.Scores,
		},
	})

	// Pause avant la prochaine manche
	time.AfterFunc(5*time.Second, func() {
		gm.startRound(game)
	})
}

// SubmitAnswer soumet une réponse d'un joueur
func (gm *GameManager) SubmitAnswer(roomCode string, userID int64, pseudo, answer string) {
	gm.mutex.RLock()
	game, exists := gm.games[roomCode]
	gm.mutex.RUnlock()

	if !exists {
		return
	}

	game.Mutex.Lock()
	defer game.Mutex.Unlock()

	if game.Status != "playing" {
		return
	}

	// Vérifier si le joueur a déjà répondu
	if _, exists := game.Answers[userID]; exists {
		return
	}

	// Vérifier si la réponse est correcte
	correct := isCorrectAnswer(answer, game.CurrentTrack.Name, game.CurrentTrack.Artist)

	game.Answers[userID] = &Answer{
		UserID:    userID,
		Pseudo:    pseudo,
		Answer:    answer,
		Timestamp: time.Now(),
		Correct:   correct,
	}

	log.Printf("🎤 Blind Test %s: %s a répondu '%s' (correct: %v)", roomCode, pseudo, answer, correct)

	// Si réponse correcte, notifier les autres
	if correct {
		gm.hub.Broadcast(roomCode, &models.WSMessage{
			Type: models.WSTypeBTAnswer,
			Payload: map[string]interface{}{
				"user_id": userID,
				"pseudo":  pseudo,
				"correct": true,
			},
		})
	}
}

// calculateRoundScores calcule les points de la manche
func (gm *GameManager) calculateRoundScores(game *Game) {
	var correctAnswers []*Answer
	for _, answer := range game.Answers {
		if answer.Correct {
			correctAnswers = append(correctAnswers, answer)
		}
	}

	// Trier par timestamp
	for i := 0; i < len(correctAnswers)-1; i++ {
		for j := i + 1; j < len(correctAnswers); j++ {
			if correctAnswers[i].Timestamp.After(correctAnswers[j].Timestamp) {
				correctAnswers[i], correctAnswers[j] = correctAnswers[j], correctAnswers[i]
			}
		}
	}

	// Attribuer les points
	for rank, answer := range correctAnswers {
		var points int
		switch rank {
		case 0:
			points = PointsFirst
		case 1:
			points = PointsSecond
		case 2:
			points = PointsThird
		default:
			points = PointsOther
		}

		answer.Points = points
		game.Scores[answer.UserID] += points
		game.RoundScores[answer.UserID] = append(game.RoundScores[answer.UserID], points)
	}

	// 0 point pour ceux qui n'ont pas trouvé
	for userID := range game.Scores {
		if _, exists := game.Answers[userID]; !exists {
			game.RoundScores[userID] = append(game.RoundScores[userID], 0)
		} else if !game.Answers[userID].Correct {
			game.RoundScores[userID] = append(game.RoundScores[userID], 0)
		}
	}
}

// endGame termine la partie
func (gm *GameManager) endGame(game *Game) {
	game.Mutex.Lock()
	game.Status = "finished"
	
	roomCode := game.RoomCode
	scores := make(map[int64]int)
	for k, v := range game.Scores {
		scores[k] = v
	}
	roundScores := make(map[int64][]int)
	for k, v := range game.RoundScores {
		roundScores[k] = v
	}
	game.Mutex.Unlock()

	// Construire le classement final
	rankings := gm.buildRankings(scores)

	// Notifier les joueurs
	gm.hub.Broadcast(roomCode, &models.WSMessage{
		Type: models.WSTypeBTGameEnd,
		Payload: map[string]interface{}{
			"rankings":     rankings,
			"scores":       scores,
			"round_scores": roundScores,
		},
	})

	// Mettre à jour la salle
	gm.rooms.EndGame(roomCode)

	// Sauvegarder les scores
	service := rooms.NewService()
	room, _ := gm.rooms.GetRoom(roomCode)
	service.SaveGameScores(room, roundScores)

	// Supprimer la partie de la mémoire
	gm.mutex.Lock()
	delete(gm.games, roomCode)
	gm.mutex.Unlock()

	log.Printf("🏆 Blind Test %s terminé", roomCode)
}

// buildRankings construit le classement final
func (gm *GameManager) buildRankings(scores map[int64]int) []map[string]interface{} {
	type entry struct {
		UserID int64
		Score  int
	}

	entries := make([]entry, 0, len(scores))
	for userID, score := range scores {
		entries = append(entries, entry{UserID: userID, Score: score})
	}

	// Trier par score décroissant
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].Score < entries[j].Score {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	rankings := make([]map[string]interface{}, 0)
	for rank, e := range entries {
		rankings = append(rankings, map[string]interface{}{
			"rank":    rank + 1,
			"user_id": e.UserID,
			"score":   e.Score,
		})
	}

	return rankings
}

// GetGame retourne une partie en cours
func (gm *GameManager) GetGame(roomCode string) *Game {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	return gm.games[roomCode]
}

// isCorrectAnswer vérifie si une réponse est correcte
func isCorrectAnswer(answer, trackName, artistName string) bool {
	answer = normalizeString(answer)
	trackName = normalizeString(trackName)

	// Vérifier le titre exact ou partiel
	if strings.Contains(trackName, answer) || strings.Contains(answer, trackName) {
		return true
	}

	// Vérifier la similarité
	if similarity(answer, trackName) > 0.8 {
		return true
	}

	return false
}

// normalizeString normalise une chaîne pour la comparaison
func normalizeString(s string) string {
	s = strings.ToLower(s)
	
	result := make([]rune, 0, len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' {
			result = append(result, r)
		}
	}
	
	return strings.TrimSpace(string(result))
}

// similarity calcule la similarité entre deux chaînes (0-1)
func similarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	len1, len2 := len(s1), len(s2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	// Distance de Levenshtein
	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,
				min(matrix[i][j-1]+1, matrix[i-1][j-1]+cost),
			)
		}
	}

	maxLen := float64(max(len1, len2))
	return 1.0 - float64(matrix[len1][len2])/maxLen
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

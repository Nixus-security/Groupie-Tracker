// Package blindtest gère la logique du jeu Blind Test
package blindtest

import (
	"log"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"groupie-tracker/internal/models"
	"groupie-tracker/internal/rooms"
	"groupie-tracker/internal/spotify"
)

// GameState représente l'état d'une partie de Blind Test
type GameState struct {
	RoomID       string                 `json:"room_id"`
	CurrentRound int                    `json:"current_round"`
	TotalRounds  int                    `json:"total_rounds"`
	CurrentTrack *models.SpotifyTrack   `json:"current_track,omitempty"`
	Tracks       []*models.SpotifyTrack `json:"-"` // Ne pas exposer les réponses
	TimeLeft     int                    `json:"time_left"`
	Answers      map[int64]string       `json:"answers"`      // UserID -> réponse
	HasAnswered  map[int64]bool         `json:"has_answered"` // UserID -> a répondu
	IsRevealed   bool                   `json:"is_revealed"`
	Timer        *time.Timer            `json:"-"`
	Mutex        sync.RWMutex           `json:"-"`
}

// GameManager gère toutes les parties de Blind Test actives
type GameManager struct {
	games       map[string]*GameState // RoomID -> GameState
	mutex       sync.RWMutex
	roomManager *rooms.Manager
}

var (
	gameManagerInstance *GameManager
	gameManagerOnce     sync.Once
)

// GetGameManager retourne l'instance singleton du GameManager
func GetGameManager() *GameManager {
	gameManagerOnce.Do(func() {
		gameManagerInstance = &GameManager{
			games:       make(map[string]*GameState),
			roomManager: rooms.GetManager(),
		}
	})
	return gameManagerInstance
}

// StartGame démarre une nouvelle partie de Blind Test
func (gm *GameManager) StartGame(roomID string, genre string, rounds int) (*GameState, error) {
	// Récupérer les pistes depuis Spotify
	client := spotify.GetClient()
	if client == nil {
		log.Println("[BlindTest] Client Spotify non initialisé")
		return nil, spotify.ErrNoToken
	}

	tracks, err := client.GetRandomTracksForBlindTest(genre, rounds)
	if err != nil {
		log.Printf("[BlindTest] Erreur récupération pistes: %v", err)
		return nil, err
	}

	// S'assurer qu'on a assez de pistes
	if len(tracks) < rounds {
		rounds = len(tracks)
	}

	// Créer l'état du jeu
	state := &GameState{
		RoomID:       roomID,
		CurrentRound: 0,
		TotalRounds:  rounds,
		Tracks:       tracks,
		Answers:      make(map[int64]string),
		HasAnswered:  make(map[int64]bool),
		IsRevealed:   false,
	}

	gm.mutex.Lock()
	gm.games[roomID] = state
	gm.mutex.Unlock()

	// Mettre à jour le statut de la salle
	gm.roomManager.UpdateRoomStatus(roomID, models.RoomStatusPlaying)
	gm.roomManager.ResetPlayerScores(roomID)

	log.Printf("[BlindTest] Partie démarrée dans la salle %s avec %d manches", roomID, rounds)
	return state, nil
}

// GetGameState retourne l'état actuel du jeu
func (gm *GameManager) GetGameState(roomID string) *GameState {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	return gm.games[roomID]
}

// NextRound passe à la manche suivante
func (gm *GameManager) NextRound(roomID string) (*RoundInfo, error) {
	state := gm.GetGameState(roomID)
	if state == nil {
		return nil, rooms.ErrRoomNotFound
	}

	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	// Vérifier si le jeu est terminé
	if state.CurrentRound >= state.TotalRounds {
		return nil, nil // Jeu terminé
	}

	// Réinitialiser pour la nouvelle manche
	state.CurrentRound++
	state.CurrentTrack = state.Tracks[state.CurrentRound-1]
	state.TimeLeft = models.BlindTestDefaultTime
	state.Answers = make(map[int64]string)
	state.HasAnswered = make(map[int64]bool)
	state.IsRevealed = false

	log.Printf("[BlindTest] Manche %d/%d - Piste: %s", state.CurrentRound, state.TotalRounds, state.CurrentTrack.Name)

	return &RoundInfo{
		Round:      state.CurrentRound,
		Total:      state.TotalRounds,
		PreviewURL: state.CurrentTrack.PreviewURL,
		Duration:   state.TimeLeft,
	}, nil
}

// RoundInfo informations envoyées aux joueurs pour une manche
type RoundInfo struct {
	Round      int    `json:"round"`
	Total      int    `json:"total"`
	PreviewURL string `json:"preview_url"`
	Duration   int    `json:"duration"`
}

// SubmitAnswer soumet une réponse pour un joueur
func (gm *GameManager) SubmitAnswer(roomID string, userID int64, answer string) (*AnswerResult, error) {
	state := gm.GetGameState(roomID)
	if state == nil {
		return nil, rooms.ErrRoomNotFound
	}

	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	// Vérifier si le joueur a déjà répondu correctement
	if state.HasAnswered[userID] {
		// Vérifier si sa réponse précédente était correcte
		prevAnswer := state.Answers[userID]
		if checkAnswer(prevAnswer, state.CurrentTrack.Name, state.CurrentTrack.Artist) {
			return &AnswerResult{AlreadyAnswered: true}, nil
		}
	}

	// Enregistrer la réponse
	state.Answers[userID] = answer
	state.HasAnswered[userID] = true

	// Vérifier si la réponse est correcte
	isCorrect := checkAnswer(answer, state.CurrentTrack.Name, state.CurrentTrack.Artist)

	// Calculer les points
	points := 0
	if isCorrect {
		// Plus de points si réponse rapide
		points = calculatePoints(state.TimeLeft, models.BlindTestDefaultTime)
		gm.roomManager.AddPlayerScore(roomID, userID, points)
	}

	log.Printf("[BlindTest] Réponse de %d: %s (correct: %v, points: %d)", userID, answer, isCorrect, points)

	return &AnswerResult{
		IsCorrect: isCorrect,
		Points:    points,
	}, nil
}

// AnswerResult résultat d'une réponse
type AnswerResult struct {
	IsCorrect       bool `json:"is_correct"`
	Points          int  `json:"points"`
	AlreadyAnswered bool `json:"already_answered"`
}

// RevealAnswer révèle la réponse de la manche actuelle
func (gm *GameManager) RevealAnswer(roomID string) *RevealInfo {
	state := gm.GetGameState(roomID)
	if state == nil {
		return nil
	}

	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	state.IsRevealed = true

	return &RevealInfo{
		TrackName:  state.CurrentTrack.Name,
		ArtistName: state.CurrentTrack.Artist,
		AlbumName:  state.CurrentTrack.Album,
		ImageURL:   state.CurrentTrack.ImageURL,
	}
}

// RevealInfo informations de révélation
type RevealInfo struct {
	TrackName  string `json:"track_name"`
	ArtistName string `json:"artist_name"`
	AlbumName  string `json:"album_name"`
	ImageURL   string `json:"image_url"`
}

// GetScores retourne les scores actuels
func (gm *GameManager) GetScores(roomID string) []PlayerScore {
	room, err := gm.roomManager.GetRoom(roomID)
	if err != nil {
		return nil
	}

	room.Mutex.RLock()
	defer room.Mutex.RUnlock()

	scores := make([]PlayerScore, 0, len(room.Players))
	for _, player := range room.Players {
		scores = append(scores, PlayerScore{
			UserID: player.UserID,
			Pseudo: player.Pseudo,
			Score:  player.Score,
		})
	}

	// Trier par score décroissant
	for i := 0; i < len(scores)-1; i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].Score > scores[i].Score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	return scores
}

// PlayerScore score d'un joueur
type PlayerScore struct {
	UserID int64  `json:"user_id"`
	Pseudo string `json:"pseudo"`
	Score  int    `json:"score"`
}

// EndGame termine la partie
func (gm *GameManager) EndGame(roomID string) *GameResult {
	state := gm.GetGameState(roomID)
	if state == nil {
		return nil
	}

	// Mettre à jour le statut de la salle
	gm.roomManager.UpdateRoomStatus(roomID, models.RoomStatusFinished)

	scores := gm.GetScores(roomID)

	// Supprimer l'état du jeu
	gm.mutex.Lock()
	delete(gm.games, roomID)
	gm.mutex.Unlock()

	log.Printf("[BlindTest] Partie terminée dans la salle %s", roomID)

	winner := ""
	if len(scores) > 0 {
		winner = scores[0].Pseudo
	}

	return &GameResult{
		Scores: scores,
		Winner: winner,
	}
}

// GameResult résultat final de la partie
type GameResult struct {
	Scores []PlayerScore `json:"scores"`
	Winner string        `json:"winner"`
}

// IsGameOver vérifie si le jeu est terminé
func (gm *GameManager) IsGameOver(roomID string) bool {
	state := gm.GetGameState(roomID)
	if state == nil {
		return true
	}
	state.Mutex.RLock()
	defer state.Mutex.RUnlock()
	return state.CurrentRound >= state.TotalRounds
}

// ============================================================================
// FONCTIONS UTILITAIRES
// ============================================================================

// checkAnswer vérifie si une réponse est correcte
func checkAnswer(answer, trackName, artistName string) bool {
	answer = normalizeString(answer)
	trackName = normalizeString(trackName)
	artistName = normalizeString(artistName)

	// Vérifier si la réponse contient le titre ou l'artiste
	if strings.Contains(answer, trackName) || strings.Contains(trackName, answer) {
		return true
	}
	if strings.Contains(answer, artistName) || strings.Contains(artistName, answer) {
		return true
	}

	// Vérifier la similarité (tolérance aux fautes de frappe)
	if similarity(answer, trackName) > 0.7 || similarity(answer, artistName) > 0.7 {
		return true
	}

	return false
}

// normalizeString normalise une chaîne pour la comparaison
func normalizeString(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	// Supprimer les accents courants
	replacer := strings.NewReplacer(
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"à", "a", "â", "a", "ä", "a",
		"ô", "o", "ö", "o",
		"ù", "u", "û", "u", "ü", "u",
		"î", "i", "ï", "i",
		"ç", "c",
	)
	return replacer.Replace(s)
}

// similarity calcule la similarité entre deux chaînes (Jaro-Winkler simplifié)
func similarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	// Algorithme de distance de Levenshtein simplifié
	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}

	distance := levenshteinDistance(s1, s2)
	return 1.0 - float64(distance)/float64(maxLen)
}

// levenshteinDistance calcule la distance de Levenshtein
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
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

	return matrix[len(s1)][len(s2)]
}

// calculatePoints calcule les points en fonction du temps restant
func calculatePoints(timeLeft, totalTime int) int {
	// Points de base: 100
	// Bonus temps: jusqu'à 50 points supplémentaires
	basePoints := 100
	timeBonus := int(float64(timeLeft) / float64(totalTime) * 50)
	return basePoints + timeBonus
}

// ShuffleTracks mélange les pistes (utilisé pour varier les parties)
func ShuffleTracks(tracks []*models.SpotifyTrack) {
	rand.Shuffle(len(tracks), func(i, j int) {
		tracks[i], tracks[j] = tracks[j], tracks[i]
	})
}
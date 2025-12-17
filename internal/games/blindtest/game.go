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

type GameState struct {
	RoomID       string                 `json:"room_id"`
	CurrentRound int                    `json:"current_round"`
	TotalRounds  int                    `json:"total_rounds"`
	CurrentTrack *models.SpotifyTrack   `json:"current_track,omitempty"`
	Tracks       []*models.SpotifyTrack `json:"-"`
	TimeLeft     int                    `json:"time_left"`
	Answers      map[int64]string       `json:"answers"`
	HasAnswered  map[int64]bool         `json:"has_answered"`
	IsRevealed   bool                   `json:"is_revealed"`
	Timer        *time.Timer            `json:"-"`
	Mutex        sync.RWMutex           `json:"-"`
}

type GameManager struct {
	games       map[string]*GameState
	mutex       sync.RWMutex
	roomManager *rooms.Manager
}

var (
	gameManagerInstance *GameManager
	gameManagerOnce     sync.Once
)

func GetGameManager() *GameManager {
	gameManagerOnce.Do(func() {
		gameManagerInstance = &GameManager{
			games:       make(map[string]*GameState),
			roomManager: rooms.GetManager(),
		}
	})
	return gameManagerInstance
}

func (gm *GameManager) StartGame(roomID string, genre string, rounds int) (*GameState, error) {
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

	if len(tracks) < rounds {
		rounds = len(tracks)
	}

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

	gm.roomManager.UpdateRoomStatus(roomID, models.RoomStatusPlaying)
	gm.roomManager.ResetPlayerScores(roomID)

	log.Printf("[BlindTest] Partie démarrée dans la salle %s avec %d manches", roomID, rounds)
	return state, nil
}

func (gm *GameManager) GetGameState(roomID string) *GameState {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	return gm.games[roomID]
}

func (gm *GameManager) NextRound(roomID string) (*RoundInfo, error) {
	state := gm.GetGameState(roomID)
	if state == nil {
		return nil, rooms.ErrRoomNotFound
	}

	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	if state.CurrentRound >= state.TotalRounds {
		return nil, nil
	}

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

type RoundInfo struct {
	Round      int    `json:"round"`
	Total      int    `json:"total"`
	PreviewURL string `json:"preview_url"`
	Duration   int    `json:"duration"`
}

func (gm *GameManager) SubmitAnswer(roomID string, userID int64, answer string) (*AnswerResult, error) {
	state := gm.GetGameState(roomID)
	if state == nil {
		return nil, rooms.ErrRoomNotFound
	}

	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	if state.HasAnswered[userID] {
		prevAnswer := state.Answers[userID]
		if checkAnswer(prevAnswer, state.CurrentTrack.Name, state.CurrentTrack.Artist) {
			return &AnswerResult{AlreadyAnswered: true}, nil
		}
	}

	state.Answers[userID] = answer
	state.HasAnswered[userID] = true

	isCorrect := checkAnswer(answer, state.CurrentTrack.Name, state.CurrentTrack.Artist)

	points := 0
	if isCorrect {
		points = calculatePoints(state.TimeLeft, models.BlindTestDefaultTime)
		gm.roomManager.AddPlayerScore(roomID, userID, points)
	}

	log.Printf("[BlindTest] Réponse de %d: %s (correct: %v, points: %d)", userID, answer, isCorrect, points)

	return &AnswerResult{
		IsCorrect: isCorrect,
		Points:    points,
	}, nil
}

type AnswerResult struct {
	IsCorrect       bool `json:"is_correct"`
	Points          int  `json:"points"`
	AlreadyAnswered bool `json:"already_answered"`
}

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

type RevealInfo struct {
	TrackName  string `json:"track_name"`
	ArtistName string `json:"artist_name"`
	AlbumName  string `json:"album_name"`
	ImageURL   string `json:"image_url"`
}

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

	for i := 0; i < len(scores)-1; i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].Score > scores[i].Score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	return scores
}

type PlayerScore struct {
	UserID int64  `json:"user_id"`
	Pseudo string `json:"pseudo"`
	Score  int    `json:"score"`
}

func (gm *GameManager) EndGame(roomID string) *GameResult {
	state := gm.GetGameState(roomID)
	if state == nil {
		return nil
	}

	gm.roomManager.UpdateRoomStatus(roomID, models.RoomStatusFinished)

	scores := gm.GetScores(roomID)

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

type GameResult struct {
	Scores []PlayerScore `json:"scores"`
	Winner string        `json:"winner"`
}

func (gm *GameManager) IsGameOver(roomID string) bool {
	state := gm.GetGameState(roomID)
	if state == nil {
		return true
	}
	state.Mutex.RLock()
	defer state.Mutex.RUnlock()
	return state.CurrentRound >= state.TotalRounds
}

func checkAnswer(answer, trackName, artistName string) bool {
	answer = normalizeString(answer)
	trackName = normalizeString(trackName)
	artistName = normalizeString(artistName)

	if strings.Contains(answer, trackName) || strings.Contains(trackName, answer) {
		return true
	}
	if strings.Contains(answer, artistName) || strings.Contains(artistName, answer) {
		return true
	}

	if similarity(answer, trackName) > 0.7 || similarity(answer, artistName) > 0.7 {
		return true
	}

	return false
}

func normalizeString(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
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

func similarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}

	distance := levenshteinDistance(s1, s2)
	return 1.0 - float64(distance)/float64(maxLen)
}

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

func calculatePoints(timeLeft, totalTime int) int {
	basePoints := 100
	timeBonus := int(float64(timeLeft) / float64(totalTime) * 50)
	return basePoints + timeBonus
}

func ShuffleTracks(tracks []*models.SpotifyTrack) {
	rand.Shuffle(len(tracks), func(i, j int) {
		tracks[i], tracks[j] = tracks[j], tracks[i]
	})
}
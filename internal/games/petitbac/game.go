package petitbac

import (
	"log"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"groupie-tracker/internal/models"
	"groupie-tracker/internal/rooms"
)

var AvailableLetters = []string{
	"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
	"N", "O", "P", "R", "S", "T", "V",
}

type GameState struct {
	RoomID         string                       `json:"room_id"`
	CurrentRound   int                          `json:"current_round"`
	TotalRounds    int                          `json:"total_rounds"`
	CurrentLetter  string                       `json:"current_letter"`
	UsedLetters    []string                     `json:"used_letters"`
	Categories     []string                     `json:"categories"`
	Answers        map[int64]map[string]string  `json:"answers"`
	HasSubmitted   map[int64]bool               `json:"has_submitted"`
	Votes          map[int64]map[string][]int64 `json:"votes"`
	RoundStoppedBy int64                        `json:"round_stopped_by"`
	TimeLeft       int                          `json:"time_left"`
	RoundDuration  int                          `json:"round_duration"`
	Phase          GamePhase                    `json:"phase"`
	Timer          *time.Timer                  `json:"-"`
	Mutex          sync.RWMutex                 `json:"-"`
}

type GamePhase string

const (
	PhaseWaiting   GamePhase = "waiting"
	PhaseAnswering GamePhase = "answering"
	PhaseVoting    GamePhase = "voting"
	PhaseResults   GamePhase = "results"
)

const (
	DefaultAnswerTime = 60
	VoteTime          = 30
)

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

func (gm *GameManager) StartGame(roomID string, categories []string, rounds int) (*GameState, error) {
	return gm.StartGameWithDuration(roomID, categories, rounds, DefaultAnswerTime)
}

func (gm *GameManager) StartGameWithDuration(roomID string, categories []string, rounds int, duration int) (*GameState, error) {
	if len(categories) == 0 {
		categories = models.DefaultPetitBacCategories
	}

	if rounds <= 0 || rounds > len(AvailableLetters) {
		rounds = models.NbrsManche
	}

	if duration <= 0 {
		duration = DefaultAnswerTime
	}

	state := &GameState{
		RoomID:        roomID,
		CurrentRound:  0,
		TotalRounds:   rounds,
		Categories:    categories,
		UsedLetters:   []string{},
		RoundDuration: duration,
		Phase:         PhaseWaiting,
	}

	gm.mutex.Lock()
	gm.games[roomID] = state
	gm.mutex.Unlock()

	gm.roomManager.UpdateRoomStatus(roomID, models.RoomStatusPlaying)
	gm.roomManager.ResetPlayerScores(roomID)

	log.Printf("[PetitBac] Partie démarrée dans la salle %s avec %d manches, %d sec/manche, catégories: %v", roomID, rounds, duration, categories)
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

	letter := gm.pickRandomLetter(state.UsedLetters)
	state.UsedLetters = append(state.UsedLetters, letter)

	state.CurrentRound++
	state.CurrentLetter = letter
	state.TimeLeft = state.RoundDuration
	state.Answers = make(map[int64]map[string]string)
	state.HasSubmitted = make(map[int64]bool)
	state.Votes = make(map[int64]map[string][]int64)
	state.RoundStoppedBy = 0
	state.Phase = PhaseAnswering

	log.Printf("[PetitBac] Manche %d/%d - Lettre: %s", state.CurrentRound, state.TotalRounds, letter)

	return &RoundInfo{
		Round:      state.CurrentRound,
		Total:      state.TotalRounds,
		Letter:     letter,
		Categories: state.Categories,
		Duration:   state.RoundDuration,
	}, nil
}

type RoundInfo struct {
	Round      int      `json:"round"`
	Total      int      `json:"total"`
	Letter     string   `json:"letter"`
	Categories []string `json:"categories"`
	Duration   int      `json:"duration"`
}

func (gm *GameManager) SubmitAnswers(roomID string, userID int64, answers map[string]string) error {
	state := gm.GetGameState(roomID)
	if state == nil {
		return rooms.ErrRoomNotFound
	}

	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	if state.Phase != PhaseAnswering {
		return nil
	}

	cleanAnswers := make(map[string]string)
	for _, cat := range state.Categories {
		answer := strings.TrimSpace(answers[cat])
		if len(answer) > 0 && strings.ToUpper(string(answer[0])) == state.CurrentLetter {
			cleanAnswers[cat] = answer
		} else {
			cleanAnswers[cat] = ""
		}
	}

	state.Answers[userID] = cleanAnswers
	state.HasSubmitted[userID] = true

	log.Printf("[PetitBac] Réponses de %d soumises", userID)
	return nil
}

func (gm *GameManager) StopRound(roomID string, userID int64) error {
	state := gm.GetGameState(roomID)
	if state == nil {
		return rooms.ErrRoomNotFound
	}

	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	if state.Phase != PhaseAnswering {
		return nil
	}

	state.RoundStoppedBy = userID
	log.Printf("[PetitBac] Manche arrêtée par le joueur %d", userID)

	return nil
}

func (gm *GameManager) HasPlayerFilledAllCategories(roomID string, userID int64) bool {
	state := gm.GetGameState(roomID)
	if state == nil {
		return false
	}

	state.Mutex.RLock()
	defer state.Mutex.RUnlock()

	answers, exists := state.Answers[userID]
	if !exists {
		return false
	}

	for _, cat := range state.Categories {
		if answer, ok := answers[cat]; !ok || answer == "" {
			return false
		}
	}

	return true
}

func (gm *GameManager) StartVoting(roomID string) *VotingInfo {
	state := gm.GetGameState(roomID)
	if state == nil {
		return nil
	}

	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	state.Phase = PhaseVoting
	state.TimeLeft = VoteTime

	allAnswers := make(map[string]map[int64]string)
	for _, cat := range state.Categories {
		allAnswers[cat] = make(map[int64]string)
		for userID, answers := range state.Answers {
			if answer := answers[cat]; answer != "" {
				allAnswers[cat][userID] = answer
			}
		}
	}

	log.Printf("[PetitBac] Phase de vote démarrée")

	return &VotingInfo{
		Answers:    allAnswers,
		Duration:   VoteTime,
		Categories: state.Categories,
	}
}

type VotingInfo struct {
	Answers    map[string]map[int64]string `json:"answers"`
	Duration   int                         `json:"duration"`
	Categories []string                    `json:"categories"`
}

func (gm *GameManager) SubmitVote(roomID string, voterID int64, targetUserID int64, category string, reject bool) error {
	state := gm.GetGameState(roomID)
	if state == nil {
		return rooms.ErrRoomNotFound
	}

	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	if state.Phase != PhaseVoting {
		return nil
	}

	if voterID == targetUserID {
		return nil
	}

	if state.Votes[targetUserID] == nil {
		state.Votes[targetUserID] = make(map[string][]int64)
	}

	if reject {
		state.Votes[targetUserID][category] = append(state.Votes[targetUserID][category], voterID)
	}

	return nil
}

func (gm *GameManager) CalculateRoundScores(roomID string) *RoundScores {
	state := gm.GetGameState(roomID)
	if state == nil {
		return nil
	}

	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	state.Phase = PhaseResults

	room, err := gm.roomManager.GetRoom(roomID)
	if err != nil {
		return nil
	}

	room.Mutex.RLock()
	totalPlayers := len(room.Players)
	room.Mutex.RUnlock()

	scores := make(map[int64]int)
	details := make(map[int64]map[string]AnswerScore)

	for userID, answers := range state.Answers {
		details[userID] = make(map[string]AnswerScore)

		for category, answer := range answers {
			if answer == "" {
				details[userID][category] = AnswerScore{Answer: "", Points: 0, Rejected: false}
				continue
			}

			rejectVotes := 0
			if state.Votes[userID] != nil {
				rejectVotes = len(state.Votes[userID][category])
			}

			potentialVoters := totalPlayers - 1
			if potentialVoters <= 0 {
				potentialVoters = 1
			}

			rejectThreshold := (potentialVoters + 2) / 3
			rejected := rejectVotes >= rejectThreshold

			if totalPlayers == 1 {
				rejected = false
			}

			points := 0
			if !rejected {
				points = gm.calculateAnswerPoints(category, answer, state)
			}

			scores[userID] += points
			details[userID][category] = AnswerScore{
				Answer:   answer,
				Points:   points,
				Rejected: rejected,
			}
		}
	}

	for userID, pts := range scores {
		gm.roomManager.AddPlayerScore(roomID, userID, pts)
	}

	log.Printf("[PetitBac] Scores de la manche calculés")

	return &RoundScores{
		Scores:  scores,
		Details: details,
	}
}

type AnswerScore struct {
	Answer   string `json:"answer"`
	Points   int    `json:"points"`
	Rejected bool   `json:"rejected"`
}

type RoundScores struct {
	Scores  map[int64]int                    `json:"scores"`
	Details map[int64]map[string]AnswerScore `json:"details"`
}

func (gm *GameManager) calculateAnswerPoints(category, answer string, state *GameState) int {
	answerLower := strings.ToLower(strings.TrimSpace(answer))
	count := 0

	for _, answers := range state.Answers {
		otherAnswer := strings.ToLower(strings.TrimSpace(answers[category]))
		if otherAnswer == answerLower {
			count++
		}
	}

	if count == 1 {
		return 2
	}
	return 1
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

	log.Printf("[PetitBac] Partie terminée dans la salle %s", roomID)

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

func (gm *GameManager) AllPlayersSubmitted(roomID string) bool {
	state := gm.GetGameState(roomID)
	if state == nil {
		return false
	}

	room, err := gm.roomManager.GetRoom(roomID)
	if err != nil {
		return false
	}

	state.Mutex.RLock()
	defer state.Mutex.RUnlock()

	room.Mutex.RLock()
	defer room.Mutex.RUnlock()

	for userID := range room.Players {
		if !state.HasSubmitted[userID] {
			return false
		}
	}

	return true
}

func (gm *GameManager) AnyPlayerFilledAll(roomID string) (bool, int64) {
	state := gm.GetGameState(roomID)
	if state == nil {
		return false, 0
	}

	state.Mutex.RLock()
	defer state.Mutex.RUnlock()

	for userID, answers := range state.Answers {
		if !state.HasSubmitted[userID] {
			continue
		}

		allFilled := true
		for _, cat := range state.Categories {
			if answer, ok := answers[cat]; !ok || answer == "" {
				allFilled = false
				break
			}
		}

		if allFilled {
			return true, userID
		}
	}

	return false, 0
}

func (gm *GameManager) pickRandomLetter(usedLetters []string) string {
	available := make([]string, 0)

	for _, letter := range AvailableLetters {
		used := false
		for _, usedLetter := range usedLetters {
			if letter == usedLetter {
				used = true
				break
			}
		}
		if !used {
			available = append(available, letter)
		}
	}

	if len(available) == 0 {
		return AvailableLetters[rand.IntN(len(AvailableLetters))]
	}

	return available[rand.IntN(len(available))]
}

func GetAvailableCategories() []string {
	return models.DefaultPetitBacCategories
}
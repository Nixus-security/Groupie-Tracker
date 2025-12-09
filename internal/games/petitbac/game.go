// Package petitbac gère la logique du jeu Petit Bac Musical
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

// Lettres disponibles pour le Petit Bac (sans les lettres difficiles)
var AvailableLetters = []string{
	"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
	"N", "O", "P", "R", "S", "T", "V",
}

// GameState représente l'état d'une partie de Petit Bac
type GameState struct {
	RoomID        string                       `json:"room_id"`
	CurrentRound  int                          `json:"current_round"`
	TotalRounds   int                          `json:"total_rounds"`
	CurrentLetter string                       `json:"current_letter"`
	UsedLetters   []string                     `json:"used_letters"`
	Categories    []string                     `json:"categories"`
	Answers       map[int64]map[string]string  `json:"answers"`     // UserID -> Category -> Answer
	HasSubmitted  map[int64]bool               `json:"has_submitted"` // UserID -> a soumis
	Votes         map[int64]map[string][]int64 `json:"votes"`       // UserID -> Category -> VotersIDs
	RoundStoppedBy int64                       `json:"round_stopped_by"`
	TimeLeft      int                          `json:"time_left"`
	Phase         GamePhase                    `json:"phase"`
	Timer         *time.Timer                  `json:"-"`
	Mutex         sync.RWMutex                 `json:"-"`
}

// GamePhase phase de jeu
type GamePhase string

const (
	PhaseWaiting  GamePhase = "waiting"
	PhaseAnswering GamePhase = "answering"
	PhaseVoting   GamePhase = "voting"
	PhaseResults  GamePhase = "results"
)

// Constantes de temps
const (
	AnswerTime = 60  // Temps pour répondre en secondes
	VoteTime   = 30  // Temps pour voter en secondes
)

// GameManager gère toutes les parties de Petit Bac actives
type GameManager struct {
	games       map[string]*GameState
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

// StartGame démarre une nouvelle partie de Petit Bac
func (gm *GameManager) StartGame(roomID string, categories []string, rounds int) (*GameState, error) {
	if len(categories) == 0 {
		categories = models.DefaultPetitBacCategories
	}

	if rounds <= 0 || rounds > len(AvailableLetters) {
		rounds = models.NbrsManche
	}

	state := &GameState{
		RoomID:       roomID,
		CurrentRound: 0,
		TotalRounds:  rounds,
		Categories:   categories,
		UsedLetters:  []string{},
		Phase:        PhaseWaiting,
	}

	gm.mutex.Lock()
	gm.games[roomID] = state
	gm.mutex.Unlock()

	// Mettre à jour le statut de la salle
	gm.roomManager.UpdateRoomStatus(roomID, models.RoomStatusPlaying)
	gm.roomManager.ResetPlayerScores(roomID)

	log.Printf("[PetitBac] Partie démarrée dans la salle %s avec %d manches", roomID, rounds)
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
		return nil, nil
	}

	// Tirer une nouvelle lettre non utilisée
	letter := gm.pickRandomLetter(state.UsedLetters)
	state.UsedLetters = append(state.UsedLetters, letter)

	// Réinitialiser pour la nouvelle manche
	state.CurrentRound++
	state.CurrentLetter = letter
	state.TimeLeft = AnswerTime
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
		Duration:   state.TimeLeft,
	}, nil
}

// RoundInfo informations envoyées aux joueurs pour une manche
type RoundInfo struct {
	Round      int      `json:"round"`
	Total      int      `json:"total"`
	Letter     string   `json:"letter"`
	Categories []string `json:"categories"`
	Duration   int      `json:"duration"`
}

// SubmitAnswers soumet les réponses d'un joueur
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

	// Valider et nettoyer les réponses
	cleanAnswers := make(map[string]string)
	for _, cat := range state.Categories {
		answer := strings.TrimSpace(answers[cat])
		// Vérifier que la réponse commence par la bonne lettre
		if len(answer) > 0 && strings.ToUpper(string(answer[0])) == state.CurrentLetter {
			cleanAnswers[cat] = answer
		} else {
			cleanAnswers[cat] = "" // Réponse invalide
		}
	}

	state.Answers[userID] = cleanAnswers
	state.HasSubmitted[userID] = true

	log.Printf("[PetitBac] Réponses de %d soumises", userID)
	return nil
}

// StopRound arrête la manche (appelé par un joueur qui a fini)
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

// StartVoting démarre la phase de vote
func (gm *GameManager) StartVoting(roomID string) *VotingInfo {
	state := gm.GetGameState(roomID)
	if state == nil {
		return nil
	}

	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	state.Phase = PhaseVoting
	state.TimeLeft = VoteTime

	// Préparer les réponses à voter
	allAnswers := make(map[string]map[int64]string) // Category -> UserID -> Answer
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

// VotingInfo informations pour la phase de vote
type VotingInfo struct {
	Answers    map[string]map[int64]string `json:"answers"`
	Duration   int                         `json:"duration"`
	Categories []string                    `json:"categories"`
}

// SubmitVote soumet un vote contre une réponse
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

	// Ne peut pas voter pour soi-même
	if voterID == targetUserID {
		return nil
	}

	// Initialiser les votes si nécessaire
	if state.Votes[targetUserID] == nil {
		state.Votes[targetUserID] = make(map[string][]int64)
	}

	if reject {
		// Ajouter le vote de rejet
		state.Votes[targetUserID][category] = append(state.Votes[targetUserID][category], voterID)
	}

	return nil
}

// CalculateRoundScores calcule les scores de la manche
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

			// Compter les votes de rejet
			rejectVotes := 0
			if state.Votes[userID] != nil {
				rejectVotes = len(state.Votes[userID][category])
			}

			// Réponse rejetée si majorité de votes contre
			rejected := rejectVotes > totalPlayers/2

			// Calculer les points
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

	// Ajouter les scores à la salle
	for userID, pts := range scores {
		gm.roomManager.AddPlayerScore(roomID, userID, pts)
	}

	log.Printf("[PetitBac] Scores de la manche calculés")

	return &RoundScores{
		Scores:  scores,
		Details: details,
	}
}

// AnswerScore score détaillé d'une réponse
type AnswerScore struct {
	Answer   string `json:"answer"`
	Points   int    `json:"points"`
	Rejected bool   `json:"rejected"`
}

// RoundScores scores de la manche
type RoundScores struct {
	Scores  map[int64]int                    `json:"scores"`
	Details map[int64]map[string]AnswerScore `json:"details"`
}

// calculateAnswerPoints calcule les points pour une réponse
func (gm *GameManager) calculateAnswerPoints(category, answer string, state *GameState) int {
	// Vérifier si c'est une réponse unique
	answerLower := strings.ToLower(answer)
	count := 0
	
	for _, answers := range state.Answers {
		if strings.ToLower(answers[category]) == answerLower {
			count++
		}
	}

	// Points: 10 si unique, 5 si partagé
	if count == 1 {
		return 10
	}
	return 5
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

// AllPlayersSubmitted vérifie si tous les joueurs ont soumis leurs réponses
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

// ============================================================================
// FONCTIONS UTILITAIRES
// ============================================================================

// pickRandomLetter sélectionne une lettre aléatoire non utilisée
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
		// Toutes les lettres utilisées, on recommence
		return AvailableLetters[rand.IntN(len(AvailableLetters))]
	}

	return available[rand.IntN(len(available))]
}

// GetAvailableCategories retourne les catégories disponibles
func GetAvailableCategories() []string {
	return models.DefaultPetitBacCategories
}
// Package petitbac gère la logique du jeu Petit Bac Musical
package petitbac

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"groupie-tracker/internal/database"
	"groupie-tracker/internal/models"
	"groupie-tracker/internal/rooms"
	"groupie-tracker/internal/websocket"
)

// GameManager gère toutes les parties de Petit Bac en cours
type GameManager struct {
	games  map[string]*Game
	mutex  sync.RWMutex
	hub    *websocket.Hub
	rooms  *rooms.Manager
}

// Game représente une partie de Petit Bac
type Game struct {
	RoomCode      string
	CurrentRound  int
	TotalRounds   int
	CurrentLetter string
	UsedLetters   []string
	Categories    []string
	Players       map[int64]*PlayerState
	Scores        map[int64]int
	RoundScores   map[int64][]int
	Status        string
	RoundStart    time.Time
	StoppedBy     int64
	Mutex         sync.RWMutex
}

// PlayerState état d'un joueur dans la partie
type PlayerState struct {
	UserID    int64
	Pseudo    string
	Answers   map[string]string
	Submitted bool
	Votes     map[string]map[int64]bool
}

// Points
const (
	PointsUniqueValid = 2
	PointsSharedValid = 1
	PointsInvalid     = 0
)

// Lettres disponibles
var AvailableLetters = []string{
	"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
	"N", "O", "P", "Q", "R", "S", "T", "U", "V",
}

var (
	managerInstance *GameManager
	managerOnce     sync.Once
)

// GetManager retourne l'instance singleton du GameManager
func GetManager() *GameManager {
	managerOnce.Do(func() {
		managerInstance = &GameManager{
			games: make(map[string]*Game),
			hub:   websocket.GetHub(),
			rooms: rooms.GetManager(),
		}
	})
	return managerInstance
}

// StartGame démarre une nouvelle partie de Petit Bac
func (gm *GameManager) StartGame(roomCode string) error {
	room, err := gm.rooms.GetRoom(roomCode)
	if err != nil {
		return err
	}

	categories := room.Config.Categories
	if len(categories) == 0 {
		categories = models.DefaultPetitBacCategories
	}

	totalRounds := room.Config.NbRounds
	if totalRounds <= 0 {
		totalRounds = models.NbrsManche
	}

	game := &Game{
		RoomCode:     roomCode,
		CurrentRound: 0,
		TotalRounds:  totalRounds,
		UsedLetters:  make([]string, 0),
		Categories:   categories,
		Players:      make(map[int64]*PlayerState),
		Scores:       make(map[int64]int),
		RoundScores:  make(map[int64][]int),
		Status:       "playing",
	}

	room.Mutex.RLock()
	for userID, player := range room.Players {
		game.Players[userID] = &PlayerState{
			UserID:  userID,
			Pseudo:  player.Pseudo,
			Answers: make(map[string]string),
			Votes:   make(map[string]map[int64]bool),
		}
		game.Scores[userID] = 0
		game.RoundScores[userID] = make([]int, 0)
	}
	room.Mutex.RUnlock()

	gm.mutex.Lock()
	gm.games[roomCode] = game
	gm.mutex.Unlock()

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

	game.CurrentLetter = gm.pickRandomLetter(game.UsedLetters)
	game.UsedLetters = append(game.UsedLetters, game.CurrentLetter)
	game.Status = "playing"
	game.RoundStart = time.Now()
	game.StoppedBy = 0

	for _, player := range game.Players {
		player.Answers = make(map[string]string)
		player.Submitted = false
		player.Votes = make(map[string]map[int64]bool)
	}

	roundInfo := map[string]interface{}{
		"round":        game.CurrentRound,
		"total_rounds": game.TotalRounds,
		"letter":       game.CurrentLetter,
		"categories":   game.Categories,
	}
	roomCode := game.RoomCode

	game.Mutex.Unlock()

	gm.hub.Broadcast(roomCode, &models.WSMessage{
		Type:    models.WSTypePBNewRound,
		Payload: roundInfo,
	})

	log.Printf("🎼 Petit Bac %s: Manche %d - Lettre %s", roomCode, game.CurrentRound, game.CurrentLetter)
}

// pickRandomLetter choisit une lettre aléatoire non utilisée
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
		return AvailableLetters[rand.Intn(len(AvailableLetters))]
	}

	return available[rand.Intn(len(available))]
}

// SubmitAnswers soumet les réponses d'un joueur
func (gm *GameManager) SubmitAnswers(roomCode string, userID int64, answers map[string]string) {
	gm.mutex.RLock()
	game, exists := gm.games[roomCode]
	gm.mutex.RUnlock()

	if !exists {
		return
	}

	game.Mutex.Lock()

	if game.Status != "playing" {
		game.Mutex.Unlock()
		return
	}

	player, exists := game.Players[userID]
	if !exists || player.Submitted {
		game.Mutex.Unlock()
		return
	}

	for category, answer := range answers {
		answer = strings.TrimSpace(answer)
		if answer != "" && !strings.HasPrefix(strings.ToUpper(answer), game.CurrentLetter) {
			answers[category] = ""
		} else {
			answers[category] = answer
		}
	}

	player.Answers = answers
	player.Submitted = true

	pseudo := player.Pseudo
	roomCodeCopy := game.RoomCode

	allSubmitted := true
	for _, p := range game.Players {
		if !p.Submitted {
			allSubmitted = false
			break
		}
	}

	game.Mutex.Unlock()

	gm.hub.Broadcast(roomCodeCopy, &models.WSMessage{
		Type: models.WSTypePBAnswer,
		Payload: map[string]interface{}{
			"user_id":   userID,
			"pseudo":    pseudo,
			"submitted": true,
		},
	})

	log.Printf("📝 Petit Bac %s: %s a soumis ses réponses", roomCodeCopy, pseudo)

	if allSubmitted {
		gm.startVoting(game)
	}
}

// StopRound arrête la manche
func (gm *GameManager) StopRound(roomCode string, userID int64) {
	gm.mutex.RLock()
	game, exists := gm.games[roomCode]
	gm.mutex.RUnlock()

	if !exists {
		return
	}

	game.Mutex.Lock()

	if game.Status != "playing" || game.StoppedBy != 0 {
		game.Mutex.Unlock()
		return
	}

	player, exists := game.Players[userID]
	if !exists || !player.Submitted {
		game.Mutex.Unlock()
		return
	}

	game.StoppedBy = userID
	pseudo := player.Pseudo
	roomCodeCopy := game.RoomCode

	game.Mutex.Unlock()

	gm.hub.Broadcast(roomCodeCopy, &models.WSMessage{
		Type: models.WSTypePBStopRound,
		Payload: map[string]interface{}{
			"stopped_by": userID,
			"pseudo":     pseudo,
		},
	})

	log.Printf("🛑 Petit Bac %s: %s a stoppé la manche", roomCodeCopy, pseudo)

	time.AfterFunc(3*time.Second, func() {
		gm.startVoting(game)
	})
}

// startVoting démarre la phase de vote
func (gm *GameManager) startVoting(game *Game) {
	game.Mutex.Lock()

	if game.Status != "playing" {
		game.Mutex.Unlock()
		return
	}

	game.Status = "voting"

	answersToVote := make(map[string][]map[string]interface{})
	for _, category := range game.Categories {
		answersToVote[category] = make([]map[string]interface{}, 0)
		for _, player := range game.Players {
			answer := player.Answers[category]
			if answer != "" {
				answersToVote[category] = append(answersToVote[category], map[string]interface{}{
					"user_id": player.UserID,
					"pseudo":  player.Pseudo,
					"answer":  answer,
				})
			}
		}
	}

	roomCode := game.RoomCode
	game.Mutex.Unlock()

	gm.hub.Broadcast(roomCode, &models.WSMessage{
		Type: models.WSTypePBVote,
		Payload: map[string]interface{}{
			"phase":   "start",
			"answers": answersToVote,
		},
	})

	log.Printf("🗳️ Petit Bac %s: Phase de vote", roomCode)
}

// SubmitVote soumet un vote pour une réponse
func (gm *GameManager) SubmitVote(roomCode string, voterID int64, targetUserID int64, category string, isValid bool) {
	gm.mutex.RLock()
	game, exists := gm.games[roomCode]
	gm.mutex.RUnlock()

	if !exists {
		return
	}

	game.Mutex.Lock()

	if game.Status != "voting" {
		game.Mutex.Unlock()
		return
	}

	voter, exists := game.Players[voterID]
	if !exists || voterID == targetUserID {
		game.Mutex.Unlock()
		return
	}

	if voter.Votes[category] == nil {
		voter.Votes[category] = make(map[int64]bool)
	}
	voter.Votes[category][targetUserID] = isValid

	allVoted := gm.checkAllVotesComplete(game)

	roomCodeCopy := game.RoomCode
	game.Mutex.Unlock()

	gm.hub.Broadcast(roomCodeCopy, &models.WSMessage{
		Type: models.WSTypePBVote,
		Payload: map[string]interface{}{
			"phase":     "vote",
			"voter_id":  voterID,
			"target_id": targetUserID,
			"category":  category,
		},
	})

	if allVoted {
		gm.endRound(game)
	}
}

// checkAllVotesComplete vérifie si tous les votes sont terminés
func (gm *GameManager) checkAllVotesComplete(game *Game) bool {
	for _, category := range game.Categories {
		for _, targetPlayer := range game.Players {
			if targetPlayer.Answers[category] == "" {
				continue
			}

			voteCount := 0
			for _, voter := range game.Players {
				if voter.UserID == targetPlayer.UserID {
					continue
				}
				if voter.Votes[category] != nil {
					if _, voted := voter.Votes[category][targetPlayer.UserID]; voted {
						voteCount++
					}
				}
			}

			expectedVotes := len(game.Players) - 1
			if voteCount < expectedVotes {
				return false
			}
		}
	}
	return true
}

// endRound termine la manche et calcule les scores
func (gm *GameManager) endRound(game *Game) {
	game.Mutex.Lock()

	if game.Status != "voting" {
		game.Mutex.Unlock()
		return
	}

	game.Status = "results"

	type AnswerResult struct {
		UserID       int64  `json:"user_id"`
		Pseudo       string `json:"pseudo"`
		Answer       string `json:"answer"`
		VotesFor     int    `json:"votes_for"`
		VotesAgainst int    `json:"votes_against"`
		Points       int    `json:"points"`
		IsValid      bool   `json:"is_valid"`
	}

	results := make(map[string][]*AnswerResult)

	for _, category := range game.Categories {
		results[category] = make([]*AnswerResult, 0)

		answersMap := make(map[string][]int64)

		for _, player := range game.Players {
			answer := strings.ToLower(strings.TrimSpace(player.Answers[category]))
			if answer != "" {
				answersMap[answer] = append(answersMap[answer], player.UserID)
			}
		}

		for _, player := range game.Players {
			answer := player.Answers[category]
			if answer == "" {
				results[category] = append(results[category], &AnswerResult{
					UserID:  player.UserID,
					Pseudo:  player.Pseudo,
					Answer:  "",
					Points:  PointsInvalid,
					IsValid: false,
				})
				continue
			}

			votesFor := 0
			votesAgainst := 0
			for _, voter := range game.Players {
				if voter.UserID == player.UserID {
					continue
				}
				if voter.Votes[category] != nil {
					if valid, voted := voter.Votes[category][player.UserID]; voted {
						if valid {
							votesFor++
						} else {
							votesAgainst++
						}
					}
				}
			}

			totalVotes := votesFor + votesAgainst
			isValid := totalVotes > 0 && votesFor > votesAgainst

			points := PointsInvalid
			if isValid {
				normalizedAnswer := strings.ToLower(strings.TrimSpace(answer))
				if len(answersMap[normalizedAnswer]) == 1 {
					points = PointsUniqueValid
				} else {
					points = PointsSharedValid
				}
			}

			results[category] = append(results[category], &AnswerResult{
				UserID:       player.UserID,
				Pseudo:       player.Pseudo,
				Answer:       answer,
				Points:       points,
				IsValid:      isValid,
				VotesFor:     votesFor,
				VotesAgainst: votesAgainst,
			})

			game.Scores[player.UserID] += points
		}
	}

	for userID := range game.Players {
		roundScore := 0
		for _, categoryResults := range results {
			for _, result := range categoryResults {
				if result.UserID == userID {
					roundScore += result.Points
				}
			}
		}
		game.RoundScores[userID] = append(game.RoundScores[userID], roundScore)
	}

	roomCode := game.RoomCode
	scores := make(map[int64]int)
	for k, v := range game.Scores {
		scores[k] = v
	}

	game.Mutex.Unlock()

	gm.hub.Broadcast(roomCode, &models.WSMessage{
		Type: models.WSTypePBVoteResult,
		Payload: map[string]interface{}{
			"results": results,
			"scores":  scores,
		},
	})

	log.Printf("📊 Petit Bac %s: Résultats manche %d", roomCode, game.CurrentRound)

	time.AfterFunc(5*time.Second, func() {
		gm.startRound(game)
	})
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

	rankings := gm.buildRankings(scores)

	gm.hub.Broadcast(roomCode, &models.WSMessage{
		Type: models.WSTypePBGameEnd,
		Payload: map[string]interface{}{
			"rankings":     rankings,
			"scores":       scores,
			"round_scores": roundScores,
		},
	})

	gm.rooms.EndGame(roomCode)

	service := rooms.NewService()
	room, _ := gm.rooms.GetRoom(roomCode)
	service.SaveGameScores(room, roundScores)

	gm.mutex.Lock()
	delete(gm.games, roomCode)
	gm.mutex.Unlock()

	log.Printf("🏆 Petit Bac %s terminé", roomCode)
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

// ============================================================================
// HANDLER HTTP POUR CRUD CATÉGORIES
// ============================================================================

// Handler gère les requêtes HTTP pour le Petit Bac
type Handler struct{}

// NewHandler crée un nouveau handler
func NewHandler() *Handler {
	return &Handler{}
}

// CategoriesAPI gère le CRUD des catégories
func (h *Handler) CategoriesAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listCategories(w, r)
	case http.MethodPost:
		h.createCategory(w, r)
	default:
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
	}
}

// CategoryAPI gère le CRUD d'une catégorie spécifique
func (h *Handler) CategoryAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/petitbac/categories/")
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "ID invalide", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		h.deleteCategory(w, r, id)
	default:
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
	}
}

// listCategories liste toutes les catégories
func (h *Handler) listCategories(w http.ResponseWriter, _ *http.Request) {
	db := database.GetDB()

	rows, err := db.Query("SELECT id, name, created_at FROM petitbac_categories ORDER BY name")
	if err != nil {
		http.Error(w, "Erreur base de données", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	categories := make([]models.PetitBacCategory, 0)
	for rows.Next() {
		var cat models.PetitBacCategory
		if err := rows.Scan(&cat.ID, &cat.Name, &cat.CreatedAt); err != nil {
			continue
		}
		categories = append(categories, cat)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"categories": categories,
	})
}

// createCategory crée une nouvelle catégorie
func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON invalide", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, "Nom requis", http.StatusBadRequest)
		return
	}

	db := database.GetDB()

	result, err := db.Exec(
		"INSERT INTO petitbac_categories (name) VALUES (?)",
		strings.ToLower(name),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, "Catégorie déjà existante", http.StatusConflict)
			return
		}
		http.Error(w, "Erreur création", http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":   id,
		"name": name,
	})
}

// deleteCategory supprime une catégorie
func (h *Handler) deleteCategory(w http.ResponseWriter, _ *http.Request, id int64) {
	db := database.GetDB()

	result, err := db.Exec("DELETE FROM petitbac_categories WHERE id = ?", id)
	if err != nil {
		http.Error(w, "Erreur suppression", http.StatusInternalServerError)
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		http.Error(w, "Catégorie non trouvée", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
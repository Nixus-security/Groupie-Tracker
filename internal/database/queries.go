package database

import (
	"database/sql"
	"time"
)

// Users

func CreateUser(pseudo, email, passwordHash string) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO users (pseudo, email, password_hash) VALUES (?, ?, ?)`,
		pseudo, email, passwordHash,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetUserByID(id int64) (*User, error) {
	u := &User{}
	err := db.QueryRow(
		`SELECT id, pseudo, email, password_hash, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Pseudo, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func GetUserByPseudo(pseudo string) (*User, error) {
	u := &User{}
	err := db.QueryRow(
		`SELECT id, pseudo, email, password_hash, created_at FROM users WHERE pseudo = ?`, pseudo,
	).Scan(&u.ID, &u.Pseudo, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func GetUserByEmail(email string) (*User, error) {
	u := &User{}
	err := db.QueryRow(
		`SELECT id, pseudo, email, password_hash, created_at FROM users WHERE email = ?`, email,
	).Scan(&u.ID, &u.Pseudo, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func GetUserByIdentifier(identifier string) (*User, error) {
	u := &User{}
	err := db.QueryRow(
		`SELECT id, pseudo, email, password_hash, created_at FROM users WHERE pseudo = ? OR email = ?`,
		identifier, identifier,
	).Scan(&u.ID, &u.Pseudo, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func PseudoExists(pseudo string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE pseudo = ?`, pseudo).Scan(&count)
	return count > 0, err
}

func EmailExists(email string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ?`, email).Scan(&count)
	return count > 0, err
}

// Sessions

func CreateSession(userID int64, token string, expiresAt time.Time) error {
	_, err := db.Exec(
		`INSERT INTO sessions (user_id, token, expires_at) VALUES (?, ?, ?)`,
		userID, token, expiresAt,
	)
	return err
}

func GetSessionByToken(token string) (*Session, error) {
	s := &Session{}
	err := db.QueryRow(
		`SELECT id, user_id, token, expires_at, created_at FROM sessions WHERE token = ? AND expires_at > datetime('now')`,
		token,
	).Scan(&s.ID, &s.UserID, &s.Token, &s.ExpiresAt, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func DeleteSession(token string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func CleanExpiredSessions() error {
	_, err := db.Exec(`DELETE FROM sessions WHERE expires_at <= datetime('now')`)
	return err
}

// Rooms

func CreateRoom(code string, creatorID int64, gameType, config string) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO rooms (code, creator_id, game_type, config) VALUES (?, ?, ?, ?)`,
		code, creatorID, gameType, config,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetRoomByCode(code string) (*Room, error) {
	r := &Room{}
	err := db.QueryRow(
		`SELECT id, code, creator_id, game_type, status, config, created_at FROM rooms WHERE code = ?`,
		code,
	).Scan(&r.ID, &r.Code, &r.CreatorID, &r.GameType, &r.Status, &r.Config, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

func GetRoomByID(id int64) (*Room, error) {
	r := &Room{}
	err := db.QueryRow(
		`SELECT id, code, creator_id, game_type, status, config, created_at FROM rooms WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.Code, &r.CreatorID, &r.GameType, &r.Status, &r.Config, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

func UpdateRoomStatus(roomID int64, status string) error {
	_, err := db.Exec(`UPDATE rooms SET status = ? WHERE id = ?`, status, roomID)
	return err
}

func GetActiveRooms() ([]*Room, error) {
	rows, err := db.Query(
		`SELECT id, code, creator_id, game_type, status, config, created_at 
		 FROM rooms WHERE status IN ('waiting', 'playing') ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []*Room
	for rows.Next() {
		r := &Room{}
		if err := rows.Scan(&r.ID, &r.Code, &r.CreatorID, &r.GameType, &r.Status, &r.Config, &r.CreatedAt); err != nil {
			return nil, err
		}
		rooms = append(rooms, r)
	}
	return rooms, nil
}

func RoomCodeExists(code string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM rooms WHERE code = ?`, code).Scan(&count)
	return count > 0, err
}

// Room Players

func AddPlayerToRoom(roomID, userID int64) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO room_players (room_id, user_id) VALUES (?, ?)`,
		roomID, userID,
	)
	return err
}

func RemovePlayerFromRoom(roomID, userID int64) error {
	_, err := db.Exec(`DELETE FROM room_players WHERE room_id = ? AND user_id = ?`, roomID, userID)
	return err
}

func GetRoomPlayers(roomID int64) ([]*Player, error) {
	rows, err := db.Query(
		`SELECT u.id, u.pseudo FROM users u
		 INNER JOIN room_players rp ON u.id = rp.user_id
		 WHERE rp.room_id = ? ORDER BY rp.joined_at`,
		roomID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []*Player
	for rows.Next() {
		p := &Player{}
		if err := rows.Scan(&p.ID, &p.Pseudo); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, nil
}

func GetPlayerCount(roomID int64) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM room_players WHERE room_id = ?`, roomID).Scan(&count)
	return count, err
}

func IsPlayerInRoom(roomID, userID int64) (bool, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM room_players WHERE room_id = ? AND user_id = ?`,
		roomID, userID,
	).Scan(&count)
	return count > 0, err
}

// Games

func CreateGame(roomID int64, gameType string, totalRounds int, config string) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO games (room_id, game_type, total_rounds, config) VALUES (?, ?, ?, ?)`,
		roomID, gameType, totalRounds, config,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetGameByID(id int64) (*Game, error) {
	g := &Game{}
	err := db.QueryRow(
		`SELECT id, room_id, game_type, current_round, total_rounds, status, used_letters, config, created_at 
		 FROM games WHERE id = ?`,
		id,
	).Scan(&g.ID, &g.RoomID, &g.GameType, &g.CurrentRound, &g.TotalRounds, &g.Status, &g.UsedLetters, &g.Config, &g.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return g, err
}

func GetActiveGameByRoomID(roomID int64) (*Game, error) {
	g := &Game{}
	err := db.QueryRow(
		`SELECT id, room_id, game_type, current_round, total_rounds, status, used_letters, config, created_at 
		 FROM games WHERE room_id = ? AND status IN ('pending', 'active', 'voting') 
		 ORDER BY created_at DESC LIMIT 1`,
		roomID,
	).Scan(&g.ID, &g.RoomID, &g.GameType, &g.CurrentRound, &g.TotalRounds, &g.Status, &g.UsedLetters, &g.Config, &g.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return g, err
}

func UpdateGameStatus(gameID int64, status string) error {
	_, err := db.Exec(`UPDATE games SET status = ? WHERE id = ?`, status, gameID)
	return err
}

func UpdateGameRound(gameID int64, round int) error {
	_, err := db.Exec(`UPDATE games SET current_round = ? WHERE id = ?`, round, gameID)
	return err
}

func UpdateGameUsedLetters(gameID int64, letters string) error {
	_, err := db.Exec(`UPDATE games SET used_letters = ? WHERE id = ?`, letters, gameID)
	return err
}

// Scores

func AddScore(gameID, userID int64, points, roundNumber int) error {
	_, err := db.Exec(
		`INSERT INTO scores (game_id, user_id, points, round_number) VALUES (?, ?, ?, ?)
		 ON CONFLICT(game_id, user_id, round_number) DO UPDATE SET points = points + excluded.points`,
		gameID, userID, points, roundNumber,
	)
	return err
}

func GetGameScores(gameID int64) ([]*Score, error) {
	rows, err := db.Query(
		`SELECT s.id, s.game_id, s.user_id, s.points, s.round_number, u.pseudo
		 FROM scores s INNER JOIN users u ON s.user_id = u.id
		 WHERE s.game_id = ? ORDER BY s.round_number`,
		gameID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scores []*Score
	for rows.Next() {
		s := &Score{}
		if err := rows.Scan(&s.ID, &s.GameID, &s.UserID, &s.Points, &s.RoundNumber, &s.Pseudo); err != nil {
			return nil, err
		}
		scores = append(scores, s)
	}
	return scores, nil
}

func GetScoreboard(gameID int64) ([]*ScoreboardEntry, error) {
	rows, err := db.Query(
		`SELECT u.id, u.pseudo, COALESCE(SUM(s.points), 0) as total
		 FROM room_players rp
		 INNER JOIN users u ON rp.user_id = u.id
		 INNER JOIN games g ON rp.room_id = g.room_id
		 LEFT JOIN scores s ON s.game_id = g.id AND s.user_id = u.id
		 WHERE g.id = ?
		 GROUP BY u.id, u.pseudo
		 ORDER BY total DESC`,
		gameID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*ScoreboardEntry
	rank := 1
	for rows.Next() {
		e := &ScoreboardEntry{Rank: rank}
		if err := rows.Scan(&e.UserID, &e.Pseudo, &e.TotalScore); err != nil {
			return nil, err
		}
		entries = append(entries, e)
		rank++
	}
	return entries, nil
}

// Categories

func GetAllCategories() ([]*Category, error) {
	rows, err := db.Query(`SELECT id, name, is_default FROM categories ORDER BY is_default DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []*Category
	for rows.Next() {
		c := &Category{}
		if err := rows.Scan(&c.ID, &c.Name, &c.IsDefault); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, nil
}

func GetCategoryByID(id int64) (*Category, error) {
	c := &Category{}
	err := db.QueryRow(`SELECT id, name, is_default FROM categories WHERE id = ?`, id).Scan(&c.ID, &c.Name, &c.IsDefault)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func CreateCategory(name string) (int64, error) {
	result, err := db.Exec(`INSERT INTO categories (name, is_default) VALUES (?, 0)`, name)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func UpdateCategory(id int64, name string) error {
	_, err := db.Exec(`UPDATE categories SET name = ? WHERE id = ? AND is_default = 0`, name, id)
	return err
}

func DeleteCategory(id int64) error {
	_, err := db.Exec(`DELETE FROM categories WHERE id = ? AND is_default = 0`, id)
	return err
}

// Petit Bac Answers

func SavePetitBacAnswer(gameID, userID int64, roundNumber int, categoryID int64, letter, answer string) error {
	_, err := db.Exec(
		`INSERT INTO petitbac_answers (game_id, user_id, round_number, category_id, letter, answer)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(game_id, user_id, round_number, category_id) DO UPDATE SET answer = excluded.answer`,
		gameID, userID, roundNumber, categoryID, letter, answer,
	)
	return err
}

func GetRoundAnswers(gameID int64, roundNumber int) ([]*PetitBacAnswer, error) {
	rows, err := db.Query(
		`SELECT id, game_id, user_id, round_number, category_id, letter, answer, 
		        votes_valid, votes_invalid, is_validated, points_awarded
		 FROM petitbac_answers WHERE game_id = ? AND round_number = ?`,
		gameID, roundNumber,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var answers []*PetitBacAnswer
	for rows.Next() {
		a := &PetitBacAnswer{}
		if err := rows.Scan(&a.ID, &a.GameID, &a.UserID, &a.RoundNumber, &a.CategoryID,
			&a.Letter, &a.Answer, &a.VotesValid, &a.VotesInvalid, &a.IsValidated, &a.PointsAwarded); err != nil {
			return nil, err
		}
		answers = append(answers, a)
	}
	return answers, nil
}

func UpdateAnswerVotes(answerID int64, votesValid, votesInvalid int, isValidated bool) error {
	validated := 0
	if isValidated {
		validated = 1
	}
	_, err := db.Exec(
		`UPDATE petitbac_answers SET votes_valid = ?, votes_invalid = ?, is_validated = ? WHERE id = ?`,
		votesValid, votesInvalid, validated, answerID,
	)
	return err
}

func UpdateAnswerPoints(answerID int64, points int) error {
	_, err := db.Exec(`UPDATE petitbac_answers SET points_awarded = ? WHERE id = ?`, points, answerID)
	return err
}

// Types pour les requêtes

type User struct {
	ID           int64
	Pseudo       string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type Session struct {
	ID        int64
	UserID    int64
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type Room struct {
	ID        int64
	Code      string
	CreatorID int64
	GameType  string
	Status    string
	Config    string
	CreatedAt time.Time
}

type Player struct {
	ID     int64
	Pseudo string
}

type Game struct {
	ID           int64
	RoomID       int64
	GameType     string
	CurrentRound int
	TotalRounds  int
	Status       string
	UsedLetters  string
	Config       string
	CreatedAt    time.Time
}

type Score struct {
	ID          int64
	GameID      int64
	UserID      int64
	Points      int
	RoundNumber int
	Pseudo      string
}

type ScoreboardEntry struct {
	Rank       int
	UserID     int64
	Pseudo     string
	TotalScore int
}

type Category struct {
	ID        int64
	Name      string
	IsDefault bool
}

type PetitBacAnswer struct {
	ID            int64
	GameID        int64
	UserID        int64
	RoundNumber   int
	CategoryID    int64
	Letter        string
	Answer        string
	VotesValid    int
	VotesInvalid  int
	IsValidated   bool
	PointsAwarded int
}

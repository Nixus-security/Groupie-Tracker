package database

import "log"

func RunMigrations() error {
	migrations := []string{
		createUsersTable,
		createSessionsTable,
		createRoomsTable,
		createRoomPlayersTable,
		createGamesTable,
		createScoresTable,
		createCategoriesTable,
		createPetitBacAnswersTable,
		insertDefaultCategories,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			log.Printf("Erreur migration: %v", err)
			return err
		}
	}

	return nil
}

const createUsersTable = `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	pseudo TEXT NOT NULL UNIQUE,
	email TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	CHECK (pseudo GLOB '[A-Z]*')
)`

const createSessionsTable = `
CREATE TABLE IF NOT EXISTS sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	token TEXT NOT NULL UNIQUE,
	expires_at DATETIME NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
)`

const createRoomsTable = `
CREATE TABLE IF NOT EXISTS rooms (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	code TEXT NOT NULL UNIQUE,
	creator_id INTEGER NOT NULL,
	game_type TEXT NOT NULL CHECK (game_type IN ('blindtest', 'petitbac')),
	status TEXT NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting', 'playing', 'finished')),
	config TEXT DEFAULT '{}',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (creator_id) REFERENCES users(id) ON DELETE CASCADE
)`

const createRoomPlayersTable = `
CREATE TABLE IF NOT EXISTS room_players (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	room_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (room_id, user_id),
	FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
)`

const createGamesTable = `
CREATE TABLE IF NOT EXISTS games (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	room_id INTEGER NOT NULL,
	game_type TEXT NOT NULL CHECK (game_type IN ('blindtest', 'petitbac')),
	current_round INTEGER DEFAULT 1,
	total_rounds INTEGER NOT NULL,
	status TEXT DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'voting', 'finished')),
	used_letters TEXT DEFAULT '',
	config TEXT DEFAULT '{}',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE
)`

const createScoresTable = `
CREATE TABLE IF NOT EXISTS scores (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	game_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	points INTEGER DEFAULT 0,
	round_number INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (game_id, user_id, round_number),
	FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
)`

const createCategoriesTable = `
CREATE TABLE IF NOT EXISTS categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	is_default INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`

const createPetitBacAnswersTable = `
CREATE TABLE IF NOT EXISTS petitbac_answers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	game_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	round_number INTEGER NOT NULL,
	category_id INTEGER NOT NULL,
	letter TEXT NOT NULL,
	answer TEXT DEFAULT '',
	votes_valid INTEGER DEFAULT 0,
	votes_invalid INTEGER DEFAULT 0,
	is_validated INTEGER DEFAULT 0,
	points_awarded INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (game_id, user_id, round_number, category_id),
	FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE
)`

const insertDefaultCategories = `
INSERT OR IGNORE INTO categories (id, name, is_default) VALUES
	(1, 'Artiste', 1),
	(2, 'Album', 1),
	(3, 'Groupe', 1),
	(4, 'Instrument', 1),
	(5, 'Featuring', 1)
`

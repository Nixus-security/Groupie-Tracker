// Package database gère la connexion et les opérations SQLite
package database

import (
	"database/sql"
	"log"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

var (
	db   *sql.DB
	once sync.Once
)

// Init initialise la connexion à la base de données
func Init(dbPath string) error {
	var err error
	once.Do(func() {
		db, err = sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
		if err != nil {
			return
		}

		// Configuration du pool de connexions
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)

		// Vérifier la connexion
		if err = db.Ping(); err != nil {
			return
		}

		log.Println("✅ Base de données SQLite connectée:", dbPath)

		// Exécuter les migrations
		if err = RunMigrations(); err != nil {
			log.Printf("❌ Erreur migrations: %v", err)
			return
		}
	})
	return err
}

// GetDB retourne l'instance de la base de données
func GetDB() *sql.DB {
	return db
}

// Close ferme la connexion à la base de données
func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

// RunMigrations exécute toutes les migrations
func RunMigrationsS() error {
	migrations := []struct {
		name string
		sql  string
	}{
		{
			name: "create_users_table",
			sql: `
				CREATE TABLE IF NOT EXISTS users (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					pseudo TEXT NOT NULL UNIQUE,
					email TEXT NOT NULL UNIQUE,
					password_hash TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);
				CREATE INDEX IF NOT EXISTS idx_users_pseudo ON users(pseudo);
				CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
			`,
		},
		{
			name: "create_sessions_table",
			sql: `
				CREATE TABLE IF NOT EXISTS sessions (
					id TEXT PRIMARY KEY,
					user_id INTEGER NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					expires_at DATETIME NOT NULL,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				);
				CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
				CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
			`,
		},
		{
			name: "create_rooms_table",
			sql: `
				CREATE TABLE IF NOT EXISTS rooms (
					id TEXT PRIMARY KEY,
					code TEXT NOT NULL UNIQUE,
					name TEXT NOT NULL,
					host_id INTEGER NOT NULL,
					game_type TEXT NOT NULL,
					status TEXT DEFAULT 'waiting',
					config TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (host_id) REFERENCES users(id) ON DELETE CASCADE
				);
				CREATE INDEX IF NOT EXISTS idx_rooms_code ON rooms(code);
				CREATE INDEX IF NOT EXISTS idx_rooms_status ON rooms(status);
			`,
		},
		{
			name: "create_room_players_table",
			sql: `
				CREATE TABLE IF NOT EXISTS room_players (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					room_id TEXT NOT NULL,
					user_id INTEGER NOT NULL,
					score INTEGER DEFAULT 0,
					is_host BOOLEAN DEFAULT 0,
					joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
					UNIQUE(room_id, user_id)
				);
			`,
		},
		{
			name: "create_game_scores_table",
			sql: `
				CREATE TABLE IF NOT EXISTS game_scores (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					room_id TEXT NOT NULL,
					user_id INTEGER NOT NULL,
					game_type TEXT NOT NULL,
					score INTEGER DEFAULT 0,
					round_scores TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				);
				CREATE INDEX IF NOT EXISTS idx_game_scores_room ON game_scores(room_id);
				CREATE INDEX IF NOT EXISTS idx_game_scores_user ON game_scores(user_id);
			`,
		},
		{
			name: "create_petitbac_categories_table",
			sql: `
				CREATE TABLE IF NOT EXISTS petitbac_categories (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL UNIQUE,
					is_default BOOLEAN DEFAULT 0,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);
				INSERT OR IGNORE INTO petitbac_categories (name, is_default) VALUES 
					('artiste', 1),
					('album', 1),
					('groupe', 1),
					('instrument', 1),
					('featuring', 1);
			`,
		},
	}

	for _, m := range migrations {
		log.Printf("📦 Exécution migration: %s", m.name)
		_, err := db.Exec(m.sql)
		if err != nil {
			log.Printf("❌ Erreur migration %s: %v", m.name, err)
			return err
		}
	}

	log.Println("✅ Toutes les migrations exécutées avec succès")
	return nil
}
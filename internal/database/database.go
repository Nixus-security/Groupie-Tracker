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

		log.Println("[DB] Base de données SQLite connectée:", dbPath)

		// Exécuter les migrations
		if err = RunMigrations(); err != nil {
			log.Printf("[DB] Erreur migrations: %v", err)
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
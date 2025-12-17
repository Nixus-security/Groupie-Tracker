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

func Init(dbPath string) error {
	var err error
	once.Do(func() {
		db, err = sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
		if err != nil {
			return
		}

		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)

		if err = db.Ping(); err != nil {
			return
		}

		log.Println("[DB] Base de données SQLite connectée:", dbPath)

		if err = RunMigrations(); err != nil {
			log.Printf("[DB] Erreur migrations: %v", err)
			return
		}
	})
	return err
}

func GetDB() *sql.DB {
	return db
}

func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}
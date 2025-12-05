package config

import (
	"os"
)

type Config struct {
	Port            string
	DatabasePath    string
	SessionSecret   string
	SessionName     string
	SpotifyClientID string
	SpotifySecret   string
	SpotifyRedirect string
}

func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8080"),
		DatabasePath:    getEnv("DB_PATH", "./data/music_platform.db"),
		SessionSecret:   getEnv("SESSION_SECRET", "super-secret-key-change-in-prod"),
		SessionName:     getEnv("SESSION_NAME", "music_session"),
		SpotifyClientID: getEnv("SPOTIFY_CLIENT_ID", ""),
		SpotifySecret:   getEnv("SPOTIFY_CLIENT_SECRET", ""),
		SpotifyRedirect: getEnv("SPOTIFY_REDIRECT_URI", "http://localhost:8080/callback"),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

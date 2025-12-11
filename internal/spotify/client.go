// Package spotify gère l'intégration avec l'API Deezer (renommé pour compatibilité)
package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"groupie-tracker/internal/models"
)

var (
	ErrNoToken  = errors.New("pas de token valide")
	ErrNoTracks = errors.New("aucune piste trouvée")
)

// Config configuration du client (gardé pour compatibilité)
type Config struct {
	ClientID     string
	ClientSecret string
}

// Client gère les appels à l'API Deezer
type Client struct {
	httpClient *http.Client
	mutex      *sync.RWMutex
}

// instance singleton
var (
	clientInstance *Client
	clientOnce     sync.Once
)

// NewClient crée ou retourne le client singleton
func NewClient(config Config) *Client {
	clientOnce.Do(func() {
		clientInstance = &Client{
			httpClient: &http.Client{
				Timeout: 15 * time.Second,
			},
		}
	})
	return clientInstance
}

// GetClient retourne l'instance du client
func GetClient() *Client {
	return clientInstance
}

// Authenticate - Deezer n'a pas besoin d'auth pour les données publiques
func (c *Client) Authenticate() error {
	log.Println("[Deezer] Pas d'authentification requise pour l'API publique")
	return nil
}

// DeezerTrack représente une piste Deezer
type DeezerTrack struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Preview string `json:"preview"` // URL de preview 30s
	Artist  struct {
		Name string `json:"name"`
	} `json:"artist"`
	Album struct {
		Title string `json:"title"`
		Cover string `json:"cover_big"`
	} `json:"album"`
}

// GetChartTracks récupère les pistes du top chart
func (c *Client) GetChartTracks(limit int) ([]*models.SpotifyTrack, error) {
	apiURL := fmt.Sprintf("https://api.deezer.com/chart/0/tracks?limit=%d", limit)

	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []DeezerTrack `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var tracks []*models.SpotifyTrack
	for _, item := range result.Data {
		// Deezer a toujours des previews !
		if item.Preview == "" {
			continue
		}

		tracks = append(tracks, &models.SpotifyTrack{
			ID:         fmt.Sprintf("%d", item.ID),
			Name:       item.Title,
			Artist:     item.Artist.Name,
			Album:      item.Album.Title,
			PreviewURL: item.Preview,
			ImageURL:   item.Album.Cover,
		})
	}

	log.Printf("[Deezer] Chart: %d pistes avec preview", len(tracks))
	return tracks, nil
}

// SearchTracks recherche des pistes
func (c *Client) SearchTracks(query string, limit int) ([]*models.SpotifyTrack, error) {
	apiURL := fmt.Sprintf("https://api.deezer.com/search?q=%s&limit=%d", url.QueryEscape(query), limit)

	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []DeezerTrack `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var tracks []*models.SpotifyTrack
	for _, item := range result.Data {
		if item.Preview == "" {
			continue
		}

		tracks = append(tracks, &models.SpotifyTrack{
			ID:         fmt.Sprintf("%d", item.ID),
			Name:       item.Title,
			Artist:     item.Artist.Name,
			Album:      item.Album.Title,
			PreviewURL: item.Preview,
			ImageURL:   item.Album.Cover,
		})
	}

	log.Printf("[Deezer] Recherche '%s': %d pistes avec preview", query, len(tracks))
	return tracks, nil
}

// GetPlaylistTracks récupère les pistes d'une playlist Deezer
func (c *Client) GetPlaylistTracks(playlistID string, limit int) ([]*models.SpotifyTrack, error) {
	apiURL := fmt.Sprintf("https://api.deezer.com/playlist/%s/tracks?limit=%d", playlistID, limit)

	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []DeezerTrack `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var tracks []*models.SpotifyTrack
	for _, item := range result.Data {
		if item.Preview == "" {
			continue
		}

		tracks = append(tracks, &models.SpotifyTrack{
			ID:         fmt.Sprintf("%d", item.ID),
			Name:       item.Title,
			Artist:     item.Artist.Name,
			Album:      item.Album.Title,
			PreviewURL: item.Preview,
			ImageURL:   item.Album.Cover,
		})
	}

	log.Printf("[Deezer] Playlist %s: %d pistes avec preview", playlistID, len(tracks))
	return tracks, nil
}

// GetRandomTracksForBlindTest récupère des pistes aléatoires pour un Blind Test
func (c *Client) GetRandomTracksForBlindTest(genre string, count int) ([]*models.SpotifyTrack, error) {
	var allTracks []*models.SpotifyTrack

	// 1. Récupérer le top chart mondial
	chartTracks, err := c.GetChartTracks(50)
	if err != nil {
		log.Printf("[Deezer] Erreur chart: %v", err)
	} else {
		allTracks = append(allTracks, chartTracks...)
	}

	// 2. Si pas assez, faire une recherche
	if len(allTracks) < count {
		searchQueries := []string{"hit 2024", "pop", "top"}
		for _, query := range searchQueries {
			searchTracks, err := c.SearchTracks(query, 30)
			if err != nil {
				continue
			}
			allTracks = append(allTracks, searchTracks...)
			if len(allTracks) >= count*2 {
				break
			}
		}
	}

	if len(allTracks) == 0 {
		return nil, ErrNoTracks
	}

	// Supprimer les doublons
	seen := make(map[string]bool)
	uniqueTracks := make([]*models.SpotifyTrack, 0)
	for _, track := range allTracks {
		key := strings.ToLower(track.Name + track.Artist)
		if !seen[key] {
			seen[key] = true
			uniqueTracks = append(uniqueTracks, track)
		}
	}
	allTracks = uniqueTracks

	// Mélanger les pistes
	rand.Shuffle(len(allTracks), func(i, j int) {
		allTracks[i], allTracks[j] = allTracks[j], allTracks[i]
	})

	// Limiter au nombre demandé
	if count > len(allTracks) {
		count = len(allTracks)
	}

	log.Printf("[Deezer] Retourne %d pistes pour le blind test", count)
	return allTracks[:count], nil
}

// GetAvailableGenres retourne les genres disponibles
func GetAvailableGenres() []string {
	return []string{"Top Global"}
}
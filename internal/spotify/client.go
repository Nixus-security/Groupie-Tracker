// Package spotify gère l'intégration avec l'API Spotify
package spotify

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	ErrNoToken  = errors.New("pas de token Spotify valide")
	ErrNoTracks = errors.New("aucune piste trouvée")
)

// Config configuration du client Spotify
type Config struct {
	ClientID     string
	ClientSecret string
}

// Client gère les appels à l'API Spotify
type Client struct {
	config      Config
	httpClient  *http.Client
	accessToken string
	tokenExpiry time.Time
	mutex       sync.RWMutex
}

// PlaylistsByGenre - Playlists IDs par genre (playlists publiques Spotify)
var PlaylistsByGenre = map[string][]string{
	"Pop": {
		"37i9dQZF1DXcBWIGoYBM5M", // Today's Top Hits
		"37i9dQZF1DX0kbJZpiYdZl", // Hot Hits France
	},
	"Rock": {
		"37i9dQZF1DWXRqgorJj26U", // Rock Classics
		"37i9dQZF1DX1lVhptIYRda", // Hot Hits Rock
	},
	"Rap": {
		"37i9dQZF1DX0XUsuxWHRQd", // RapCaviar
		"37i9dQZF1DWU4xkXueiKGW", // Rap France
	},
}

// instance singleton
var (
	clientInstance *Client
	clientOnce     sync.Once
)

// NewClient crée ou retourne le client Spotify singleton
func NewClient(config Config) *Client {
	clientOnce.Do(func() {
		clientInstance = &Client{
			config: config,
			httpClient: &http.Client{
				Timeout: 10 * time.Second,
			},
		}
	})
	return clientInstance
}

// GetClient retourne l'instance du client Spotify
func GetClient() *Client {
	return clientInstance
}

// Authenticate s'authentifie auprès de l'API Spotify
func (c *Client) Authenticate() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Vérifier si le token est encore valide
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	// Préparer la requête
	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	// Headers d'authentification Basic
	auth := base64.StdEncoding.EncodeToString([]byte(c.config.ClientID + ":" + c.config.ClientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("erreur Spotify auth: %s - %s", resp.Status, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	c.accessToken = result.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)

	log.Println("[Spotify] Token obtenu, expire dans", result.ExpiresIn, "secondes")
	return nil
}

// getToken retourne le token d'accès, en le renouvelant si nécessaire
func (c *Client) getToken() (string, error) {
	c.mutex.RLock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		token := c.accessToken
		c.mutex.RUnlock()
		return token, nil
	}
	c.mutex.RUnlock()

	// Token expiré, le renouveler
	if err := c.Authenticate(); err != nil {
		return "", err
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.accessToken, nil
}

// GetPlaylistTracks récupère les pistes d'une playlist
func (c *Client) GetPlaylistTracks(playlistID string, limit int) ([]*models.SpotifyTrack, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://api.spotify.com/v1/playlists/%s/tracks?limit=%d&fields=items(track(id,name,artists,album,preview_url))", playlistID, limit)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("erreur Spotify: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Items []struct {
			Track struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name   string `json:"name"`
					Images []struct {
						URL string `json:"url"`
					} `json:"images"`
				} `json:"album"`
				PreviewURL string `json:"preview_url"`
			} `json:"track"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var tracks []*models.SpotifyTrack
	for _, item := range result.Items {
		// Ignorer les pistes sans URL de prévisualisation
		if item.Track.PreviewURL == "" {
			continue
		}

		artistNames := make([]string, len(item.Track.Artists))
		for i, a := range item.Track.Artists {
			artistNames[i] = a.Name
		}

		imageURL := ""
		if len(item.Track.Album.Images) > 0 {
			imageURL = item.Track.Album.Images[0].URL
		}

		tracks = append(tracks, &models.SpotifyTrack{
			ID:         item.Track.ID,
			Name:       item.Track.Name,
			Artist:     strings.Join(artistNames, ", "),
			Album:      item.Track.Album.Name,
			PreviewURL: item.Track.PreviewURL,
			ImageURL:   imageURL,
		})
	}

	if len(tracks) == 0 {
		return nil, ErrNoTracks
	}

	return tracks, nil
}

// GetRandomTracksForBlindTest récupère des pistes aléatoires pour un Blind Test
func (c *Client) GetRandomTracksForBlindTest(genre string, count int) ([]*models.SpotifyTrack, error) {
	playlists, ok := PlaylistsByGenre[genre]
	if !ok {
		// Genre par défaut
		playlists = PlaylistsByGenre["Pop"]
	}

	// Récupérer des pistes de plusieurs playlists
	var allTracks []*models.SpotifyTrack
	for _, playlistID := range playlists {
		tracks, err := c.GetPlaylistTracks(playlistID, 50)
		if err != nil {
			log.Printf("[Spotify] Erreur playlist %s: %v", playlistID, err)
			continue
		}
		allTracks = append(allTracks, tracks...)
	}

	if len(allTracks) == 0 {
		return nil, ErrNoTracks
	}

	// Mélanger les pistes (Go 1.21+ : pas besoin de seed, utilise math/rand/v2)
	rand.Shuffle(len(allTracks), func(i, j int) {
		allTracks[i], allTracks[j] = allTracks[j], allTracks[i]
	})

	// Limiter au nombre demandé
	if count > len(allTracks) {
		count = len(allTracks)
	}

	return allTracks[:count], nil
}

// GetAvailableGenres retourne les genres disponibles
func GetAvailableGenres() []string {
	genres := make([]string, 0, len(PlaylistsByGenre))
	for genre := range PlaylistsByGenre {
		genres = append(genres, genre)
	}
	return genres
}
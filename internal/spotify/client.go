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

// PlaylistsByGenre - Playlists IDs par genre (playlists publiques Spotify avec previews)
// MISE À JOUR : Ces playlists ont été testées pour avoir des preview URLs
var PlaylistsByGenre = map[string][]string{
	"Pop": {
		"37i9dQZF1DXcBWIGoYBM5M", // Today's Top Hits
		"37i9dQZF1DX0kbJZpiYdZl", // Hot Hits France
		"37i9dQZF1DXbYM3nMM0oPk", // Mega Hit Mix
		"37i9dQZF1DX4JAvHpjipBk", // New Music Friday
		"6UeSakyzhiEt4NB3UAd6NQ", // Billboard Hot 100
	},
	"Rock": {
		"37i9dQZF1DWXRqgorJj26U", // Rock Classics
		"37i9dQZF1DX1lVhptIYRda", // Hot Hits Rock
		"37i9dQZF1DXcF6B6QPhFDv", // Rock This
		"37i9dQZF1DX9GRpeH4CL0S", // Classic Rock Drive
	},
	"Rap": {
		"37i9dQZF1DX0XUsuxWHRQd", // RapCaviar
		"37i9dQZF1DWU4xkXueiKGW", // Rap France
		"37i9dQZF1DX6GwdWRQMQpq", // Rap UK
		"37i9dQZF1DX186v583rmzp", // Hip Hop Drive
	},
	"Electro": {
		"37i9dQZF1DX4dyzvuaRJ0n", // mint
		"37i9dQZF1DX1kCIzMYtzum", // Dance Hits
		"37i9dQZF1DXa41CMuUARjl", // Dance Party
		"37i9dQZF1DX5Q27plkaOQ3", // Dance Rising
	},
	"Années 80": {
		"37i9dQZF1DX4UtSsGT1Sbe", // All Out 80s
		"37i9dQZF1DXb57FjYWz00c", // 80s Hits
	},
	"Années 90": {
		"37i9dQZF1DXbTxeAdrVG2l", // All Out 90s
		"37i9dQZF1DX4o1oenSJRJd", // 90s Hits
	},
	"Années 2000": {
		"37i9dQZF1DX4o1oenSJRJd", // All Out 2000s
		"37i9dQZF1DX3Sp0P28SIer", // 2000s Hits
	},
	"Français": {
		"37i9dQZF1DWU0ScTcjJBdj", // Hits français
		"37i9dQZF1DXd0ZFXhY0CID", // Variété Française
		"37i9dQZF1DX1X2wbzjFCo6", // Chanson française
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
				Timeout: 15 * time.Second,
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

	// Récupérer plus de pistes pour avoir plus de chances d'en trouver avec preview
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
		log.Printf("[Spotify] Erreur API: %s - %s", resp.Status, string(body))
		return nil, fmt.Errorf("erreur Spotify: %s", resp.Status)
	}

	var result struct {
		Items []struct {
			Track *struct {
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
		// Ignorer les items sans track (peut arriver avec des pistes supprimées)
		if item.Track == nil {
			continue
		}
		
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

	log.Printf("[Spotify] Playlist %s: %d pistes avec preview sur %d total", playlistID, len(tracks), len(result.Items))

	return tracks, nil
}

// SearchTracks recherche des pistes par genre/mot-clé
func (c *Client) SearchTracks(query string, limit int) ([]*models.SpotifyTrack, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://api.spotify.com/v1/search?q=%s&type=track&limit=%d&market=FR", 
		url.QueryEscape(query), limit)

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
		return nil, fmt.Errorf("erreur Spotify search: %s", resp.Status)
	}

	var result struct {
		Tracks struct {
			Items []struct {
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
			} `json:"items"`
		} `json:"tracks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var tracks []*models.SpotifyTrack
	for _, item := range result.Tracks.Items {
		if item.PreviewURL == "" {
			continue
		}

		artistNames := make([]string, len(item.Artists))
		for i, a := range item.Artists {
			artistNames[i] = a.Name
		}

		imageURL := ""
		if len(item.Album.Images) > 0 {
			imageURL = item.Album.Images[0].URL
		}

		tracks = append(tracks, &models.SpotifyTrack{
			ID:         item.ID,
			Name:       item.Name,
			Artist:     strings.Join(artistNames, ", "),
			Album:      item.Album.Name,
			PreviewURL: item.PreviewURL,
			ImageURL:   imageURL,
		})
	}

	return tracks, nil
}

// GetRandomTracksForBlindTest récupère des pistes aléatoires pour un Blind Test
func (c *Client) GetRandomTracksForBlindTest(genre string, count int) ([]*models.SpotifyTrack, error) {
	playlists, ok := PlaylistsByGenre[genre]
	if !ok {
		// Genre par défaut
		playlists = PlaylistsByGenre["Pop"]
		log.Printf("[Spotify] Genre '%s' non trouvé, utilisation de Pop", genre)
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

	log.Printf("[Spotify] Total pistes avec preview pour genre '%s': %d", genre, len(allTracks))

	// Si toujours pas de pistes, essayer une recherche
	if len(allTracks) < count {
		log.Printf("[Spotify] Pas assez de pistes, recherche par mot-clé: %s", genre)
		searchTracks, err := c.SearchTracks("genre:"+strings.ToLower(genre), 50)
		if err == nil {
			allTracks = append(allTracks, searchTracks...)
		}
	}

	// Dernière tentative : recherche générique
	if len(allTracks) < count {
		log.Printf("[Spotify] Dernière tentative avec recherche générique")
		searchTracks, err := c.SearchTracks("top hits 2024", 50)
		if err == nil {
			allTracks = append(allTracks, searchTracks...)
		}
	}

	if len(allTracks) == 0 {
		return nil, ErrNoTracks
	}

	// Supprimer les doublons
	seen := make(map[string]bool)
	uniqueTracks := make([]*models.SpotifyTrack, 0)
	for _, track := range allTracks {
		if !seen[track.ID] {
			seen[track.ID] = true
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

	log.Printf("[Spotify] Retourne %d pistes pour le blind test", count)
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
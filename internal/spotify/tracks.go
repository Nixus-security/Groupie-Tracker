package spotify

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

var playlistIDs = map[string]string{
	"Rock": "37i9dQZF1DXcF6B6QPhFDv",
	"Rap":  "37i9dQZF1DX0XUsuxWHRQd",
	"Pop":  "37i9dQZF1DXcBWIGoYBM5M",
}

type PlaylistResponse struct {
	Tracks PlaylistTracks `json:"tracks"`
}

type PlaylistTracks struct {
	Items []PlaylistItem `json:"items"`
	Total int            `json:"total"`
}

type PlaylistItem struct {
	Track SpotifyTrack `json:"track"`
}

type SpotifyTrack struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	PreviewURL string        `json:"preview_url"`
	Album      SpotifyAlbum  `json:"album"`
	Artists    []SpotifyArtist `json:"artists"`
}

type SpotifyAlbum struct {
	Name   string         `json:"name"`
	Images []SpotifyImage `json:"images"`
}

type SpotifyArtist struct {
	Name string `json:"name"`
}

type SpotifyImage struct {
	URL    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
}

type SearchResponse struct {
	Tracks SearchTracks `json:"tracks"`
}

type SearchTracks struct {
	Items []SpotifyTrack `json:"items"`
	Total int            `json:"total"`
}

func GetRandomTrack(playlist string) (*Track, error) {
	if client == nil {
		return getMockTrack(playlist), nil
	}

	playlistID, ok := playlistIDs[playlist]
	if !ok {
		playlistID = playlistIDs["Pop"]
	}

	resp, err := client.Request("GET", fmt.Sprintf("/playlists/%s?fields=tracks.items(track(id,name,preview_url,album(name,images),artists(name)))&limit=50", playlistID))
	if err != nil {
		return getMockTrack(playlist), nil
	}
	defer resp.Body.Close()

	var playlistResp PlaylistResponse
	if err := json.NewDecoder(resp.Body).Decode(&playlistResp); err != nil {
		return getMockTrack(playlist), nil
	}

	var tracksWithPreview []SpotifyTrack
	for _, item := range playlistResp.Tracks.Items {
		if item.Track.PreviewURL != "" {
			tracksWithPreview = append(tracksWithPreview, item.Track)
		}
	}

	if len(tracksWithPreview) == 0 {
		return getMockTrack(playlist), nil
	}

	rand.Seed(time.Now().UnixNano())
	selected := tracksWithPreview[rand.Intn(len(tracksWithPreview))]

	return convertTrack(selected), nil
}

func GetPlaylistTracks(playlist string, limit int) ([]*Track, error) {
	if client == nil {
		return getMockTracks(playlist, limit), nil
	}

	playlistID, ok := playlistIDs[playlist]
	if !ok {
		playlistID = playlistIDs["Pop"]
	}

	resp, err := client.Request("GET", fmt.Sprintf("/playlists/%s?fields=tracks.items(track(id,name,preview_url,album(name,images),artists(name)))&limit=%d", playlistID, limit))
	if err != nil {
		return getMockTracks(playlist, limit), nil
	}
	defer resp.Body.Close()

	var playlistResp PlaylistResponse
	if err := json.NewDecoder(resp.Body).Decode(&playlistResp); err != nil {
		return getMockTracks(playlist, limit), nil
	}

	var tracks []*Track
	for _, item := range playlistResp.Tracks.Items {
		if item.Track.PreviewURL != "" {
			tracks = append(tracks, convertTrack(item.Track))
		}
	}

	return tracks, nil
}

func SearchTrack(query string) (*Track, error) {
	if client == nil {
		return nil, fmt.Errorf("spotify client not initialized")
	}

	resp, err := client.Request("GET", fmt.Sprintf("/search?q=%s&type=track&limit=1", query))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	if len(searchResp.Tracks.Items) == 0 {
		return nil, fmt.Errorf("no track found")
	}

	return convertTrack(searchResp.Tracks.Items[0]), nil
}

func convertTrack(st SpotifyTrack) *Track {
	artist := ""
	if len(st.Artists) > 0 {
		artist = st.Artists[0].Name
	}

	imageURL := ""
	if len(st.Album.Images) > 0 {
		imageURL = st.Album.Images[0].URL
	}

	return &Track{
		ID:         st.ID,
		Name:       st.Name,
		Artist:     artist,
		Album:      st.Album.Name,
		PreviewURL: st.PreviewURL,
		ImageURL:   imageURL,
	}
}

func getMockTrack(playlist string) *Track {
	mockTracks := map[string][]Track{
		"Rock": {
			{ID: "1", Name: "Bohemian Rhapsody", Artist: "Queen", Album: "A Night at the Opera", PreviewURL: "", ImageURL: ""},
			{ID: "2", Name: "Stairway to Heaven", Artist: "Led Zeppelin", Album: "Led Zeppelin IV", PreviewURL: "", ImageURL: ""},
			{ID: "3", Name: "Hotel California", Artist: "Eagles", Album: "Hotel California", PreviewURL: "", ImageURL: ""},
		},
		"Rap": {
			{ID: "4", Name: "Lose Yourself", Artist: "Eminem", Album: "8 Mile", PreviewURL: "", ImageURL: ""},
			{ID: "5", Name: "Juicy", Artist: "The Notorious B.I.G.", Album: "Ready to Die", PreviewURL: "", ImageURL: ""},
			{ID: "6", Name: "N.Y. State of Mind", Artist: "Nas", Album: "Illmatic", PreviewURL: "", ImageURL: ""},
		},
		"Pop": {
			{ID: "7", Name: "Billie Jean", Artist: "Michael Jackson", Album: "Thriller", PreviewURL: "", ImageURL: ""},
			{ID: "8", Name: "Like a Prayer", Artist: "Madonna", Album: "Like a Prayer", PreviewURL: "", ImageURL: ""},
			{ID: "9", Name: "Shape of You", Artist: "Ed Sheeran", Album: "÷", PreviewURL: "", ImageURL: ""},
		},
	}

	tracks, ok := mockTracks[playlist]
	if !ok {
		tracks = mockTracks["Pop"]
	}

	rand.Seed(time.Now().UnixNano())
	selected := tracks[rand.Intn(len(tracks))]

	return &selected
}

func getMockTracks(playlist string, limit int) []*Track {
	var tracks []*Track
	for i := 0; i < limit; i++ {
		tracks = append(tracks, getMockTrack(playlist))
	}
	return tracks
}

func GetAvailablePlaylists() []string {
	return []string{"Rock", "Rap", "Pop"}
}

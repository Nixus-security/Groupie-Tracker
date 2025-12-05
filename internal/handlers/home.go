package handlers

import (
	"net/http"

	"music-platform/internal/database"
)

func Home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	var user *database.User

	cookie, err := r.Cookie(cfg.SessionName)
	if err == nil {
		session, err := database.GetSessionByToken(cookie.Value)
		if err == nil && session != nil {
			user, _ = database.GetUserByID(session.UserID)
		}
	}

	data := map[string]interface{}{
		"User": user,
		"Games": []map[string]string{
			{
				"Name":        "Blind Test",
				"Description": "Devine le titre de la musique le plus vite possible !",
				"Icon":        "🎵",
				"URL":         "/lobby?game=blindtest",
			},
			{
				"Name":        "Petit Bac",
				"Description": "Trouve des mots musicaux commençant par la lettre imposée !",
				"Icon":        "🎼",
				"URL":         "/lobby?game=petitbac",
			},
		},
	}

	renderTemplate(w, "home.html", data)
}

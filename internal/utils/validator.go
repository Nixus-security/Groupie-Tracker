package utils

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func ValidatePseudo(pseudo string) error {
	pseudo = strings.TrimSpace(pseudo)

	if len(pseudo) < 3 {
		return errors.New("Le pseudo doit contenir au moins 3 caractères")
	}

	if len(pseudo) > 20 {
		return errors.New("Le pseudo ne doit pas dépasser 20 caractères")
	}

	if !unicode.IsUpper(rune(pseudo[0])) {
		return errors.New("Le pseudo doit commencer par une majuscule")
	}

	for _, r := range pseudo {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return errors.New("Le pseudo ne peut contenir que des lettres, chiffres et underscores")
		}
	}

	return nil
}

func ValidateEmail(email string) error {
	email = strings.TrimSpace(email)

	if email == "" {
		return errors.New("L'email est requis")
	}

	if len(email) > 255 {
		return errors.New("L'email est trop long")
	}

	if !emailRegex.MatchString(email) {
		return errors.New("Format d'email invalide")
	}

	return nil
}

func ValidatePassword(password string) error {
	if len(password) < 12 {
		return errors.New("Le mot de passe doit contenir au moins 12 caractères")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool

	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return errors.New("Le mot de passe doit contenir au moins une majuscule")
	}

	if !hasLower {
		return errors.New("Le mot de passe doit contenir au moins une minuscule")
	}

	if !hasDigit {
		return errors.New("Le mot de passe doit contenir au moins un chiffre")
	}

	if !hasSpecial {
		return errors.New("Le mot de passe doit contenir au moins un caractère spécial")
	}

	return nil
}

func ValidateRoomCode(code string) error {
	code = strings.TrimSpace(code)

	if len(code) != 6 {
		return errors.New("Le code de salle doit contenir 6 caractères")
	}

	for _, r := range code {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return errors.New("Le code de salle ne peut contenir que des lettres et chiffres")
		}
	}

	return nil
}

func SanitizeString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

func NormalizeAnswer(answer string) string {
	answer = strings.TrimSpace(answer)
	answer = strings.ToLower(answer)

	var result strings.Builder
	for _, r := range answer {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			result.WriteRune(r)
		}
	}

	return result.String()
}
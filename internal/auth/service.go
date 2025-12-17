package auth

import (
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
	"groupie-tracker/internal/database"
	"groupie-tracker/internal/models"
)

var (
	ErrInvalidPseudo      = errors.New("pseudo invalide: doit contenir au moins une majuscule")
	ErrPseudoTaken        = errors.New("ce pseudo est déjà utilisé")
	ErrEmailTaken         = errors.New("cet email est déjà utilisé")
	ErrInvalidEmail       = errors.New("format d'email invalide")
	ErrWeakPassword       = errors.New("mot de passe trop faible (min 12 car., maj, min, chiffre, spécial)")
	ErrUserNotFound       = errors.New("utilisateur non trouvé")
	ErrInvalidCredentials = errors.New("identifiants invalides")
)

type Service struct {
	db *sql.DB
}

func NewService() *Service {
	return &Service{
		db: database.GetDB(),
	}
}

func (s *Service) Register(pseudo, email, password string) (*models.User, error) {
	if !isValidPseudo(pseudo) {
		return nil, ErrInvalidPseudo
	}

	if !isValidEmail(email) {
		return nil, ErrInvalidEmail
	}

	if !isValidPasswordCNIL(password) {
		return nil, ErrWeakPassword
	}

	exists, err := s.pseudoExists(pseudo)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrPseudoTaken
	}

	exists, err = s.emailExists(email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrEmailTaken
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	result, err := s.db.Exec(
		"INSERT INTO users (pseudo, email, password_hash) VALUES (?, ?, ?)",
		pseudo, strings.ToLower(email), string(hashedPassword),
	)
	if err != nil {
		return nil, err
	}

	userID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return s.GetUserByID(userID)
}

func (s *Service) Login(identifier, password string) (*models.User, error) {
	var user models.User

	query := `
		SELECT id, pseudo, email, password_hash, created_at 
		FROM users 
		WHERE pseudo = ? OR email = ?
	`
	err := s.db.QueryRow(query, identifier, strings.ToLower(identifier)).Scan(
		&user.ID, &user.Pseudo, &user.Email, &user.PasswordHash, &user.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return &user, nil
}

func (s *Service) GetUserByID(id int64) (*models.User, error) {
	var user models.User
	query := "SELECT id, pseudo, email, password_hash, created_at FROM users WHERE id = ?"
	err := s.db.QueryRow(query, id).Scan(
		&user.ID, &user.Pseudo, &user.Email, &user.PasswordHash, &user.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func isValidPseudo(pseudo string) bool {
	if len(pseudo) < 3 || len(pseudo) > 30 {
		return false
	}
	hasUpper := false
	for _, r := range pseudo {
		if unicode.IsUpper(r) {
			hasUpper = true
			break
		}
	}
	return hasUpper
}

func isValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

func isValidPasswordCNIL(password string) bool {
	if len(password) < 12 {
		return false
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	specialChars := "!@#$%^&*()_+-=[]{}|;':\",./<>?"

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case strings.ContainsRune(specialChars, char):
			hasSpecial = true
		}
	}

	return hasUpper && hasLower && hasDigit && hasSpecial
}

func (s *Service) pseudoExists(pseudo string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE pseudo = ?", pseudo).Scan(&count)
	return count > 0, err
}

func (s *Service) emailExists(email string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE email = ?", strings.ToLower(email)).Scan(&count)
	return count > 0, err
}
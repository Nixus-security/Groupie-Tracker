// Package auth gère l'authentification des utilisateurs
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
	ErrInvalidPseudo       = errors.New("pseudo invalide: doit contenir au moins une majuscule")
	ErrPseudoTaken         = errors.New("ce pseudo est déjà utilisé")
	ErrEmailTaken          = errors.New("cet email est déjà utilisé")
	ErrInvalidEmail        = errors.New("format d'email invalide")
	ErrWeakPassword        = errors.New("mot de passe trop faible (min 12 car., maj, min, chiffre, spécial)")
	ErrUserNotFound        = errors.New("utilisateur non trouvé")
	ErrInvalidCredentials  = errors.New("identifiants invalides")
)

// Service gère la logique d'authentification
type Service struct {
	db *sql.DB
}

// NewService crée une nouvelle instance du service auth
func NewService() *Service {
	return &Service{
		db: database.GetDB(),
	}
}

// Register inscrit un nouvel utilisateur
func (s *Service) Register(pseudo, email, password string) (*models.User, error) {
	// Valider le pseudo (doit contenir au moins une majuscule)
	if !isValidPseudo(pseudo) {
		return nil, ErrInvalidPseudo
	}

	// Valider l'email
	if !isValidEmail(email) {
		return nil, ErrInvalidEmail
	}

	// Valider le mot de passe selon CNIL
	if !isValidPasswordCNIL(password) {
		return nil, ErrWeakPassword
	}

	// Vérifier si le pseudo existe déjà
	exists, err := s.pseudoExists(pseudo)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrPseudoTaken
	}

	// Vérifier si l'email existe déjà
	exists, err = s.emailExists(email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrEmailTaken
	}

	// Hasher le mot de passe
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Insérer l'utilisateur
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

// Login authentifie un utilisateur par pseudo ou email
func (s *Service) Login(identifier, password string) (*models.User, error) {
	var user models.User

	// Chercher par pseudo ou email
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

	// Vérifier le mot de passe
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return &user, nil
}

// GetUserByID récupère un utilisateur par son ID
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

// ============================================================================
// FONCTIONS DE VALIDATION
// ============================================================================

// isValidPseudo vérifie que le pseudo contient au moins une majuscule
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

// isValidEmail vérifie le format de l'email
func isValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

// isValidPasswordCNIL vérifie le mot de passe selon les recommandations CNIL
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

// pseudoExists vérifie si un pseudo existe déjà
func (s *Service) pseudoExists(pseudo string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE pseudo = ?", pseudo).Scan(&count)
	return count > 0, err
}

// emailExists vérifie si un email existe déjà
func (s *Service) emailExists(email string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE email = ?", strings.ToLower(email)).Scan(&count)
	return count > 0, err
}

package auth

import (
	"database/sql"
	"errors"
	"time"

	"katsironni-hub/internal/db"
)

type AuthService struct {
	db *db.DB
}

func NewAuthService(database *db.DB) *AuthService {
	return &AuthService{db: database}
}

func (s *AuthService) HashPassword(password string) (string, error) {
	return HashPassword(password)
}

func (s *AuthService) VerifyPassword(password, encodedHash string) (bool, error) {
	return VerifyPassword(password, encodedHash)
}

func (s *AuthService) CreateSession(userID string) (string, error) {
	return CreateSession(s.db, userID)
}

func (s *AuthService) ValidateSession(token string) (string, error) {
	return ValidateSession(s.db, token)
}

func (s *AuthService) DeleteSession(token string) error {
	return DeleteSession(s.db, token)
}

func (s *AuthService) CleanExpiredSessions() error {
	return CleanExpiredSessions(s.db)
}

func (s *AuthService) CreateUser(email, password string) (string, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return "", err
	}

	userID := db.GenerateID()
	_, err = s.db.Exec(
		`INSERT INTO users (id, email, password_hash) VALUES (?, ?, ?)`,
		userID, email, hash,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("email already registered")
		}
		return "", err
	}
	return userID, nil
}

func (s *AuthService) GetUserByEmail(email string) (string, string, sql.NullString, error) {
	var userID, hash string
	var emailVerifiedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, password_hash, email_verified_at FROM users WHERE email = ?`,
		email,
	).Scan(&userID, &hash, &emailVerifiedAt)
	return userID, hash, emailVerifiedAt, err
}

func (s *AuthService) VerifyEmail(token string) error {
	tokenHash := HashToken(token)
	var userID string
	var expiresAtStr string
	err := s.db.QueryRow(
		`SELECT user_id, expires_at FROM email_tokens WHERE token = ? AND purpose = 'verify'`,
		tokenHash,
	).Scan(&userID, &expiresAtStr)
	if err != nil {
		return errors.New("invalid or expired token")
	}

	expiresAt, _ := time.Parse(time.RFC3339, expiresAtStr)
	if time.Now().After(expiresAt) {
		return errors.New("token expired")
	}

	_, err = s.db.Exec(
		`UPDATE users SET email_verified_at = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339), userID,
	)
	if err != nil {
		return err
	}

	s.db.Exec(`DELETE FROM email_tokens WHERE token = ?`, tokenHash)
	return nil
}

func (s *AuthService) CreateVerificationToken(userID string) (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", err
	}

	tokenHash := HashToken(token)
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err = s.db.Exec(
		`INSERT INTO email_tokens (id, user_id, token, purpose, expires_at) VALUES (?, ?, ?, 'verify', ?)`,
		db.GenerateID(), userID, tokenHash, expiresAt.Format(time.RFC3339),
	)
	return token, err
}
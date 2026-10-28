package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	"katsironni-hub/internal/db"
)

const (
	argon2Time    = 3
	argon2Memory  = 64 * 1024
	argon2Threads = 4
	argon2KeyLen  = 32
	saltLength    = 16
	tokenLength   = 32
	sessionTTL    = 30 * 24 * time.Hour // 30 days
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionExpired     = errors.New("session expired")
	ErrInvalidToken       = errors.New("invalid token")
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	// Format: $argon2id$v=19$m=65536,t=3,p=4$salt$hash
	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argon2Memory, argon2Time, argon2Threads,
		hex.EncodeToString(salt), hex.EncodeToString(hash))
	return encoded, nil
}

func VerifyPassword(password, encodedHash string) (bool, error) {
	// Parse the encoded hash
	// $argon2id$v=19$m=65536,t=3,p=4$salt$hash
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid hash format")
	}

	var memory, timeCost uint32
	var parallelism uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &parallelism)
	if err != nil {
		return false, err
	}

	salt, err := hex.DecodeString(parts[4])
	if err != nil {
		return false, err
	}

	expectedHash, err := hex.DecodeString(parts[5])
	if err != nil {
		return false, err
	}

	computedHash := argon2.IDKey([]byte(password), salt, timeCost, memory, parallelism, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(computedHash, expectedHash) == 1, nil
}

func GenerateToken() (string, error) {
	b := make([]byte, tokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func CreateSession(database *db.DB, userID string) (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", err
	}

	tokenHash := HashToken(token)
	expiresAt := time.Now().Add(sessionTTL)
	sessionID := db.GenerateID()

	_, err = database.Exec(
		`INSERT INTO sessions (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?)`,
		sessionID, userID, tokenHash, expiresAt.Format(time.RFC3339),
	)
	if err != nil {
		return "", err
	}

	return token, nil
}

func ValidateSession(database *db.DB, token string) (string, error) {
	tokenHash := HashToken(token)

	var userID string
	var expiresAtStr string
	err := database.QueryRow(
		`SELECT user_id, expires_at FROM sessions WHERE token_hash = ?`,
		tokenHash,
	).Scan(&userID, &expiresAtStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrInvalidToken
		}
		return "", err
	}

	expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		return "", err
	}

	if time.Now().After(expiresAt) {
		// Clean up expired session
		database.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
		return "", ErrSessionExpired
	}

	return userID, nil
}

func DeleteSession(database *db.DB, token string) error {
	tokenHash := HashToken(token)
	_, err := database.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func CleanExpiredSessions(database *db.DB) error {
	_, err := database.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Format(time.RFC3339))
	return err
}
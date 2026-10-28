package auth

// API keys for MCP-server and programmatic access. Keys are shown ONCE at generation
// and stored only as SHA-256 hashes. One active key per (user, service); generating
// a new key revokes the previous one.

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"katsironni-hub/internal/db"
)

const apiKeyPrefix = "kk_live_"

func newRawAPIKey() string {
	b := make([]byte, 24)
	rand.Read(b)
	return apiKeyPrefix + hex.EncodeToString(b)
}

// EnsureAPIKey issues a fresh key for user+service, revoking any current active key.
// Returns the raw key (shown to the user exactly once).
func (s *AuthService) EnsureAPIKey(userID, serviceKey string) (string, error) {
	raw := newRawAPIKey()
	if _, err := s.db.Exec(
		`UPDATE api_keys SET status = 'revoked' WHERE user_id = ? AND service_key = ? AND status = 'active'`,
		userID, serviceKey); err != nil {
		return "", err
	}
	_, err := s.db.Exec(
		`INSERT INTO api_keys (id, user_id, service_key, key_hash, status, created_at) VALUES (?, ?, ?, ?, 'active', ?)`,
		db.GenerateID(), userID, serviceKey, HashToken(raw), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	return raw, nil
}

// RevokeAPIKey revokes the active key for user+service.
func (s *AuthService) RevokeAPIKey(userID, serviceKey string) error {
	res, err := s.db.Exec(
		`UPDATE api_keys SET status = 'revoked' WHERE user_id = ? AND service_key = ? AND status = 'active'`,
		userID, serviceKey)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("no active key for this service")
	}
	return nil
}

type APIKeyInfo struct {
	ServiceKey string
	Status     string
	CreatedAt  string
	LastUsedAt string
}

// ListAPIKeys returns the user's keys (metadata only — never the raw keys).
func (s *AuthService) ListAPIKeys(userID string) ([]APIKeyInfo, error) {
	rows, err := s.db.Query(
		`SELECT service_key, status, created_at, COALESCE(last_used_at, '') FROM api_keys
		 WHERE user_id = ? AND status = 'active' ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []APIKeyInfo{}
	for rows.Next() {
		var k APIKeyInfo
		if err := rows.Scan(&k.ServiceKey, &k.Status, &k.CreatedAt, &k.LastUsedAt); err != nil {
			continue
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// ValidateAPIKey checks a raw Bearer key against a service and returns the owner's
// user ID. Entitlement freshness is enforced by the caller (key + subscription both
// must be valid — keycheck endpoint does both checks).
func (s *AuthService) ValidateAPIKey(rawKey, serviceKey string) (string, error) {
	if rawKey == "" || serviceKey == "" {
		return "", errors.New("missing key or service")
	}
	var uid, svc, status string
	err := s.db.QueryRow(
		`SELECT user_id, service_key, status FROM api_keys WHERE key_hash = ?`,
		HashToken(rawKey)).Scan(&uid, &svc, &status)
	if err == sql.ErrNoRows {
		return "", errors.New("unknown key")
	}
	if err != nil {
		return "", err
	}
	if status != "active" {
		return "", errors.New("key revoked")
	}
	if svc != serviceKey {
		return "", errors.New("key not valid for this service")
	}
	_, _ = s.db.Exec(`UPDATE api_keys SET last_used_at = ? WHERE key_hash = ?`,
		time.Now().UTC().Format(time.RFC3339), HashToken(rawKey))
	return uid, nil
}

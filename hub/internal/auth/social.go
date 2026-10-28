package auth

// Social authentication: SIWE (Sign In With Ethereum) and OAuth account linking.

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"

	"katsironni-hub/internal/db"
)

const siweDomain = "katsironni.com"

// CreateSIWENonce validates the wallet address, stores a single-use nonce bound to it,
// and returns the exact EIP-4361-style message the wallet must sign.
func (s *AuthService) CreateSIWENonce(address string) (nonce, message string, err error) {
	address = strings.TrimSpace(address)
	if !common.IsHexAddress(address) {
		return "", "", errors.New("invalid Ethereum address")
	}
	nb := make([]byte, 16)
	if _, err := rand.Read(nb); err != nil {
		return "", "", err
	}
	nonce = hex.EncodeToString(nb)
	message = fmt.Sprintf(
		"%s wants you to sign in with your Ethereum account:\n%s\n\n"+
			"Sign in to Katsironni.\n\n"+
			"URI: https://%s\nVersion: 1\nChain ID: 1\nNonce: %s\nIssued At: %s",
		siweDomain, address, siweDomain, nonce, time.Now().UTC().Format(time.RFC3339))

	_, err = s.db.Exec(
		`INSERT INTO siwe_nonces (nonce, address, message, expires_at) VALUES (?, ?, ?, ?)`,
		nonce, strings.ToLower(address), message, time.Now().Add(10*time.Minute).UTC().Format(time.RFC3339))
	if err != nil {
		return "", "", err
	}
	return nonce, message, nil
}

// VerifySIWE checks the signature against the exact server-issued message,
// consumes the nonce (single use), and returns the canonical wallet address.
func (s *AuthService) VerifySIWE(address, message, signature string) (string, error) {
	address = strings.ToLower(strings.TrimSpace(address))
	if !common.IsHexAddress(address) {
		return "", errors.New("invalid Ethereum address")
	}

	var storedAddr, storedMsg, expiresAt string
	err := s.db.QueryRow(
		`SELECT address, message, expires_at FROM siwe_nonces WHERE nonce = ?`, extractNonce(message),
	).Scan(&storedAddr, &storedMsg, &expiresAt)
	if err == sql.ErrNoRows {
		return "", errors.New("unknown or expired nonce")
	}
	if err != nil {
		return "", err
	}
	// Consume the nonce regardless of outcome (no replay).
	_, _ = s.db.Exec(`DELETE FROM siwe_nonces WHERE nonce = ?`, extractNonce(message))

	if storedAddr != address {
		return "", errors.New("address mismatch")
	}
	if storedMsg != message {
		return "", errors.New("message was not issued by this server")
	}
	exp, _ := time.Parse(time.RFC3339, expiresAt)
	if time.Now().After(exp) {
		return "", errors.New("nonce expired")
	}

	sig := common.FromHex(signature)
	if len(sig) != 65 {
		return "", errors.New("invalid signature length")
	}
	// MetaMask returns v in {27,28}; geth expects {0,1}.
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	hash := accounts.TextHash([]byte(message))
	pub, err := gethcrypto.SigToPub(hash, sig)
	if err != nil {
		return "", errors.New("signature recovery failed")
	}
	recovered := gethcrypto.PubkeyToAddress(*pub).Hex()
	if !strings.EqualFold(recovered, address) {
		return "", errors.New("signature does not match wallet address")
	}
	return strings.ToLower(address), nil
}

func extractNonce(message string) string {
	for _, line := range strings.Split(message, "\n") {
		if strings.HasPrefix(line, "Nonce: ") {
			return strings.TrimPrefix(line, "Nonce: ")
		}
	}
	return ""
}

// FindOrCreateSocialUser resolves a provider identity to a hub user:
// 1. already-linked → that user;
// 2. provider email matches an existing account → LINK to it (no duplicate, no error);
// 3. otherwise create a fresh account (email marked verified — the provider vouched for it).
// Wallet/OAuth users have no usable password; email login remains available only for
// users who set a password (email signup path).
func (s *AuthService) FindOrCreateSocialUser(provider, providerID, email string) (string, error) {
	if provider == "" || providerID == "" {
		return "", errors.New("provider identity missing")
	}
	email = strings.ToLower(strings.TrimSpace(email))

	// 1. already linked
	var uid string
	err := s.db.QueryRow(
		`SELECT user_id FROM oauth_accounts WHERE provider = ? AND provider_account_id = ?`,
		provider, providerID,
	).Scan(&uid)
	if err == nil {
		return uid, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}

	// helper: create the link row
	link := func(userID string) error {
		_, lerr := s.db.Exec(
			`INSERT INTO oauth_accounts (id, user_id, provider, provider_account_id, email) VALUES (?, ?, ?, ?, ?)`,
			db.GenerateID(), userID, provider, providerID, email)
		return lerr
	}

	// 2. link by verified email
	if email != "" && !isSyntheticEmail(email) {
		err = s.db.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&uid)
		if err == nil {
			if lerr := link(uid); lerr != nil {
				return "", lerr
			}
			return uid, nil
		}
		if err != sql.ErrNoRows {
			return "", err
		}
	}

	// 3. create a new account
	if email == "" {
		email = fmt.Sprintf("%s-%s@oauth.katsironni.com", provider, providerID)
	}
	uid = db.GenerateID()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(
		`INSERT INTO users (id, email, password_hash, email_verified_at) VALUES (?, ?, ?, ?)`,
		uid, email, "!", now)
	if err != nil {
		return "", err
	}
	if lerr := link(uid); lerr != nil {
		return "", lerr
	}
	return uid, nil
}

func isSyntheticEmail(email string) bool {
	return strings.HasSuffix(email, "@oauth.katsironni.com")
}

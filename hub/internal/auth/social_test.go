package auth

import (
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"

	"katsironni-hub/internal/db"
)

func testAuthService(t *testing.T) *AuthService {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return NewAuthService(d)
}

func testAddress() (addrHex string, sign func(message string) string) {
	key, _ := gethcrypto.GenerateKey()
	addr := gethcrypto.PubkeyToAddress(key.PublicKey)
	sign = func(message string) string {
		hash := accounts.TextHash([]byte(message))
		sig, err := gethcrypto.Sign(hash, key)
		if err != nil {
			panic(err)
		}
		return hex.EncodeToString(sig)
	}
	return strings.ToLower(addr.Hex()), sign
}

func TestSIWERoundTrip(t *testing.T) {
	svc := testAuthService(t)
	addr, sign := testAddress()

	_, message, err := svc.CreateSIWENonce(addr)
	if err != nil {
		t.Fatalf("nonce: %v", err)
	}
	if !strings.Contains(message, addr) || !strings.Contains(message, "Nonce: ") {
		t.Fatalf("message malformed: %q", message)
	}

	// 1. valid signature verifies and returns the address
	got, err := svc.VerifySIWE(addr, message, sign(message))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got != addr {
		t.Fatalf("got %s want %s", got, addr)
	}

	// 2. replay: nonce was consumed
	if _, err := svc.VerifySIWE(addr, message, sign(message)); err == nil {
		t.Fatal("replay must fail")
	}
}

func TestSIWETamperedAndWrongSigner(t *testing.T) {
	svc := testAuthService(t)
	addr, sign := testAddress()

	_, message, err := svc.CreateSIWENonce(addr)
	if err != nil {
		t.Fatalf("nonce: %v", err)
	}

	// tampered message (not the one the server issued)
	tampered := strings.Replace(message, "Sign in to Katsironni.", "Sign in to Evil.", 1)
	if _, err := svc.VerifySIWE(addr, tampered, sign(tampered)); err == nil {
		t.Fatal("tampered message must be rejected")
	}
	// (nonce survives — rejection path deletes it too, so re-issue)
	_, message, err = svc.CreateSIWENonce(addr)
	if err != nil {
		t.Fatalf("nonce2: %v", err)
	}

	// wrong signer: sign the server's message with a different key
	otherAddr, _ := testAddress()
	_, otherSign := testAddress()
	if otherAddr == addr || otherSign == nil {
		t.Skip("identical keys generated — retry")
	}
	if _, err := svc.VerifySIWE(addr, message, otherSign(message)); err == nil {
		t.Fatal("wrong signer must be rejected")
	}

	// address mismatch: claim a different address than the nonce was issued for
	if _, err := svc.VerifySIWE(otherAddr, message, sign(message)); err == nil {
		t.Fatal("address mismatch must be rejected")
	}
}

func TestFindOrCreateSocialUser(t *testing.T) {
	svc := testAuthService(t)

	// create via provider identity
	uid1, err := svc.FindOrCreateSocialUser("github", "42", "dev@example.com")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// same identity again → same user
	uid2, err := svc.FindOrCreateSocialUser("github", "42", "dev@example.com")
	if err != nil || uid1 != uid2 {
		t.Fatalf("re-login should return same user: %v %s %s", err, uid1, uid2)
	}
	// different provider, same email → LINK to the existing account
	uid3, err := svc.FindOrCreateSocialUser("google", "sub-xyz", "dev@example.com")
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if uid3 != uid1 {
		t.Fatalf("email link failed: %s != %s", uid3, uid1)
	}
	// SIWE with no email → synthetic address user, repeatable
	wallet := "0x85ee7e71f762d772599cbf1ec20e651b30657521"
	w1, err := svc.FindOrCreateSocialUser("siwe", wallet, "")
	if err != nil {
		t.Fatalf("siwe user: %v", err)
	}
	w2, err := svc.FindOrCreateSocialUser("siwe", wallet, "")
	if err != nil || w1 != w2 {
		t.Fatalf("siwe repeat: %v", err)
	}
}

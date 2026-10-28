package auth

import (
	"strings"
	"testing"
)

func TestAPIKeyLifecycle(t *testing.T) {
	svc := testAuthService(t)

	// provision a user via the social path (no password needed)
	uid, err := svc.FindOrCreateSocialUser("siwe", "0xabc123deadbeef", "")
	if err != nil {
		t.Fatalf("user: %v", err)
	}

	// 1. issue a key
	raw1, err := svc.EnsureAPIKey(uid, "contractaudit")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !strings.HasPrefix(raw1, "kk_live_") || len(raw1) < 20 {
		t.Fatalf("bad key format: %q", raw1)
	}
	// 2. validates, returns the owner
	got, err := svc.ValidateAPIKey(raw1, "contractaudit")
	if err != nil || got != uid {
		t.Fatalf("validate: %v %s", err, got)
	}
	// 3. wrong service rejected
	if _, err := svc.ValidateAPIKey(raw1, "agentops"); err == nil {
		t.Fatal("cross-service use must fail")
	}
	// 4. re-issue revokes the old key
	raw2, err := svc.EnsureAPIKey(uid, "contractaudit")
	if err != nil {
		t.Fatalf("reissue: %v", err)
	}
	if raw2 == raw1 {
		t.Fatal("reissue must produce a new key")
	}
	if _, err := svc.ValidateAPIKey(raw1, "contractaudit"); err == nil {
		t.Fatal("old key must be dead after reissue")
	}
	if _, err := svc.ValidateAPIKey(raw2, "contractaudit"); err != nil {
		t.Fatalf("new key must work: %v", err)
	}
	// 5. revoke
	if err := svc.RevokeAPIKey(uid, "contractaudit"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.ValidateAPIKey(raw2, "contractaudit"); err == nil {
		t.Fatal("revoked key must fail")
	}
	// 6. listing shows nothing active
	keys, _ := svc.ListAPIKeys(uid)
	for _, k := range keys {
		if k.ServiceKey == "contractaudit" && k.Status == "active" {
			t.Fatal("list must not show revoked as active")
		}
	}
}

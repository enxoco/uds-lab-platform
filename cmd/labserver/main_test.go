package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var testKey = []byte("test-hmac-key-that-is-at-least-32-bytes-long!")

func TestDemoTokenRoundTrip(t *testing.T) {
	expUnix := time.Now().Add(24 * time.Hour).Unix()
	tokenID, token, err := generateDemoToken(testKey, "uds-package", "ae@company.com", expUnix)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if tokenID == "" || token == "" {
		t.Fatal("empty tokenID or token")
	}

	claims, err := validateDemoToken(testKey, token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.ScenarioID != "uds-package" {
		t.Errorf("scenarioID: got %q, want %q", claims.ScenarioID, "uds-package")
	}
	if claims.AEEmail != "ae@company.com" {
		t.Errorf("aeEmail: got %q, want %q", claims.AEEmail, "ae@company.com")
	}
	if claims.TokenID != tokenID {
		t.Errorf("tokenID mismatch: got %q, want %q", claims.TokenID, tokenID)
	}
}

func TestDemoTokenExpired(t *testing.T) {
	expUnix := time.Now().Add(-1 * time.Hour).Unix()
	_, token, _ := generateDemoToken(testKey, "uds-package", "ae@company.com", expUnix)

	_, err := validateDemoToken(testKey, token)
	if err != errExpiredToken {
		t.Errorf("expected errExpiredToken, got %v", err)
	}
}

func TestDemoTokenTamperedPayload(t *testing.T) {
	expUnix := time.Now().Add(24 * time.Hour).Unix()
	_, token, _ := generateDemoToken(testKey, "uds-package", "ae@company.com", expUnix)

	// Flip a byte in the middle of the token to tamper with payload.
	b := []byte(token)
	b[10] ^= 0xFF
	_, err := validateDemoToken(testKey, string(b))
	if err == nil {
		t.Error("expected error for tampered token, got nil")
	}
}

func TestDemoTokenWrongKey(t *testing.T) {
	expUnix := time.Now().Add(24 * time.Hour).Unix()
	_, token, _ := generateDemoToken(testKey, "uds-package", "ae@company.com", expUnix)

	wrongKey := []byte("wrong-key-that-is-also-at-least-32-bytes-long!")
	_, err := validateDemoToken(wrongKey, token)
	if err != errInvalidToken {
		t.Errorf("expected errInvalidToken for wrong key, got %v", err)
	}
}

func TestDemoTokenTruncated(t *testing.T) {
	expUnix := time.Now().Add(24 * time.Hour).Unix()
	_, token, _ := generateDemoToken(testKey, "uds-package", "ae@company.com", expUnix)
	_, err := validateDemoToken(testKey, token[:8])
	if err == nil {
		t.Error("expected error for truncated token, got nil")
	}
}

func TestDemoClientIDDeterministic(t *testing.T) {
	id1 := demoClientID("tok-abc", "user@example.com")
	id2 := demoClientID("tok-abc", "user@example.com")
	if id1 != id2 {
		t.Errorf("clientID not deterministic: %q vs %q", id1, id2)
	}
	if len(id1) != 32 {
		t.Errorf("clientID wrong length: got %d, want 32", len(id1))
	}
	id3 := demoClientID("tok-abc", "User@EXAMPLE.COM")
	if id1 != id3 {
		t.Errorf("clientID should be case-insensitive for email: %q vs %q", id1, id3)
	}
}

func TestRequireAEAllows(t *testing.T) {
	srv := &server{aeGroup: "/UDS Core/Admin"}
	called := false
	h := srv.requireAE(func(w http.ResponseWriter, r *http.Request) { called = true })

	r := httptest.NewRequest("GET", "/api/demo-tokens", nil)
	r.Header.Set("X-Auth-Request-Groups", "/Other Group")
	r.Header.Set("Authorization", "Bearer "+testJWTWithGroups("/Other Group", "/UDS Core/Admin"))
	w := httptest.NewRecorder()
	h(w, r)

	if !called {
		t.Error("handler was not called for user in AE group")
	}
}

func TestRequireAEBlocks(t *testing.T) {
	srv := &server{aeGroup: "/UDS Core/Admin"}
	called := false
	h := srv.requireAE(func(w http.ResponseWriter, r *http.Request) { called = true })

	r := httptest.NewRequest("GET", "/api/demo-tokens", nil)
	r.Header.Set("X-Auth-Request-Groups", "/UDS Core/Admin")
	r.Header.Set("Authorization", "Bearer "+testJWTWithGroups("/Other Group", "/UDS Core/Administrators"))
	w := httptest.NewRecorder()
	h(w, r)

	if called {
		t.Error("handler should not be called for user NOT in AE group")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func testJWTWithGroups(groups ...string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadJSON, _ := json.Marshal(struct {
		Groups []string `json:"groups"`
	}{Groups: groups})
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	return header + "." + payload + ".test-signature"
}

func TestIsAESubstringFalsePositive(t *testing.T) {
	srv := &server{aeGroup: "/UDS Core/Admin"}
	// "/UDS Core/Administrators" contains "/UDS Core/Admin" as substring —
	// but isAE must use exact match, so this should be false.
	if srv.isAE([]string{"/UDS Core/Administrators"}, "") {
		t.Error("isAE should not match substring — only exact group name")
	}
}

func TestIsAEEmptyGroup(t *testing.T) {
	srv := &server{aeGroup: ""}
	if srv.isAE([]string{"/anything"}, "") {
		t.Error("isAE with empty aeGroup should always return false")
	}
}

func TestDemoBaseURLHTTPS(t *testing.T) {
	r := httptest.NewRequest("GET", "/demo", nil)
	r.Host = "labs.uds.dev"
	r.Header.Set("X-Forwarded-Proto", "https")
	u := demoBaseURL(r)
	if !strings.HasPrefix(u, "https://") {
		t.Errorf("expected https, got %q", u)
	}
}

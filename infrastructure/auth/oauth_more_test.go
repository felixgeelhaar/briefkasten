package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitHostPort(t *testing.T) {
	cases := []struct {
		addr     string
		fallback int
		wantHost string
		wantPort int
	}{
		{"imap.example.org:993", 143, "imap.example.org", 993},
		{"imap.example.org", 993, "imap.example.org", 993},       // no port → default
		{"imap.example.org:imaps", 993, "imap.example.org", 993}, // non-numeric port → default
	}
	for _, tc := range cases {
		host, port := SplitHostPort(tc.addr, tc.fallback)
		if host != tc.wantHost || port != tc.wantPort {
			t.Errorf("SplitHostPort(%q, %d) = %q, %d; want %q, %d",
				tc.addr, tc.fallback, host, port, tc.wantHost, tc.wantPort)
		}
	}
}

// TestXOAuth2NextFailsAfterChallenge locks in the two-step error protocol:
// the first challenge gets an empty response, the second fails the exchange
// carrying the server's error.
func TestXOAuth2NextFailsAfterChallenge(t *testing.T) {
	c := NewXOAuth2Client("alice@example.org", "tok")
	if _, _, err := c.Start(); err != nil {
		t.Fatal(err)
	}
	resp, err := c.Next([]byte(`{"status":"401"}`))
	if err != nil || len(resp) != 0 {
		t.Fatalf("first Next = %q, %v; want empty response, nil", resp, err)
	}
	if _, err := c.Next([]byte(`{"status":"401"}`)); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("second Next err = %v, want failure carrying the challenge", err)
	}
}

// TestTokenRefreshFlow mints an access token from a refresh token against a
// local token endpoint, and surfaces endpoint failures.
func TestTokenRefreshFlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if got := r.Form.Get("refresh_token"); got != "rtok" {
			t.Errorf("refresh_token = %q, want rtok", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"fresh-tok","token_type":"Bearer","expires_in":3600}`)
	}))
	defer srv.Close()

	o := &OAuth2Settings{
		ClientID:     "cid",
		ClientSecret: "csec",
		RefreshToken: "rtok",
		TokenURL:     srv.URL,
	}
	tok, err := o.Token(t.Context())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "fresh-tok" {
		t.Errorf("token = %q, want fresh-tok", tok)
	}

	// A failing endpoint surfaces as a wrapped oauth2 error.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	}))
	defer bad.Close()
	o2 := &OAuth2Settings{ClientID: "cid", RefreshToken: "rtok", TokenURL: bad.URL}
	if _, err := o2.Token(t.Context()); err == nil || !strings.Contains(err.Error(), "oauth2: token") {
		t.Errorf("failing endpoint err = %v, want wrapped oauth2: token error", err)
	}
}

// TestSASLClientTokenError propagates a token failure instead of building a
// client with empty credentials.
func TestSASLClientTokenError(t *testing.T) {
	o := &OAuth2Settings{} // no token, no refresh token, no credentials file
	if _, err := o.SASLClient(t.Context(), "alice@example.org", "imap.example.org", 993); err == nil {
		t.Error("SASLClient without any credentials accepted")
	}
}

// TestLoadCredentialsFromFile reads the credentials from disk (the
// CredentialsFile path, not the in-memory bytes).
func TestLoadCredentialsFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.json")
	if err := os.WriteFile(path, fakeOAuthClientJSON(), 0o600); err != nil {
		t.Fatal(err)
	}
	o := &OAuth2Settings{CredentialsFile: path}
	if err := o.LoadCredentials(t.Context(), "alice@example.org"); err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if o.ClientID != "cid.apps.googleusercontent.com" {
		t.Errorf("client_id not hydrated from file: %q", o.ClientID)
	}
	if o.TokenURL != "https://oauth2.googleapis.com/token" {
		t.Errorf("token_url = %q", o.TokenURL)
	}
}

// TestLoadCredentialsParseErrors covers the malformed-content rejections:
// non-JSON, a service-account key Google's parser rejects, and an OAuth
// client secret with the wrong shape.
func TestLoadCredentialsParseErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"not JSON", `this is not json`},
		// type probes as service_account but private_key has the wrong type,
		// so google.JWTConfigFromJSON rejects it.
		{"bad service-account key", `{"type":"service_` + `account","private_key":5}`},
		// web probes non-empty but is not an object, so google.ConfigFromJSON
		// rejects it.
		{"bad OAuth client secret", `{"web":5}`},
	}
	for _, tc := range cases {
		o := &OAuth2Settings{CredentialsJSON: []byte(tc.raw)}
		if err := o.LoadCredentials(t.Context(), "alice@example.org"); err == nil {
			t.Errorf("%s accepted", tc.name)
		}
	}
}

package auth

import (
	"testing"
)

func TestXOAuth2InitialResponse(t *testing.T) {
	c := NewXOAuth2Client("alice@example.org", "tok-123")
	mech, ir, err := c.Start()
	if err != nil {
		t.Fatal(err)
	}
	if mech != "XOAUTH2" {
		t.Errorf("mech = %q", mech)
	}
	want := "user=alice@example.org\x01auth=Bearer tok-123\x01\x01"
	if string(ir) != want {
		t.Errorf("ir = %q, want %q", ir, want)
	}
	resp, err := c.Next([]byte(`{"status":"401"}`))
	if err != nil || len(resp) != 0 {
		t.Errorf("Next = %q, %v", resp, err)
	}
}

func TestOAuth2SettingsToken(t *testing.T) {
	o := &OAuth2Settings{AccessToken: "static-tok"}
	tok, err := o.Token(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "static-tok" {
		t.Errorf("token = %q", tok)
	}
	if _, err := (&OAuth2Settings{}).Token(t.Context()); err == nil {
		t.Error("empty settings accepted")
	}
}

func TestSASLClientMechanisms(t *testing.T) {
	o := &OAuth2Settings{AccessToken: "tok"}
	c, err := o.SASLClient(t.Context(), "alice@example.org", "imap.example.org", 993)
	if err != nil {
		t.Fatal(err)
	}
	mech, _, _ := c.Start()
	if mech != "XOAUTH2" {
		t.Errorf("default mech = %q, want XOAUTH2", mech)
	}

	o.Mechanism = "oauthbearer"
	c, err = o.SASLClient(t.Context(), "alice@example.org", "imap.example.org", 993)
	if err != nil {
		t.Fatal(err)
	}
	mech, _, _ = c.Start()
	if mech != "OAUTHBEARER" {
		t.Errorf("mech = %q, want OAUTHBEARER", mech)
	}

	o.Mechanism = "bogus"
	if _, err := o.SASLClient(t.Context(), "a", "h", 1); err == nil {
		t.Error("bogus mechanism accepted")
	}
}

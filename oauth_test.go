package briefkasten

import (
	"io"
	"net"
	"strings"
	"testing"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

func TestXOAuth2InitialResponse(t *testing.T) {
	c := newXOAuth2Client("alice@example.org", "tok-123")
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
	// Error challenge: client answers empty so the server can fail the
	// exchange cleanly.
	resp, err := c.Next([]byte(`{"status":"401"}`))
	if err != nil || len(resp) != 0 {
		t.Errorf("Next = %q, %v", resp, err)
	}
}

func TestOAuth2SettingsTokenSource(t *testing.T) {
	// Static access token: no refresh dance needed (tests, short-lived use).
	o := &OAuth2Settings{AccessToken: "static-tok"}
	tok, err := o.token(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "static-tok" {
		t.Errorf("token = %q", tok)
	}

	// Without any token material the settings are invalid.
	if _, err := (&OAuth2Settings{}).token(t.Context()); err == nil {
		t.Error("empty settings accepted")
	}
}

func TestSASLForMechanisms(t *testing.T) {
	o := &OAuth2Settings{AccessToken: "tok"}
	c, err := o.saslClient(t.Context(), "alice@example.org", "imap.example.org", 993)
	if err != nil {
		t.Fatal(err)
	}
	mech, _, _ := c.Start()
	if mech != "XOAUTH2" {
		t.Errorf("default mech = %q, want XOAUTH2", mech)
	}

	o.Mechanism = "oauthbearer"
	c, err = o.saslClient(t.Context(), "alice@example.org", "imap.example.org", 993)
	if err != nil {
		t.Fatal(err)
	}
	mech, _, _ = c.Start()
	if mech != "OAUTHBEARER" {
		t.Errorf("mech = %q, want OAUTHBEARER", mech)
	}

	o.Mechanism = "bogus"
	if _, err := o.saslClient(t.Context(), "a", "h", 1); err == nil {
		t.Error("bogus mechanism accepted")
	}
}

// oauthSMTPSession accepts OAUTHBEARER with one specific token.
type oauthSMTPSession struct {
	memSMTPSession
	token string
}

func (s *oauthSMTPSession) AuthMechanisms() []string { return []string{"OAUTHBEARER"} }

func (s *oauthSMTPSession) Auth(mech string) (sasl.Server, error) {
	return sasl.NewOAuthBearerServer(func(opts sasl.OAuthBearerOptions) *sasl.OAuthBearerError {
		if opts.Token != s.token {
			return &sasl.OAuthBearerError{Status: "invalid_token"}
		}
		return nil
	}), nil
}

func TestSMTPSenderOAuthBearer(t *testing.T) {
	backend := &memSMTP{}
	srv := smtp.NewServer(smtp.BackendFunc(func(*smtp.Conn) (smtp.Session, error) {
		return &oauthSMTPSession{memSMTPSession: memSMTPSession{s: backend}, token: "tok-987"}, nil
	}))
	srv.Domain = "localhost"
	srv.AllowInsecureAuth = true
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	sender, err := NewSMTPSender(SMTPConfig{
		Addr:     ln.Addr().String(),
		From:     "alice@example.org",
		Username: "alice@example.org",
		Insecure: true,
		OAuth2:   &OAuth2Settings{AccessToken: "tok-987", Mechanism: "oauthbearer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = sender.Send(t.Context(), OutboundMessage{
		ID: "o1", To: []string{"x@y.z"}, Subject: "OAuth", Body: "hi",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if !strings.Contains(backend.data, "OAuth") {
		t.Errorf("not delivered: %q", backend.data)
	}

	// Wrong token fails.
	bad, _ := NewSMTPSender(SMTPConfig{
		Addr: ln.Addr().String(), From: "alice@example.org", Username: "alice@example.org",
		Insecure: true,
		OAuth2:   &OAuth2Settings{AccessToken: "wrong", Mechanism: "oauthbearer"},
	})
	if err := bad.Send(t.Context(), OutboundMessage{ID: "o2", To: []string{"x@y.z"}, Subject: "x", Body: "y"}); err == nil {
		t.Error("wrong token accepted")
	}
	_ = io.Discard
}

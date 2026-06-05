package briefkasten_test

import (
	"bytes"
	"errors"
	"net"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/felixgeelhaar/briefkasten"
)

const testMessage = "From: amt@finanzamt.example\r\nSubject: Bescheid\r\n\r\nSehr geehrte Damen und Herren,\r\n"

type literal struct {
	*bytes.Reader
	size int64
}

func (l literal) Size() int64 { return l.size }

// startIMAPServer runs an in-memory IMAP server with one user and one
// unseen message in INBOX. Returns the listen address.
func startIMAPServer(t *testing.T) string {
	t.Helper()

	user := imapmemserver.NewUser("alice", "secret")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	raw := []byte(testMessage)
	if _, err := user.Append("INBOX", literal{bytes.NewReader(raw), int64(len(raw))}, &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}

	mem := imapmemserver.New()
	mem.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String()
}

func newTestIMAPMailbox(t *testing.T, addr string) *briefkasten.IMAPMailbox {
	t.Helper()
	mb, err := briefkasten.NewIMAPMailbox(briefkasten.IMAPConfig{
		Addr:     addr,
		Username: "alice",
		Password: "secret",
		Insecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return mb
}

func TestIMAPMailboxRoundTrip(t *testing.T) {
	mb := newTestIMAPMailbox(t, startIMAPServer(t))

	ids, err := mb.ListUnread()
	if err != nil {
		t.Fatalf("ListUnread: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("unread = %v, want one id", ids)
	}

	raw, err := mb.Fetch(ids[0])
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(raw) != testMessage {
		t.Errorf("raw = %q, want %q", raw, testMessage)
	}

	// Fetch must peek: message stays unread.
	ids, err = mb.ListUnread()
	if err != nil {
		t.Fatalf("ListUnread after fetch: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("unread after fetch = %v, want still one (BODY.PEEK)", ids)
	}

	if err := mb.MarkSeen(ids[0]); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	ids, err = mb.ListUnread()
	if err != nil {
		t.Fatalf("ListUnread after seen: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("unread after seen = %v, want none", ids)
	}
}

func TestIMAPMailboxBadID(t *testing.T) {
	mb := newTestIMAPMailbox(t, startIMAPServer(t))

	if _, err := mb.Fetch("not-a-uid"); !errors.Is(err, briefkasten.ErrBadID) {
		t.Errorf("Fetch(not-a-uid) err = %v, want ErrBadID", err)
	}
	if _, err := mb.Fetch("999"); !errors.Is(err, briefkasten.ErrBadID) {
		t.Errorf("Fetch(999) err = %v, want ErrBadID", err)
	}
	if err := mb.MarkSeen("not-a-uid"); !errors.Is(err, briefkasten.ErrBadID) {
		t.Errorf("MarkSeen(not-a-uid) err = %v, want ErrBadID", err)
	}
}

func TestIMAPMailboxBadCredentials(t *testing.T) {
	addr := startIMAPServer(t)
	mb, err := briefkasten.NewIMAPMailbox(briefkasten.IMAPConfig{
		Addr:     addr,
		Username: "alice",
		Password: "wrong",
		Insecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mb.ListUnread(); err == nil {
		t.Error("ListUnread with bad credentials: want error")
	}
}

func TestNewIMAPMailboxValidation(t *testing.T) {
	if _, err := briefkasten.NewIMAPMailbox(briefkasten.IMAPConfig{}); err == nil {
		t.Error("empty config: want error")
	}
}

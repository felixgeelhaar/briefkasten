package briefkasten

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/felixgeelhaar/mcp-go/testutil"
)

func TestDirMailboxFolders(t *testing.T) {
	mb, root := newDir(t)
	drop(t, root, "in-root.eml", "From: a@b\r\n\r\nroot")
	// A sub-maildir "steuern".
	if err := os.MkdirAll(filepath.Join(root, "steuern", "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "steuern", "new", "s.eml"),
		[]byte("From: amt@fa.example\r\nSubject: Bescheid\r\n\r\nx"), 0o644); err != nil {
		t.Fatal(err)
	}

	folders, err := mb.Folders()
	if err != nil {
		t.Fatalf("Folders: %v", err)
	}
	want := map[string]bool{"INBOX": true, "steuern": true}
	for _, f := range folders {
		delete(want, f)
	}
	if len(want) != 0 {
		t.Errorf("folders = %v, missing %v", folders, want)
	}

	scoped, err := mb.InFolder("steuern")
	if err != nil {
		t.Fatalf("InFolder: %v", err)
	}
	ids, err := scoped.ListUnread()
	if err != nil || len(ids) != 1 || ids[0] != "s.eml" {
		t.Errorf("scoped ids = %v err = %v", ids, err)
	}

	// INBOX resolves to the root.
	inbox, err := mb.InFolder("INBOX")
	if err != nil {
		t.Fatal(err)
	}
	ids, _ = inbox.ListUnread()
	if len(ids) != 1 || ids[0] != "in-root.eml" {
		t.Errorf("inbox ids = %v", ids)
	}

	// Traversal rejected.
	if _, err := mb.InFolder("../outside"); err == nil {
		t.Error("traversal folder accepted")
	}
}

func TestIMAPMailboxFolders(t *testing.T) {
	user := imapmemserver.NewUser("alice", "secret")
	for _, name := range []string{"INBOX", "Steuern"} {
		if err := user.Create(name, nil); err != nil {
			t.Fatal(err)
		}
	}
	raw := []byte("From: amt@fa.example\r\nSubject: Bescheid\r\n\r\nx")
	if _, err := user.Append("Steuern", memLiteral{bytes.NewReader(raw), int64(len(raw))}, &imap.AppendOptions{}); err != nil {
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

	mb, err := NewIMAPMailbox(IMAPConfig{Addr: ln.Addr().String(), Username: "alice", Password: "secret", Insecure: true})
	if err != nil {
		t.Fatal(err)
	}

	folders, err := mb.Folders()
	if err != nil {
		t.Fatalf("Folders: %v", err)
	}
	found := map[string]bool{}
	for _, f := range folders {
		found[f] = true
	}
	if !found["INBOX"] || !found["Steuern"] {
		t.Errorf("folders = %v", folders)
	}

	scoped, err := mb.InFolder("Steuern")
	if err != nil {
		t.Fatal(err)
	}
	ids, err := scoped.ListUnread()
	if err != nil || len(ids) != 1 {
		t.Errorf("scoped ids = %v err = %v", ids, err)
	}
}

func TestToolsAcceptFolderParam(t *testing.T) {
	mb, root := newDir(t)
	if err := os.MkdirAll(filepath.Join(root, "steuern", "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "steuern", "new", "s.eml"),
		[]byte("From: amt@fa.example\r\nSubject: Bescheid\r\n\r\nSteuer"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := testutil.NewTestClient(t, NewServer(mb))

	out := callMap(t, client, "email.list_unread", map[string]any{"folder": "steuern"})
	ids := out["ids"].([]string)
	if len(ids) != 1 || ids[0] != "s.eml" {
		t.Fatalf("ids = %v", ids)
	}

	fetched := callMap(t, client, "email.fetch", map[string]any{"id": "s.eml", "folder": "steuern"})
	if fetched["raw"] == "" {
		t.Error("empty fetch")
	}

	found := callMap(t, client, "email.search", map[string]any{"query": "Steuer", "folder": "steuern"})
	if n := len(found["ids"].([]string)); n != 1 {
		t.Errorf("search ids = %d", n)
	}

	callMap(t, client, "email.mark_seen", map[string]any{"id": "s.eml", "folder": "steuern"})
	out = callMap(t, client, "email.list_unread", map[string]any{"folder": "steuern"})
	if n := len(out["ids"].([]string)); n != 0 {
		t.Errorf("unread after seen = %d", n)
	}
}

func TestFoldersResource(t *testing.T) {
	mb, root := newDir(t)
	if err := os.MkdirAll(filepath.Join(root, "steuern", "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(mb)
	RegisterResources(srv, mb, nil)
	client := testutil.NewTestClient(t, srv)

	text, err := client.ReadResource("email://folders")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	for _, want := range []string{"INBOX", "steuern"} {
		if !bytes.Contains([]byte(text), []byte(want)) {
			t.Errorf("folders resource missing %q: %s", want, text)
		}
	}
}

func TestSwitchableForwardsFolderAndSearch(t *testing.T) {
	mb, root := newDir(t)
	if err := os.MkdirAll(filepath.Join(root, "steuern", "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	drop(t, root, "a.eml", "From: x@y.z\r\nSubject: Spende\r\n\r\nDanke")
	sw := NewSwitchable(mb)

	folders, err := sw.Folders()
	if err != nil || len(folders) < 2 {
		t.Errorf("folders = %v err = %v", folders, err)
	}
	ids, err := sw.Search("Spende")
	if err != nil || len(ids) != 1 {
		t.Errorf("search = %v err = %v", ids, err)
	}
	if _, err := sw.InFolder("steuern"); err != nil {
		t.Errorf("InFolder: %v", err)
	}
}

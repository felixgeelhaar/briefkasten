package maildir

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.klarlabs.de/briefkasten/domain"
)

func newOutbox(t *testing.T) (*OutboxStore, string) {
	t.Helper()
	root := t.TempDir()
	st, err := NewOutboxStore(root)
	if err != nil {
		t.Fatalf("NewOutboxStore: %v", err)
	}
	return st, root
}

func TestOutboxStoreNewCreatesStateDirs(t *testing.T) {
	_, root := newOutbox(t)
	for _, state := range []string{"queued", "sending", "sent", "failed"} {
		info, err := os.Stat(filepath.Join(root, state))
		if err != nil || !info.IsDir() {
			t.Errorf("state dir %s not created: %v", state, err)
		}
	}
}

func TestOutboxStoreWriteFindRoundtrip(t *testing.T) {
	st, _ := newOutbox(t)
	msg := domain.OutboundMessage{
		ID:      "m1",
		To:      []string{"you@example.com"},
		Subject: "Hello",
		Body:    "hi there",
		State:   "queued",
	}
	if err := st.Write(msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := st.Find("m1")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.ID != msg.ID || got.Subject != msg.Subject || got.Body != msg.Body || got.State != "queued" {
		t.Errorf("Find = %+v, want %+v", got, msg)
	}
	if len(got.To) != 1 || got.To[0] != "you@example.com" {
		t.Errorf("Find To = %v, want [you@example.com]", got.To)
	}
}

func TestOutboxStoreFindAfterStateChange(t *testing.T) {
	st, _ := newOutbox(t)
	msg := domain.OutboundMessage{ID: "m1", To: []string{"you@example.com"}, State: "queued"}
	if err := st.Write(msg); err != nil {
		t.Fatalf("Write queued: %v", err)
	}

	msg.State = "sent"
	if err := st.Write(msg); err != nil {
		t.Fatalf("Write sent: %v", err)
	}
	if err := st.Remove("queued", "m1"); err != nil {
		t.Fatalf("Remove queued: %v", err)
	}

	got, err := st.Find("m1")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.State != "sent" {
		t.Errorf("State = %q, want sent", got.State)
	}
}

func TestOutboxStoreFindUnknown(t *testing.T) {
	st, _ := newOutbox(t)
	_, err := st.Find("nope")
	if err == nil {
		t.Fatal("unknown id accepted")
	}
	if !errors.Is(err, domain.ErrBadID) {
		t.Errorf("Find error = %v, want ErrBadID", err)
	}
}

func TestOutboxStoreFindRejectsTraversal(t *testing.T) {
	st, _ := newOutbox(t)
	_, err := st.Find("../x")
	if err == nil {
		t.Fatal("path traversal accepted in Find")
	}
	if !errors.Is(err, domain.ErrBadID) {
		t.Errorf("Find error = %v, want ErrBadID", err)
	}
}

func TestOutboxStoreList(t *testing.T) {
	st, _ := newOutbox(t)
	for _, id := range []string{"a", "b"} {
		msg := domain.OutboundMessage{ID: id, To: []string{"you@example.com"}, State: "queued"}
		if err := st.Write(msg); err != nil {
			t.Fatalf("Write %s: %v", id, err)
		}
	}

	ids, err := st.List("queued")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Errorf("List = %v, want [a b]", ids)
	}

	empty, err := st.List("failed")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("List failed = %v, want empty", empty)
	}
}

func TestOutboxStoreRemoveMissing(t *testing.T) {
	st, _ := newOutbox(t)
	if err := st.Remove("queued", "nope"); err == nil {
		t.Error("Remove of missing record accepted")
	}
}

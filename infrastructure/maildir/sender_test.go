package maildir

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.klarlabs.de/briefkasten/domain"
)

func TestNewSenderRejectsBadFrom(t *testing.T) {
	for _, from := range []string{"", "bad\r\naddress", "not-an-address"} {
		if _, err := NewSender(t.TempDir(), from); err == nil {
			t.Errorf("NewSender(%q) accepted, want error", from)
		}
	}
}

func TestSenderSend(t *testing.T) {
	root := t.TempDir()
	s, err := NewSender(root, "me@example.com")
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}

	msg := domain.OutboundMessage{
		ID:      "m1",
		To:      []string{"you@example.com"},
		Subject: "Hello",
		Body:    "hi there",
		State:   "sending",
	}
	if err := s.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "new", "m1.eml"))
	if err != nil {
		t.Fatalf("delivered message missing: %v", err)
	}
	if !strings.Contains(string(raw), "From: me@example.com") {
		t.Error("delivered message missing From header")
	}
	if !strings.Contains(string(raw), "Subject: Hello") {
		t.Error("delivered message missing Subject header")
	}

	entries, err := os.ReadDir(filepath.Join(root, "tmp"))
	if err != nil {
		t.Fatalf("tmp/ missing: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("tmp/ not empty after send: %d entries", len(entries))
	}
}

func TestSenderSendDeliverError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits are not enforced")
	}
	root := t.TempDir()
	s, err := NewSender(root, "me@example.com")
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}

	newDir := filepath.Join(root, "new")
	if err := os.Chmod(newDir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(newDir, 0o700); err != nil {
			t.Errorf("restore chmod: %v", err)
		}
	})

	msg := domain.OutboundMessage{
		ID:      "m1",
		To:      []string{"you@example.com"},
		Subject: "Hello",
		Body:    "hi",
		State:   "sending",
	}
	if err := s.Send(context.Background(), msg); err == nil {
		t.Error("Send into read-only new/ accepted, want error")
	}
}

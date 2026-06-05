package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCLIConfig prepares a maildir with one unread message and returns
// the config path.
func writeCLIConfig(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new", "m1.eml"),
		[]byte("From: a@b.c\r\nSubject: CLI\r\n\r\nhallo"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(t.TempDir(), "briefkasten.yaml")
	if err := os.WriteFile(cfgPath, []byte("maildir: "+root+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath, root
}

func runCLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestCLIListAndRead(t *testing.T) {
	cfg, _ := writeCLIConfig(t)

	code, out, _ := runCLI(t, "", "list", "--config", cfg)
	if code != 0 || !strings.Contains(out, "m1.eml") {
		t.Fatalf("list = %d %q", code, out)
	}

	code, out, _ = runCLI(t, "", "read", "--config", cfg, "m1.eml")
	if code != 0 || !strings.Contains(out, "Subject: CLI") {
		t.Fatalf("read = %d %q", code, out)
	}

	code, out, _ = runCLI(t, "", "search", "--config", cfg, "hallo")
	if code != 0 || !strings.Contains(out, "m1.eml") {
		t.Fatalf("search = %d %q", code, out)
	}

	code, out, _ = runCLI(t, "", "list", "--config", cfg, "--json")
	if code != 0 || !strings.Contains(out, `"ids"`) {
		t.Fatalf("json list = %d %q", code, out)
	}
}

func TestCLIDeletePromptsAndAborts(t *testing.T) {
	cfg, root := writeCLIConfig(t)

	// "n" aborts; the message survives.
	code, out, _ := runCLI(t, "n\n", "delete", "--config", cfg, "m1.eml")
	if code == 0 {
		t.Fatalf("aborted delete exited 0: %q", out)
	}
	if _, err := os.Stat(filepath.Join(root, "new", "m1.eml")); err != nil {
		t.Fatal("message gone despite abort")
	}

	// "y" proceeds to trash.
	code, _, errOut := runCLI(t, "y\n", "delete", "--config", cfg, "m1.eml")
	if code != 0 {
		t.Fatalf("confirmed delete failed: %s", errOut)
	}
	if _, err := os.Stat(filepath.Join(root, ".trash", "new", "m1.eml")); err != nil {
		t.Errorf("not in trash: %v", err)
	}
}

func TestCLIArchiveWithYes(t *testing.T) {
	cfg, root := writeCLIConfig(t)
	code, _, errOut := runCLI(t, "", "archive", "--config", cfg, "--yes", "m1.eml")
	if code != 0 {
		t.Fatalf("archive --yes failed: %s", errOut)
	}
	if _, err := os.Stat(filepath.Join(root, ".archive", "new", "m1.eml")); err != nil {
		t.Errorf("not archived: %v", err)
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	code, _, errOut := runCLI(t, "", "frobnicate")
	if code != 2 || !strings.Contains(errOut, "usage") {
		t.Fatalf("unknown = %d %q", code, errOut)
	}
}

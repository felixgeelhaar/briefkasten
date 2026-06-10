package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIFlagParseError(t *testing.T) {
	code, _, errOut := runCLI(t, "", "list", "--no-such-flag")
	if code != 2 {
		t.Errorf("bad flag = %d, want 2 (%s)", code, errOut)
	}
}

func TestCLIConfigLoadError(t *testing.T) {
	code, _, errOut := runCLI(t, "", "list", "--config", "/no/such/briefkasten.yaml")
	if code != 1 || !strings.Contains(errOut, "config:") {
		t.Errorf("missing config = %d %q, want 1 with config error", code, errOut)
	}
}

func TestCLIBuildServiceError(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "briefkasten.yaml")
	if err := os.WriteFile(cfgPath, []byte("backend: carrier-pigeon\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI(t, "", "list", "--config", cfgPath)
	if code != 1 || !strings.Contains(errOut, "carrier-pigeon") {
		t.Errorf("unknown backend = %d %q, want 1 naming the backend", code, errOut)
	}
}

func TestCLIBuildServiceAccountsError(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "briefkasten.yaml")
	cfg := "maildir: " + filepath.Join(root, "in") + "\n" +
		"accounts:\n" +
		"  broken:\n" +
		"    backend: carrier-pigeon\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI(t, "", "list", "--config", cfgPath)
	if code != 1 || !strings.Contains(errOut, "broken") {
		t.Errorf("bad account = %d %q, want 1 naming the account", code, errOut)
	}
}

func TestCLIUsageErrorsForMissingArgs(t *testing.T) {
	cfg, _ := writeCLIConfig(t)
	for _, cmd := range []string{"read", "seen", "search", "retry", "archive", "delete"} {
		code, _, errOut := runCLI(t, "", cmd, "--config", cfg)
		if code != 2 || !strings.Contains(errOut, "usage:") {
			t.Errorf("%s without argument = %d %q, want 2 with usage", cmd, code, errOut)
		}
	}
}

func TestCLICommandErrorsSurface(t *testing.T) {
	cfg, _ := writeCLIConfig(t)

	// Unknown account fails every mailbox command with exit 1.
	for _, args := range [][]string{
		{"list", "--config", cfg, "--account", "nope"},
		{"read", "--config", cfg, "--account", "nope", "m1.eml"},
		{"seen", "--config", cfg, "--account", "nope", "m1.eml"},
		{"search", "--config", cfg, "--account", "nope", "hallo"},
		{"folders", "--config", cfg, "--account", "nope"},
	} {
		code, _, errOut := runCLI(t, "", args...)
		if code != 1 || !strings.Contains(errOut, "unknown account") {
			t.Errorf("%v = %d %q, want 1 with unknown-account error", args, code, errOut)
		}
	}

	// Curation of an unknown id fails after the --yes gate.
	code, _, errOut := runCLI(t, "", "archive", "--config", cfg, "--yes", "ghost.eml")
	if code != 1 || errOut == "" {
		t.Errorf("archive unknown id = %d %q, want 1 with error", code, errOut)
	}
}

func TestCLISeenAndFolders(t *testing.T) {
	cfg, root := writeCLIConfig(t)

	code, out, errOut := runCLI(t, "", "seen", "--config", cfg, "m1.eml")
	if code != 0 || !strings.Contains(out, "seen: m1.eml") {
		t.Fatalf("seen = %d %q %q", code, out, errOut)
	}
	// The message left new/ — it is acknowledged, not destroyed.
	if _, err := os.Stat(filepath.Join(root, "new", "m1.eml")); !os.IsNotExist(err) {
		t.Errorf("message still unread after seen: %v", err)
	}

	code, out, _ = runCLI(t, "", "folders", "--config", cfg, "--json")
	if code != 0 || !strings.Contains(out, `"folders"`) || !strings.Contains(out, "INBOX") {
		t.Errorf("folders --json = %d %q", code, out)
	}
}

func TestCLISendUsageAndValidation(t *testing.T) {
	cfgPath := writeOutboxConfig(t)

	// Missing --to/--subject/--body is a usage error.
	code, _, errOut := runCLI(t, "", "send", "--config", cfgPath, "--subject", "S", "--body", "B")
	if code != 2 || !strings.Contains(errOut, "usage: briefkasten send") {
		t.Errorf("send without recipients = %d %q, want 2 with usage", code, errOut)
	}

	// send without any outbox configured is rejected.
	plainCfg, _ := writeCLIConfig(t)
	code, _, errOut = runCLI(t, "", "send", "--config", plainCfg,
		"--to", "a@b.c", "--subject", "S", "--body", "B")
	if code != 1 || !strings.Contains(errOut, "no outbox configured") {
		t.Errorf("send without outbox = %d %q, want 1", code, errOut)
	}

	// A missing attachment file aborts before anything is queued.
	code, _, errOut = runCLI(t, "", "send", "--config", cfgPath,
		"--to", "a@b.c", "--subject", "S", "--body", "B",
		"--attach", "/no/such/file.pdf")
	if code != 1 || !strings.Contains(errOut, "/no/such/file.pdf") {
		t.Errorf("send with missing attachment = %d %q, want 1 naming the file", code, errOut)
	}

	// An invalid recipient address is rejected by the outbox validation.
	code, _, errOut = runCLI(t, "", "send", "--config", cfgPath,
		"--to", "@@not-an-address@@", "--subject", "S", "--body", "B")
	if code != 1 || errOut == "" {
		t.Errorf("send to invalid address = %d %q, want 1 with error", code, errOut)
	}
}

func TestCLISendAndRetryWithBrokenOutboxConfig(t *testing.T) {
	// outbox.dir set but no from: the dir sender cannot be built.
	root := t.TempDir()
	cfgPath := filepath.Join(root, "briefkasten.yaml")
	cfg := "maildir: " + filepath.Join(root, "in") + "\n" +
		"outbox:\n" +
		"  dir: " + filepath.Join(root, "outbox") + "\n" +
		"  deliver_dir: " + filepath.Join(root, "delivered") + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runCLI(t, "", "send", "--config", cfgPath,
		"--to", "a@b.c", "--subject", "S", "--body", "B")
	if code != 1 || !strings.Contains(errOut, "From") {
		t.Errorf("send with broken outbox = %d %q, want 1 with From error", code, errOut)
	}
	code, _, errOut = runCLI(t, "", "retry", "--config", cfgPath, "some-id")
	if code != 1 || !strings.Contains(errOut, "From") {
		t.Errorf("retry with broken outbox = %d %q, want 1 with From error", code, errOut)
	}
}

// TestCLIRetryRecoversFailedDelivery drives a message through
// failed → retry-still-failing → retry-delivered, the full human repair loop.
func TestCLIRetryRecoversFailedDelivery(t *testing.T) {
	root := t.TempDir()
	deliverDir := filepath.Join(root, "delivered")
	cfgPath := filepath.Join(root, "briefkasten.yaml")
	cfg := "maildir: " + filepath.Join(root, "in") + "\n" +
		"outbox:\n" +
		"  dir: " + filepath.Join(root, "outbox") + "\n" +
		"  from: me@x.y\n" +
		"  deliver_dir: " + deliverDir + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	// Sabotage delivery: the dir sender writes via tmp/, so an unwritable
	// tmp/ fails every send attempt.
	tmpDir := filepath.Join(deliverDir, "tmp")
	if err := os.MkdirAll(deliverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(tmpDir, 0o500); err != nil {
		t.Fatal(err)
	}
	restore := func() {
		if err := os.Chmod(tmpDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(restore)

	code, out, errOut := runCLI(t, "", "send", "--config", cfgPath,
		"--to", "a@b.c", "--subject", "S", "--body", "B")
	if code != 0 {
		t.Fatalf("send = %d %q %q", code, out, errOut)
	}
	if !strings.Contains(out, "failed: ") {
		t.Fatalf("send with broken delivery = %q, want failed state", out)
	}
	id := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "failed:"))

	// Retry while delivery is still broken: state stays failed and the
	// human sees the delivery error.
	code, out, _ = runCLI(t, "", "retry", "--config", cfgPath, id)
	if code != 0 || !strings.Contains(out, "failed: "+id) || !strings.Contains(out, "(") {
		t.Errorf("retry while broken = %d %q, want failed with the error in parentheses", code, out)
	}

	// Repair delivery and retry again: the message goes out.
	restore()
	code, out, errOut = runCLI(t, "", "retry", "--config", cfgPath, id)
	if code != 0 || !strings.Contains(out, "sent: "+id) {
		t.Fatalf("retry after repair = %d %q %q, want sent", code, out, errOut)
	}
	entries, err := os.ReadDir(filepath.Join(deliverDir, "new"))
	if err != nil || len(entries) != 1 {
		t.Errorf("delivered messages = %v, %v; want exactly one", entries, err)
	}
}

func TestLoadConfigPathFromEnvAndCwd(t *testing.T) {
	// BRIEFKASTEN_CONFIG points at the config when --config is not given.
	cfg, _ := writeCLIConfig(t)
	t.Setenv("BRIEFKASTEN_CONFIG", cfg)
	code, out, errOut := runCLI(t, "", "list")
	if code != 0 || !strings.Contains(out, "m1.eml") {
		t.Errorf("list via env config = %d %q %q", code, out, errOut)
	}

	// Without the env var, a briefkasten.yaml in the working directory wins.
	t.Setenv("BRIEFKASTEN_CONFIG", "")
	dir := t.TempDir()
	maildirRoot := filepath.Join(dir, "box")
	if err := os.MkdirAll(filepath.Join(maildirRoot, "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(maildirRoot, "new", "cwd.eml"),
		[]byte("From: a@b.c\r\nSubject: Cwd\r\n\r\nx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "briefkasten.yaml"),
		[]byte("maildir: "+maildirRoot+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	code, out, errOut = runCLI(t, "", "list")
	if code != 0 || !strings.Contains(out, "cwd.eml") {
		t.Errorf("list via cwd config = %d %q %q", code, out, errOut)
	}
}

func TestCLINoArgsDispatchesToServe(t *testing.T) {
	// Empty args fall through to the MCP server; with a broken config the
	// server start fails fast.
	t.Setenv("BRIEFKASTEN_CONFIG", "/no/such/briefkasten.yaml")
	if code, _, _ := runCLI(t, ""); code != 1 {
		t.Errorf("run with no args and broken config = %d, want 1", code)
	}
}

func TestCLIOutboxWithBrokenOutboxConfig(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "briefkasten.yaml")
	cfg := "maildir: " + filepath.Join(root, "in") + "\n" +
		"outbox:\n" + // dir set, but no from: the sender cannot be built
		"  dir: " + filepath.Join(root, "outbox") + "\n" +
		"  deliver_dir: " + filepath.Join(root, "delivered") + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI(t, "", "outbox", "--config", cfgPath)
	if code != 1 || !strings.Contains(errOut, "From") {
		t.Errorf("outbox with broken sender = %d %q, want 1 with From error", code, errOut)
	}
}

// writeRecordBlockedOutbox prepares an outbox whose sent/ state directory is
// read-only: delivery succeeds, but recording the success fails — the
// ProcessOnce error path.
func writeRecordBlockedOutbox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	outboxDir := filepath.Join(root, "outbox")
	sentDir := filepath.Join(outboxDir, "sent")
	if err := os.MkdirAll(sentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sentDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(sentDir, 0o700); err != nil {
			t.Error(err)
		}
	})
	cfgPath := filepath.Join(root, "briefkasten.yaml")
	cfg := "maildir: " + filepath.Join(root, "in") + "\n" +
		"outbox:\n" +
		"  dir: " + outboxDir + "\n" +
		"  from: me@x.y\n" +
		"  deliver_dir: " + filepath.Join(root, "delivered") + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func TestCLISendReportsRecordingFailure(t *testing.T) {
	cfgPath := writeRecordBlockedOutbox(t)
	code, _, errOut := runCLI(t, "", "send", "--config", cfgPath,
		"--to", "a@b.c", "--subject", "S", "--body", "B")
	if code != 1 || !strings.Contains(errOut, "record delivery") {
		t.Errorf("send with blocked state recording = %d %q, want 1 with record error", code, errOut)
	}
}

func TestCLIRetryReportsRecordingFailure(t *testing.T) {
	cfgPath := writeRecordBlockedOutbox(t)
	// First send leaves the message stranded in sending→failed bookkeeping;
	// it actually ends up failed because the success could not be recorded
	// and a later run sees it under sending. Simplest deterministic setup:
	// run send (exit 1, message recorded under sending), recover via a new
	// CLI run whose Recover moves it to failed, then retry it.
	code, _, _ := runCLI(t, "", "send", "--config", cfgPath,
		"--to", "a@b.c", "--subject", "S", "--body", "B")
	if code != 1 {
		t.Fatalf("priming send = %d, want 1", code)
	}
	// Find the stranded id from the outbox listing (Recover files it under
	// failed for explicit retry).
	code, out, errOut := runCLI(t, "", "outbox", "--config", cfgPath)
	if code != 0 || !strings.Contains(out, "failed: ") {
		t.Fatalf("outbox after stranded send = %d %q %q", code, out, errOut)
	}
	var id string
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "failed: "); ok {
			id = strings.TrimSpace(rest)
		}
	}
	if id == "" {
		t.Fatalf("no failed id in %q", out)
	}

	code, _, errOut = runCLI(t, "", "retry", "--config", cfgPath, id)
	if code != 1 || !strings.Contains(errOut, "record delivery") {
		t.Errorf("retry with blocked state recording = %d %q, want 1 with record error", code, errOut)
	}
}

func TestLoadAttachmentsSniffsContentType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "NOTES") // no extension → content sniffing
	if err := os.WriteFile(path, []byte("plain text notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	atts, err := loadAttachments([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if got := atts[0].ContentType; !strings.HasPrefix(got, "text/plain") {
		t.Errorf("sniffed content type = %q, want text/plain", got)
	}
}

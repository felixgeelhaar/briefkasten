// Command briefkasten serves a mailbox as an MCP server.
//
// Maildir backend (default):
//
//	BRIEFKASTEN_ADDR=:8090 BRIEFKASTEN_MAILDIR=./maildir briefkasten
//
// Drop .eml files into <maildir>/new; consumers pull them via the
// email.list_unread / email.fetch / email.mark_seen tools and seen messages
// move to <maildir>/cur.
//
// IMAP backend (selected when BRIEFKASTEN_IMAP_ADDR is set):
//
//	BRIEFKASTEN_IMAP_ADDR=imap.example.org:993 \
//	BRIEFKASTEN_IMAP_USER=alice BRIEFKASTEN_IMAP_PASSWORD=... briefkasten
//
// Optional: BRIEFKASTEN_IMAP_MAILBOX (default INBOX),
// BRIEFKASTEN_IMAP_INSECURE=1 for plaintext IMAP (local/testing only).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	mcp "github.com/felixgeelhaar/mcp-go"

	"github.com/felixgeelhaar/briefkasten"
)

func main() {
	addr := env("BRIEFKASTEN_ADDR", ":8090")

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	mailbox, desc, err := buildMailbox()
	if err != nil {
		log.Error("mailbox init failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("briefkasten listening", "addr", addr, "backend", desc)
	if err := mcp.ServeHTTP(ctx, briefkasten.NewServer(mailbox), addr); err != nil && ctx.Err() == nil {
		log.Error("serve failed", "error", err)
		os.Exit(1)
	}
}

// buildMailbox selects the backend: IMAP when BRIEFKASTEN_IMAP_ADDR is set,
// maildir otherwise.
func buildMailbox() (briefkasten.Mailbox, string, error) {
	if imapAddr := os.Getenv("BRIEFKASTEN_IMAP_ADDR"); imapAddr != "" {
		mb, err := briefkasten.NewIMAPMailbox(briefkasten.IMAPConfig{
			Addr:     imapAddr,
			Username: os.Getenv("BRIEFKASTEN_IMAP_USER"),
			Password: os.Getenv("BRIEFKASTEN_IMAP_PASSWORD"),
			Mailbox:  os.Getenv("BRIEFKASTEN_IMAP_MAILBOX"),
			Insecure: os.Getenv("BRIEFKASTEN_IMAP_INSECURE") == "1",
		})
		return mb, "imap " + imapAddr, err
	}

	dir := env("BRIEFKASTEN_MAILDIR", "./maildir")
	mb, err := briefkasten.NewDirMailbox(dir)
	return mb, "maildir " + dir, err
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

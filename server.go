package briefkasten

import (
	"context"
	"encoding/base64"

	mcp "github.com/felixgeelhaar/mcp-go"
)

// NewServer exposes a Mailbox as an MCP server. The three email.* tools are
// the connector contract: any client that speaks them can consume any
// backend.
func NewServer(mb Mailbox) *mcp.Server {
	srv := mcp.NewServer(mcp.ServerInfo{Name: "briefkasten", Version: "0.5.0"},
		mcp.WithInstructions(Instructions))

	srv.Tool("email.list_unread").
		Description("List ids of unread messages. Optional: folder (see email://folders).").
		ReadOnly().
		UIResource(InboxUIResourceURI).
		OutputSchema(map[string]any{"ids": []string{"m1.eml"}}).
		Handler(func(_ context.Context, in struct {
			Folder string `json:"folder,omitempty"`
		}) (map[string]any, error) {
			box, err := scoped(mb, in.Folder)
			if err != nil {
				return nil, err
			}
			ids, err := box.ListUnread()
			if err != nil {
				return nil, err
			}
			if ids == nil {
				ids = []string{}
			}
			return map[string]any{"ids": ids}, nil
		})

	srv.Tool("email.fetch").
		Description("Fetch the raw RFC 5322 message for an unread id, base64-encoded.").
		ReadOnly().
		OutputSchema(map[string]any{"raw": "<base64>"}).
		Handler(func(_ context.Context, in struct {
			ID     string `json:"id"`
			Folder string `json:"folder,omitempty"`
		}) (map[string]any, error) {
			box, err := scoped(mb, in.Folder)
			if err != nil {
				return nil, err
			}
			raw, err := box.Fetch(in.ID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"raw": base64.StdEncoding.EncodeToString(raw)}, nil
		})

	srv.Tool("email.mark_seen").
		Description("Mark a message as seen so it is not ingested again.").
		Idempotent().
		OutputSchema(map[string]any{"ok": true}).
		Handler(func(_ context.Context, in struct {
			ID     string `json:"id"`
			Folder string `json:"folder,omitempty"`
		}) (map[string]any, error) {
			box, err := scoped(mb, in.Folder)
			if err != nil {
				return nil, err
			}
			if err := box.MarkSeen(in.ID); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true}, nil
		})

	srv.Tool("email.search").
		Description("Search unread messages for a text query (case-insensitive). Returns matching ids.").
		ReadOnly().
		OutputSchema(map[string]any{"ids": []string{"m1.eml"}}).
		Handler(func(_ context.Context, in struct {
			Query  string `json:"query"`
			Folder string `json:"folder,omitempty"`
		}) (map[string]any, error) {
			box, err := scoped(mb, in.Folder)
			if err != nil {
				return nil, err
			}
			ids, err := searchMailbox(box, in.Query)
			if err != nil {
				return nil, err
			}
			if ids == nil {
				ids = []string{}
			}
			return map[string]any{"ids": ids}, nil
		})

	return srv
}

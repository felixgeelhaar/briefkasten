# briefkasten

**A mailbox as an MCP server.**

Briefkasten (German: *letterbox*) exposes any message store through three
[Model Context Protocol](https://modelcontextprotocol.io) tools, so agent
runtimes and ingestion pipelines pull mail through a stable, language-agnostic
contract instead of binding to IMAP libraries:

| Tool | Does |
|---|---|
| `email.list_unread` | `{}` → `{"ids": ["..."]}` |
| `email.fetch` | `{"id": "..."}` → `{"raw": "<base64 RFC 5322>"}` |
| `email.mark_seen` | `{"id": "..."}` → `{"ok": true}` — message won't be listed again |

Built on [mcp-go](https://github.com/felixgeelhaar/mcp-go).

## Run

```bash
go install github.com/felixgeelhaar/briefkasten/cmd/briefkasten@latest

BRIEFKASTEN_ADDR=:8090 BRIEFKASTEN_MAILDIR=./maildir briefkasten
```

The default backend is a maildir-style directory: drop `.eml` files into
`<maildir>/new` — that's "receiving mail". Consumers fetch and mark seen;
seen messages move to `<maildir>/cur`. Ideal for development, testing, and
pipelines that already export messages to disk.

### IMAP backend

Set `BRIEFKASTEN_IMAP_ADDR` to serve a real mailbox instead:

```bash
BRIEFKASTEN_IMAP_ADDR=imap.example.org:993 \
BRIEFKASTEN_IMAP_USER=alice \
BRIEFKASTEN_IMAP_PASSWORD=... \
briefkasten
```

Ids are message UIDs. `email.list_unread` is `UID SEARCH UNSEEN`,
`email.fetch` reads `BODY.PEEK[]` (fetching never sets `\Seen`), and
`email.mark_seen` stores `+FLAGS \Seen`. Each call dials a fresh
connection — no state to lose across server restarts or idle timeouts.
Optional: `BRIEFKASTEN_IMAP_MAILBOX` (default `INBOX`),
`BRIEFKASTEN_IMAP_INSECURE=1` for plaintext IMAP (local/testing only).

## Consume

Any MCP client works. With mcp-go:

```go
transport, _ := client.NewHTTPTransport("http://localhost:8090")
c := client.New(transport)
c.Initialize(ctx)

res, _ := c.CallTool(ctx, "email.list_unread", map[string]any{})
// fetch each id, ingest, then email.mark_seen — only after success,
// so failures stay unread for retry.
```

## Bring your own backend

Implement the `Mailbox` port and serve it:

```go
type Mailbox interface {
    ListUnread() ([]string, error)
    Fetch(id string) ([]byte, error)
    MarkSeen(id string) error
}

mcp.ServeHTTP(ctx, briefkasten.NewServer(myIMAPBox), ":8090")
```

Gmail, Exchange, a database queue — anything that can list, fetch, and
acknowledge. The tool contract stays identical for every consumer.
(Maildir and IMAP ship built-in: `NewDirMailbox`, `NewIMAPMailbox`.)

## Design notes

- **Mark-seen is the consumer's acknowledgement.** Briefkasten never deletes;
  backends decide what "seen" means (maildir move, IMAP flag, …).
- **Ids are opaque** to consumers and validated by backends (the dir backend
  rejects path traversal).
- **Raw bytes, not parsed mail.** Parsing/MIME policy belongs to the
  consumer; the wire format is base64 RFC 5322.

## License

MIT

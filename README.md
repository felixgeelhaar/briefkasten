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

## Configure

Three layers, 12-factor precedence — **env > config file > defaults**:

```yaml
# briefkasten.yaml (or point BRIEFKASTEN_CONFIG elsewhere)
addr: ":8090"
backend: imap            # or maildir; inferred from imap.addr when omitted
maildir: ./maildir
imap:
  addr: imap.example.org:993
  username: alice
  password: "..."
  mailbox: INBOX
runtime_config: false    # enable config.get / config.set MCP tools
```

Every key has an env override: `BRIEFKASTEN_ADDR`, `BRIEFKASTEN_BACKEND`,
`BRIEFKASTEN_MAILDIR`, `BRIEFKASTEN_IMAP_ADDR` / `_USER` / `_PASSWORD` /
`_MAILBOX` / `_INSECURE`, `BRIEFKASTEN_RUNTIME_CONFIG`.

### Runtime reconfiguration over MCP

With `runtime_config: true` two extra tools are served:

| Tool | Does |
|---|---|
| `config.get` | Active configuration — credentials redacted |
| `config.set` | Partial patch: validates the new backend, hot-swaps it, persists to the config file |

A failed `config.set` leaves the old backend serving. Off by default —
`config.set` accepts mailbox credentials, so enable it only on trusted
networks.

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

Remote backends are wrapped in [fortify](https://github.com/felixgeelhaar/fortify)
resilience automatically: per-call timeout, exponential-backoff retry,
and a circuit breaker that fast-fails while the server is down. Bad
message ids are never retried and never trip the breaker.

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

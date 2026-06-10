package mcpserver

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/klarlabs-studio/auth-go/domain"
	mcp "go.klarlabs.de/mcp"
	"go.klarlabs.de/mcp/middleware"
	"go.klarlabs.de/mcp/protocol"
	"go.klarlabs.de/mcp/transport"
)

// ForwardAuthorizationHeader returns the HTTP transport option that
// copies each request's Authorization header into the request meta,
// where auth middleware (operating on the unwrapped protocol request)
// can see it. Pair it with BasicAuth when serving over HTTP.
func ForwardAuthorizationHeader() mcp.HTTPOption {
	return transport.WithRequestContextFn(func(ctx context.Context, r *http.Request) context.Context {
		if v := r.Header.Get("Authorization"); v != "" {
			ctx = protocol.SetRequestMeta(ctx, "Authorization", v)
		}
		return ctx
	})
}

// BasicAuth returns MCP middleware that requires every request to carry
// an Authorization: Basic credential matching the given username and
// argon2id password hash (a PHC string, e.g. from auth-go HashPassword).
//
// Verification is constant time on both the username and the password
// (auth-go's PasswordHash.Verify). Authentication failures are opaque —
// the same rejection regardless of which part was wrong. The MCP
// handshake methods (initialize, ping) stay reachable so clients can
// negotiate before presenting credentials; every tool, resource, and
// prompt call is gated. MCP clients are not browsers, so there is no
// cookie/session handshake: the credential rides each request, which is
// what HTTP basic auth is built for.
func BasicAuth(username string, hash domain.PasswordHash) mcp.Middleware {
	wantUser := []byte(username)
	return middleware.Auth(func(ctx context.Context, _ *protocol.Request) (*middleware.Identity, error) {
		auth := protocol.GetRequestMeta(ctx, "Authorization")
		if auth == "" {
			auth = protocol.GetRequestMeta(ctx, "authorization")
		}
		const prefix = "Basic "
		if !strings.HasPrefix(auth, prefix) {
			return nil, nil
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, prefix))
		if err != nil {
			return nil, nil
		}
		user, pass, ok := strings.Cut(string(raw), ":")
		if !ok {
			return nil, nil
		}
		// Check both halves unconditionally so a known username is not
		// distinguishable from an unknown one by timing.
		userOK := subtle.ConstantTimeCompare([]byte(user), wantUser) == 1
		passErr := hash.Verify(pass)
		if !userOK || passErr != nil {
			return nil, nil
		}
		return &middleware.Identity{ID: username, Name: username}, nil
	}, middleware.WithAuthRealm("briefkasten"))
}

package mcpserver

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/klarlabs-studio/auth-go/domain"
	"go.klarlabs.de/mcp/protocol"
)

func basicHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// invoke runs the middleware chain against a request carrying the given
// Authorization header and reports whether next ran.
func invoke(t *testing.T, hash domain.PasswordHash, method, authorization string) (called bool, err error) {
	t.Helper()
	mw := BasicAuth("alice", hash)
	next := func(_ context.Context, _ *protocol.Request) (*protocol.Response, error) {
		called = true
		return &protocol.Response{}, nil
	}
	ctx := context.Background()
	if authorization != "" {
		ctx = protocol.SetRequestMeta(ctx, "Authorization", authorization)
	}
	_, err = mw(next)(ctx, &protocol.Request{Method: method})
	return called, err
}

func TestBasicAuthMiddleware(t *testing.T) {
	hash, err := domain.HashPassword("s3cret", domain.DefaultArgon2idParams())
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		header string
		want   bool
	}{
		{"valid credentials", basicHeader("alice", "s3cret"), true},
		{"wrong password", basicHeader("alice", "nope"), false},
		{"wrong username", basicHeader("mallory", "s3cret"), false},
		{"no header", "", false},
		{"bearer scheme", "Bearer sometoken", false},
		{"malformed base64", "Basic !!!", false},
		{"no colon", "Basic " + base64.StdEncoding.EncodeToString([]byte("alice")), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called, err := invoke(t, hash, "tools/list", tc.header)
			if called != tc.want {
				t.Errorf("next called = %v, want %v (err %v)", called, tc.want, err)
			}
			if !tc.want {
				var perr *protocol.Error
				if !errors.As(err, &perr) || perr.Code != protocol.CodeUnauthorized {
					t.Errorf("want unauthorized protocol error, got %v", err)
				}
			}
		})
	}
}

func TestBasicAuthMiddlewareSkipsHandshake(t *testing.T) {
	hash, err := domain.HashPassword("s3cret", domain.DefaultArgon2idParams())
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"initialize", "ping"} {
		called, err := invoke(t, hash, method, "")
		if !called || err != nil {
			t.Errorf("%s should pass without credentials, called=%v err=%v", method, called, err)
		}
	}
}

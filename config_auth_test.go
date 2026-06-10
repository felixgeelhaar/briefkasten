package briefkasten_test

import (
	"testing"

	authdomain "github.com/klarlabs-studio/auth-go/domain"

	"go.klarlabs.de/briefkasten"
)

func TestBasicAuthSettingsEnabled(t *testing.T) {
	cases := []struct {
		name string
		b    briefkasten.BasicAuthSettings
		want bool
	}{
		{"off by default", briefkasten.BasicAuthSettings{}, false},
		{"username only", briefkasten.BasicAuthSettings{Username: "a"}, false},
		{"password only", briefkasten.BasicAuthSettings{Password: "p"}, false},
		{"username+password", briefkasten.BasicAuthSettings{Username: "a", Password: "p"}, true},
		{"username+hash", briefkasten.BasicAuthSettings{Username: "a", PasswordHash: "$argon2id$x"}, true},
	}
	for _, tc := range cases {
		if got := tc.b.Enabled(); got != tc.want {
			t.Errorf("%s: Enabled() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBuildAuthMiddleware(t *testing.T) {
	t.Run("disabled returns nil", func(t *testing.T) {
		var cfg briefkasten.Config
		mw, err := cfg.BuildAuthMiddleware()
		if err != nil || mw != nil {
			t.Fatalf("want nil, nil; got %v, %v", mw, err)
		}
	})

	t.Run("plaintext password hashed at startup", func(t *testing.T) {
		var cfg briefkasten.Config
		cfg.Auth.Basic = briefkasten.BasicAuthSettings{Username: "a", Password: "p"}
		mw, err := cfg.BuildAuthMiddleware()
		if err != nil || mw == nil {
			t.Fatalf("want middleware; got %v, %v", mw, err)
		}
	})

	t.Run("precomputed hash accepted", func(t *testing.T) {
		hash, err := authdomain.HashPassword("p", authdomain.DefaultArgon2idParams())
		if err != nil {
			t.Fatal(err)
		}
		var cfg briefkasten.Config
		cfg.Auth.Basic = briefkasten.BasicAuthSettings{Username: "a", PasswordHash: hash.String()}
		mw, err := cfg.BuildAuthMiddleware()
		if err != nil || mw == nil {
			t.Fatalf("want middleware; got %v, %v", mw, err)
		}
	})

	t.Run("malformed hash rejected", func(t *testing.T) {
		var cfg briefkasten.Config
		cfg.Auth.Basic = briefkasten.BasicAuthSettings{Username: "a", PasswordHash: "not-a-phc-string"}
		if _, err := cfg.BuildAuthMiddleware(); err == nil {
			t.Fatal("want error for malformed hash")
		}
	})
}

func TestApplyEnvAuth(t *testing.T) {
	t.Setenv("BRIEFKASTEN_AUTH_USER", "envuser")
	t.Setenv("BRIEFKASTEN_AUTH_PASSWORD", "envpass")
	t.Setenv("BRIEFKASTEN_AUTH_PASSWORD_HASH", "$argon2id$env")
	var cfg briefkasten.Config
	cfg.ApplyEnv()
	b := cfg.Auth.Basic
	if b.Username != "envuser" || b.Password != "envpass" || b.PasswordHash != "$argon2id$env" {
		t.Errorf("env overlay missed: %+v", b)
	}
}

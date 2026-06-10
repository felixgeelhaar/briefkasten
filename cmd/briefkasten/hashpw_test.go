package main

import (
	"encoding/json"
	"strings"
	"testing"

	authdomain "github.com/klarlabs-studio/auth-go/domain"
)

func TestCLIHashpw(t *testing.T) {
	code, out, _ := runCLI(t, "s3cret\n", "hashpw")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	// Output is "password: " prompt followed by the PHC string.
	phc := strings.TrimSpace(strings.TrimPrefix(out, "password: "))
	hash, err := authdomain.PasswordHashFromString(phc)
	if err != nil {
		t.Fatalf("output is not a PHC hash: %q: %v", phc, err)
	}
	if err := hash.Verify("s3cret"); err != nil {
		t.Errorf("hash does not verify the password: %v", err)
	}
	if err := hash.Verify("wrong"); err == nil {
		t.Error("hash verifies a wrong password")
	}
}

func TestCLIHashpwJSON(t *testing.T) {
	code, out, _ := runCLI(t, "s3cret\n", "hashpw", "--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	idx := strings.Index(out, "{")
	if idx < 0 {
		t.Fatalf("no JSON in output: %q", out)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(out[idx:]), &got); err != nil {
		t.Fatalf("json output: %v", err)
	}
	if _, err := authdomain.PasswordHashFromString(got["password_hash"]); err != nil {
		t.Errorf("password_hash not a PHC string: %v", err)
	}
}

func TestCLIHashpwEmpty(t *testing.T) {
	code, _, errOut := runCLI(t, "\n", "hashpw")
	if code != 1 || !strings.Contains(errOut, "empty password") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

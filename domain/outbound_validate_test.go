package domain

import (
	"strings"
	"testing"
)

func TestValidateAddress(t *testing.T) {
	cases := []struct {
		addr string
		ok   bool
	}{
		{"a@b.c", true},
		{"Alice <a@b.c>", true},
		{"not-an-address", false},
		{"", false},
		{"a@b.c\r\nBcc: evil@x.y", false},
		{"a@b.c\nBcc: evil@x.y", false}, // bare LF is enough to reject
		{"a@b.c\r", false},
	}
	for _, tc := range cases {
		err := ValidateAddress(tc.addr)
		if tc.ok && err != nil {
			t.Errorf("ValidateAddress(%q) = %v, want nil", tc.addr, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("ValidateAddress(%q) accepted", tc.addr)
		}
	}
}

func TestValidateAddressRejectsLineBreaksBeforeParsing(t *testing.T) {
	// The CR/LF check fires even for inputs the parser might tolerate —
	// the error must name the line break, not a parse failure.
	err := ValidateAddress("a@b.c\nX: y")
	if err == nil {
		t.Fatal("address with LF accepted")
	}
	if !strings.Contains(err.Error(), "line breaks") {
		t.Errorf("err = %q, want mention of line breaks", err)
	}
}

func TestValidateChecksRecipients(t *testing.T) {
	cases := []struct {
		name string
		to   []string
		ok   bool
	}{
		{"header injection via CRLF", []string{"a@b.c\r\nBcc: evil@x.y"}, false},
		{"bare invalid address", []string{"not-an-address"}, false},
		{"empty recipient", []string{""}, false},
		{"one bad among good", []string{"a@b.c", "not-an-address"}, false},
		{"plain address", []string{"a@b.c"}, true},
		{"name-addr form", []string{"Alice <a@b.c>"}, true},
	}
	for _, tc := range cases {
		msg := OutboundMessage{To: tc.to, Subject: "s", Body: "b"}
		err := msg.Validate()
		if tc.ok && err != nil {
			t.Errorf("%s: Validate = %v, want nil", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s: Validate accepted %q", tc.name, tc.to)
		}
	}
}

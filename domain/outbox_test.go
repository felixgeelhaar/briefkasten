package domain

import "testing"

func TestTransition(t *testing.T) {
	cases := []struct {
		from, event, want string
		ok                bool
	}{
		{"queued", "SEND", "sending", true},
		{"sending", "SUCCEED", "sent", true},
		{"sending", "FAIL", "failed", true},
		{"failed", "RETRY", "queued", true},
		{"queued", "RETRY", "", false},
		{"sent", "SEND", "", false},
		{"sent", "RETRY", "", false},
		{"failed", "SEND", "", false},
	}
	for _, tc := range cases {
		got, err := Transition(tc.from, tc.event)
		if tc.ok && (err != nil || got != tc.want) {
			t.Errorf("%s --%s--> got %q err %v, want %q", tc.from, tc.event, got, err, tc.want)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s --%s--> accepted", tc.from, tc.event)
		}
	}
}

func TestOutboundMessageValidate(t *testing.T) {
	if err := (OutboundMessage{}).Validate(); err == nil {
		t.Error("no recipients accepted")
	}
	if err := (OutboundMessage{To: []string{"a@b.c"}}).Validate(); err != nil {
		t.Errorf("valid message rejected: %v", err)
	}
}

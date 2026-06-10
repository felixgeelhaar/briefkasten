package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"go.klarlabs.de/mcp/server"
	"go.klarlabs.de/mcp/testutil"

	"go.klarlabs.de/briefkasten/application"
)

func promptText(t *testing.T, client *testutil.TestClient, name string, args map[string]string) string {
	t.Helper()
	result, err := client.GetPrompt(name, args)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestDraftReplyPrompt(t *testing.T) {
	client, root := newClient(t)
	drop(t, root, "m1.eml", "From: amt@fa.example\r\nSubject: Bescheid\r\n\r\nBitte antworten.")

	text := promptText(t, client, "draft_reply", map[string]string{"id": "m1.eml"})
	if !strings.Contains(text, "Bitte antworten.") || !strings.Contains(text, "Original (m1.eml)") {
		t.Errorf("draft_reply does not embed the original: %s", text)
	}

	if _, err := client.GetPrompt("draft_reply", nil); err == nil {
		t.Error("draft_reply without id accepted")
	}
	if _, err := client.GetPrompt("draft_reply", map[string]string{"id": ""}); err == nil {
		t.Error("draft_reply with empty id accepted")
	}
	if _, err := client.GetPrompt("draft_reply", map[string]string{"id": "ghost.eml"}); err == nil {
		t.Error("draft_reply for unknown id accepted")
	}
}

func TestPromptsTruncateOversizedMessages(t *testing.T) {
	client, root := newClient(t)
	big := "From: a@b.c\r\nSubject: Riesig\r\n\r\n" + strings.Repeat("x", maxEmbeddedMessageBytes+1024)
	drop(t, root, "big.eml", big)

	for _, tc := range []struct {
		name string
		args map[string]string
	}{
		{"summarize_inbox", nil},
		{"draft_reply", map[string]string{"id": "big.eml"}},
	} {
		text := promptText(t, client, tc.name, tc.args)
		if !strings.Contains(text, "[... truncated ...]") {
			t.Errorf("%s embeds an oversized message untruncated", tc.name)
		}
		if len(text) > 2*maxEmbeddedMessageBytes {
			t.Errorf("%s prompt is unbounded: %d bytes", tc.name, len(text))
		}
	}
}

func TestSummarizeInboxEdgeCases(t *testing.T) {
	// Empty inbox: the prompt says so instead of embedding nothing silently.
	client, _ := newClient(t)
	text := promptText(t, client, "summarize_inbox", nil)
	if !strings.Contains(text, "no unread messages") {
		t.Errorf("empty-inbox prompt = %s", text)
	}

	// A non-positive count is rejected like a non-numeric one.
	if _, err := client.GetPrompt("summarize_inbox", map[string]string{"count": "0"}); err == nil {
		t.Error("count=0 accepted")
	}

	// An unfetchable message is skipped, not fatal.
	svc := application.NewService(listOnlyBox{}, nil)
	client = testutil.NewTestClient(t, New(svc))
	text = promptText(t, client, "summarize_inbox", nil)
	if strings.Contains(text, "--- Message") {
		t.Errorf("unfetchable message embedded anyway: %s", text)
	}

	// A broken mailbox fails the prompt.
	if _, err := newBrokenClient(t).GetPrompt("summarize_inbox", nil); err == nil {
		t.Error("summarize_inbox over a broken mailbox did not error")
	}
}

func TestCompletions(t *testing.T) {
	mb, root := newDir(t)
	drop(t, root, "alpha.eml", "From: a@b\r\n\r\nx")
	drop(t, root, "beta.eml", "From: a@b\r\n\r\nx")
	srv := New(application.NewService(mb, nil))

	// Prompt-argument completion narrows unread ids by prefix.
	res, err := srv.HandleCompletion(t.Context(),
		server.CompletionRef{Type: server.CompletionRefPrompt, Name: "draft_reply"},
		server.CompletionArgument{Name: "id", Value: "al"})
	if err != nil {
		t.Fatalf("prompt completion: %v", err)
	}
	if len(res.Values) != 1 || res.Values[0] != "alpha.eml" || res.Total != 1 {
		t.Errorf("prompt completion = %+v", res)
	}

	// Resource completion offers the same ids for email://inbox/{id}.
	res, err = srv.HandleCompletion(t.Context(),
		server.CompletionRef{Type: server.CompletionRefResource, URI: "email://inbox/{id}"},
		server.CompletionArgument{Name: "id", Value: "b"})
	if err != nil {
		t.Fatalf("resource completion: %v", err)
	}
	if len(res.Values) != 1 || res.Values[0] != "beta.eml" {
		t.Errorf("resource completion = %+v", res)
	}

	// A broken mailbox fails both completions.
	broken := New(application.NewService(brokenBox{}, nil))
	if _, err := broken.HandleCompletion(t.Context(),
		server.CompletionRef{Type: server.CompletionRefPrompt, Name: "draft_reply"},
		server.CompletionArgument{Name: "id", Value: ""}); err == nil {
		t.Error("prompt completion over a broken mailbox did not error")
	}
	if _, err := broken.HandleCompletion(t.Context(),
		server.CompletionRef{Type: server.CompletionRefResource, URI: "email://inbox/{id}"},
		server.CompletionArgument{Name: "id", Value: ""}); err == nil {
		t.Error("resource completion over a broken mailbox did not error")
	}
}

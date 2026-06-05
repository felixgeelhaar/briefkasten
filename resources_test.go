package briefkasten

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/mcp-go/server"
	"github.com/felixgeelhaar/mcp-go/testutil"
)

// resourceServer builds a server over a maildir with one unread message
// and an outbox with one queued message.
func resourceServer(t *testing.T) (*testutil.TestClient, *Outbox) {
	t.Helper()
	mb, root := newDir(t)
	drop(t, root, "m1.eml", "From: a@b.c\r\nSubject: Quittung\r\n\r\nhallo")

	ob, err := NewOutbox(filepath.Join(t.TempDir(), "out"), &fakeSender{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ob.Enqueue(OutboundMessage{To: []string{"x@y.z"}, Subject: "s", Body: "b"}); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(mb)
	RegisterResources(srv, mb, ob)
	return testutil.NewTestClient(t, srv), ob
}

func TestResourcesListed(t *testing.T) {
	client, _ := resourceServer(t)
	resources, err := client.ListResources()
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	uris := map[string]bool{}
	for _, r := range resources {
		uris[r["uri"].(string)] = true
	}
	for _, want := range []string{"email://inbox", "email://outbox"} {
		if !uris[want] {
			t.Errorf("resource %q missing (have %v)", want, uris)
		}
	}
}

func TestInboxResource(t *testing.T) {
	client, _ := resourceServer(t)
	text, err := client.ReadResource("email://inbox")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	var payload struct {
		Unread []string `json:"unread"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("payload %q: %v", text, err)
	}
	if len(payload.Unread) != 1 || payload.Unread[0] != "m1.eml" {
		t.Errorf("unread = %v", payload.Unread)
	}
}

func TestInboxMessageResource(t *testing.T) {
	client, _ := resourceServer(t)
	text, err := client.ReadResource("email://inbox/m1.eml")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if !strings.Contains(text, "Subject: Quittung") {
		t.Errorf("message = %q", text)
	}
}

func TestOutboxResources(t *testing.T) {
	client, ob := resourceServer(t)

	text, err := client.ReadResource("email://outbox")
	if err != nil {
		t.Fatalf("ReadResource outbox: %v", err)
	}
	var summary map[string][]string
	if err := json.Unmarshal([]byte(text), &summary); err != nil {
		t.Fatalf("payload %q: %v", text, err)
	}
	if len(summary["queued"]) != 1 {
		t.Errorf("queued = %v", summary)
	}

	id := summary["queued"][0]
	text, err = client.ReadResource("email://outbox/" + id)
	if err != nil {
		t.Fatalf("ReadResource outbox msg: %v", err)
	}
	var msg OutboundMessage
	if err := json.Unmarshal([]byte(text), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.State != "queued" || msg.Subject != "s" {
		t.Errorf("msg = %+v", msg)
	}
	_ = ob
}

func TestInboxMessageResourceBadID(t *testing.T) {
	client, _ := resourceServer(t)
	if _, err := client.ReadResource("email://inbox/../../etc/passwd"); err == nil {
		t.Error("traversal id accepted")
	}
}

func TestResourceCompletionListsIDs(t *testing.T) {
	mb, root := newDir(t)
	drop(t, root, "alpha.eml", "From: a@b\r\n\r\nx")
	drop(t, root, "beta.eml", "From: a@b\r\n\r\nx")
	ob, _ := NewOutbox(filepath.Join(t.TempDir(), "out"), &fakeSender{})
	srv := NewServer(mb)
	RegisterResources(srv, mb, ob)

	res, err := srv.HandleCompletion(context.Background(),
		server.CompletionRef{Type: "ref/resource", URI: "email://inbox/{id}"},
		server.CompletionArgument{Name: "id", Value: "al"})
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	if len(res.Values) != 1 || res.Values[0] != "alpha.eml" {
		t.Errorf("completion = %+v", res)
	}
}

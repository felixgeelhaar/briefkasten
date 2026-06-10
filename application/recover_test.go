package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.klarlabs.de/briefkasten/application"
	"go.klarlabs.de/briefkasten/domain"
	"go.klarlabs.de/briefkasten/infrastructure/maildir"
)

func TestOutboxRecoverStrandedSending(t *testing.T) {
	store, err := maildir.NewOutboxStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash mid-delivery: the message sits in sending with no
	// process working on it.
	stuck := domain.OutboundMessage{
		ID:       "stuck-1",
		To:       []string{"a@b.c"},
		Subject:  "interrupted",
		Body:     "x",
		State:    "sending",
		Attempts: 1,
	}
	if err := store.Write(stuck); err != nil {
		t.Fatal(err)
	}

	ob := application.NewOutbox(store, &fakeSender{})
	if err := ob.Recover(); err != nil {
		t.Fatalf("Recover: %v", err)
	}

	msg, err := ob.Status("stuck-1")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if msg.State != "failed" {
		t.Errorf("state = %q, want failed", msg.State)
	}
	if !strings.Contains(msg.Error, "restart") {
		t.Errorf("error = %q, want mention of restart", msg.Error)
	}

	// Retry deliberately re-queues the message for another delivery.
	if err := ob.Retry("stuck-1"); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	msg, err = ob.Status("stuck-1")
	if err != nil {
		t.Fatal(err)
	}
	if msg.State != "queued" {
		t.Errorf("state after retry = %q, want queued", msg.State)
	}
}

func TestOutboxRecoverRepairsSendCrashDuplicate(t *testing.T) {
	store, err := maildir.NewOutboxStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash between apply's Write(sending) and Remove(queued)
	// during SEND: the same id exists in both states.
	msg := domain.OutboundMessage{ID: "dup-1", To: []string{"a@b.c"}, Subject: "s", Body: "b"}
	msg.State = "queued"
	if err := store.Write(msg); err != nil {
		t.Fatal(err)
	}
	msg.State = "sending"
	msg.Attempts = 1
	if err := store.Write(msg); err != nil {
		t.Fatal(err)
	}

	ob := application.NewOutbox(store, &fakeSender{})
	if err := ob.Recover(); err != nil {
		t.Fatalf("Recover: %v", err)
	}

	// The stale queued copy is gone; the sending copy was stranded and is
	// now failed — exactly one copy remains.
	copies := 0
	for _, state := range domain.OutboxStates {
		ids, err := store.List(state)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range ids {
			if id == "dup-1" {
				copies++
				if state != "failed" {
					t.Errorf("copy in %q, want failed", state)
				}
			}
		}
	}
	if copies != 1 {
		t.Errorf("copies = %d, want exactly 1", copies)
	}

	got, err := store.Find("dup-1")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.State != "failed" {
		t.Errorf("state = %q, want failed", got.State)
	}
	if got.Attempts != 1 {
		t.Errorf("attempts = %d, want 1 preserved from the sending copy", got.Attempts)
	}
}

func TestOutboxRecoverRepairsRetryCrashDuplicate(t *testing.T) {
	store, err := maildir.NewOutboxStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash between apply's Write(queued) and Remove(failed)
	// during RETRY.
	msg := domain.OutboundMessage{ID: "dup-2", To: []string{"a@b.c"}, Subject: "s", Body: "b", Attempts: 1}
	msg.State = "failed"
	msg.Error = "smtp down"
	if err := store.Write(msg); err != nil {
		t.Fatal(err)
	}
	msg.State = "queued"
	if err := store.Write(msg); err != nil {
		t.Fatal(err)
	}

	ob := application.NewOutbox(store, &fakeSender{})
	if err := ob.Recover(); err != nil {
		t.Fatalf("Recover: %v", err)
	}

	failed, err := store.List("failed")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range failed {
		if id == "dup-2" {
			t.Error("stale failed copy survived recover")
		}
	}
	queued, err := store.List("queued")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range queued {
		if id == "dup-2" {
			found = true
		}
	}
	if !found {
		t.Fatal("queued copy missing after recover")
	}

	got, err := store.Find("dup-2")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.State != "queued" {
		t.Errorf("state = %q, want queued", got.State)
	}
}

func TestOutboxRecoverNoopOnCleanStore(t *testing.T) {
	sender := &fakeSender{}
	ob := newDirOutbox(t, sender)
	id, err := ob.Enqueue(domain.OutboundMessage{To: []string{"a@b.c"}, Subject: "s", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ob.Recover(); err != nil {
		t.Fatalf("Recover on clean store: %v", err)
	}
	msg, err := ob.Status(id)
	if err != nil {
		t.Fatal(err)
	}
	if msg.State != "queued" {
		t.Errorf("state = %q, want queued untouched by recover", msg.State)
	}
}

func TestOutboxProcessOncePersistsAttempts(t *testing.T) {
	sender := &fakeSender{err: errors.New("smtp down")}
	dir := t.TempDir()
	ob := newDirOutboxAt(t, dir, sender)

	id, err := ob.Enqueue(domain.OutboundMessage{To: []string{"a@b.c"}, Subject: "s", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}

	// The attempt is counted before the send and persisted on FAIL.
	if _, err := ob.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	msg, err := ob.Status(id)
	if err != nil {
		t.Fatal(err)
	}
	if msg.State != "failed" {
		t.Errorf("state = %q, want failed", msg.State)
	}
	if msg.Attempts != 1 {
		t.Errorf("attempts after first failure = %d, want 1", msg.Attempts)
	}

	if err := ob.Retry(id); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if _, err := ob.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	msg, err = ob.Status(id)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Attempts != 2 {
		t.Errorf("attempts after second failure = %d, want 2", msg.Attempts)
	}

	// A fresh Outbox over the same dir sees the persisted count — the
	// number survives restarts, not just in-memory churn.
	ob2 := newDirOutboxAt(t, dir, &fakeSender{})
	msg, err = ob2.Status(id)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Attempts != 2 {
		t.Errorf("persisted attempts = %d, want 2", msg.Attempts)
	}
}

// removeFailingStore wraps a real store and fails Remove for one state,
// simulating a disk error while recording a delivery outcome.
type removeFailingStore struct {
	domain.OutboxStore
	failState string
	err       error
}

func (s *removeFailingStore) Remove(state, id string) error {
	if state == s.failState {
		return s.err
	}
	return s.OutboxStore.Remove(state, id)
}

func TestOutboxProcessOnceReportsOutcomePersistenceFailure(t *testing.T) {
	inner, err := maildir.NewOutboxStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	errDisk := errors.New("disk full")
	// Remove from sending fails — the SEND move (Remove from queued)
	// succeeds, so the failure hits exactly the outcome apply.
	store := &removeFailingStore{OutboxStore: inner, failState: "sending", err: errDisk}
	sender := &fakeSender{}
	ob := application.NewOutbox(store, sender)

	id, err := ob.Enqueue(domain.OutboundMessage{To: []string{"a@b.c"}, Subject: "s", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}

	delivered, err := ob.ProcessOnce(context.Background())
	if err == nil {
		t.Fatal("ProcessOnce swallowed the outcome persistence failure")
	}
	if !errors.Is(err, errDisk) {
		t.Errorf("err = %v, want wrapped %v", err, errDisk)
	}
	if !strings.Contains(err.Error(), id) {
		t.Errorf("err = %q, want mention of id %s", err, id)
	}
	if delivered != 0 {
		t.Errorf("delivered = %d, want 0 when the outcome could not be recorded", delivered)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent = %d messages, want 1 — the wire send did happen", len(sender.sent))
	}
}

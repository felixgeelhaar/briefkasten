package briefkasten

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/felixgeelhaar/fortify/ferrors"
)

// flakyMailbox fails listErr/fetchErr times before succeeding.
type flakyMailbox struct {
	mu        sync.Mutex
	failures  int
	listCalls int
	fetchErr  error
	slow      time.Duration
}

func (f *flakyMailbox) ListUnread() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	if f.slow > 0 {
		time.Sleep(f.slow)
	}
	if f.failures > 0 {
		f.failures--
		return nil, errors.New("connection reset")
	}
	return []string{"1"}, nil
}

func (f *flakyMailbox) Fetch(id string) ([]byte, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return []byte("From: a@b\r\n\r\nhi"), nil
}

func (f *flakyMailbox) MarkSeen(id string) error { return nil }

func (f *flakyMailbox) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls
}

func TestResilientRetriesTransientFailure(t *testing.T) {
	mb := &flakyMailbox{failures: 2}
	r := Resilient(mb, ResilienceConfig{InitialDelay: time.Millisecond})

	ids, err := r.ListUnread()
	if err != nil {
		t.Fatalf("ListUnread: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("ids = %v", ids)
	}
	if got := mb.calls(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestResilientDoesNotRetryBadID(t *testing.T) {
	mb := &flakyMailbox{fetchErr: fmt.Errorf("%w: nope", ErrBadID)}
	r := Resilient(mb, ResilienceConfig{InitialDelay: time.Millisecond})

	_, err := r.Fetch("nope")
	if !errors.Is(err, ErrBadID) {
		t.Fatalf("err = %v, want ErrBadID", err)
	}
}

func TestResilientCircuitOpensAfterConsecutiveFailures(t *testing.T) {
	mb := &flakyMailbox{failures: 1000}
	r := Resilient(mb, ResilienceConfig{InitialDelay: time.Millisecond, MaxAttempts: 1})

	// Default trip threshold is 5 consecutive failures.
	for i := 0; i < 5; i++ {
		if _, err := r.ListUnread(); err == nil {
			t.Fatal("want failure")
		}
	}
	_, err := r.ListUnread()
	if !errors.Is(err, ferrors.ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
	if got := mb.calls(); got != 5 {
		t.Errorf("backend calls = %d, want 5 (fast-fail once open)", got)
	}
}

func TestResilientTimesOutSlowBackend(t *testing.T) {
	mb := &flakyMailbox{slow: 200 * time.Millisecond}
	r := Resilient(mb, ResilienceConfig{
		OpTimeout:    20 * time.Millisecond,
		MaxAttempts:  1,
		InitialDelay: time.Millisecond,
	})

	_, err := r.ListUnread()
	if !errors.Is(err, ferrors.ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestBuildMailboxWrapsIMAPInResilience(t *testing.T) {
	cfg, _ := LoadConfig("")
	cfg.IMAP.Addr = "imap.example.org:993"
	mb, _, err := cfg.BuildMailbox()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := mb.(*ResilientMailbox); !ok {
		t.Errorf("imap backend = %T, want *ResilientMailbox", mb)
	}
}

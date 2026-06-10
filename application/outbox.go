package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"go.klarlabs.de/briefkasten/domain"
)

// Outbox drives outbound messages through the domain lifecycle over the
// store and sender ports. The application owns the orchestration; the
// domain owns which transitions are legal; infrastructure owns where the
// messages live and how they leave.
type Outbox struct {
	mu     sync.Mutex
	store  domain.OutboxStore
	sender domain.Sender
}

// NewOutbox binds the store and sender.
func NewOutbox(store domain.OutboxStore, sender domain.Sender) *Outbox {
	return &Outbox{store: store, sender: sender}
}

// Enqueue validates and persists a message in queued state, returning its id.
func (o *Outbox) Enqueue(msg domain.OutboundMessage) (string, error) {
	if err := msg.Validate(); err != nil {
		return "", err
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("outbox id: %w", err)
	}
	msg.ID = hex.EncodeToString(buf)
	msg.State = "queued"

	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.store.Write(msg); err != nil {
		return "", err
	}
	return msg.ID, nil
}

// Status returns the message with the given id, whatever its state.
func (o *Outbox) Status(id string) (domain.OutboundMessage, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.store.Find(id)
}

// Retry moves a failed message back to queued.
func (o *Outbox) Retry(id string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	msg, err := o.store.Find(id)
	if err != nil {
		return err
	}
	return o.apply(&msg, "RETRY")
}

// Summary returns the outbox ids grouped by lifecycle state.
func (o *Outbox) Summary() (map[string][]string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := map[string][]string{}
	for _, state := range domain.OutboxStates {
		ids, err := o.store.List(state)
		if err != nil {
			return nil, err
		}
		if ids == nil {
			ids = []string{}
		}
		out[state] = ids
	}
	return out, nil
}

// ProcessOnce delivers the queued backlog: each message transitions to
// sending, is handed to the Sender, and ends sent or failed. Returns how
// many messages were delivered. Persistence failures while recording an
// outcome are joined into the returned error — the message itself stays
// recoverable on disk either way.
func (o *Outbox) ProcessOnce(ctx context.Context) (int, error) {
	o.mu.Lock()
	ids, err := o.store.List("queued")
	o.mu.Unlock()
	if err != nil {
		return 0, fmt.Errorf("outbox list: %w", err)
	}

	delivered := 0
	var errs []error
	for _, id := range ids {
		o.mu.Lock()
		msg, err := o.store.Find(id)
		if err != nil {
			o.mu.Unlock()
			continue // raced away
		}
		// Count the attempt before the move to sending so the number
		// survives a crash mid-delivery.
		msg.Attempts++
		if err := o.apply(&msg, "SEND"); err != nil {
			o.mu.Unlock()
			continue
		}
		o.mu.Unlock()

		sendErr := o.sender.Send(ctx, msg)

		o.mu.Lock()
		if sendErr != nil {
			msg.Error = sendErr.Error()
			if err := o.apply(&msg, "FAIL"); err != nil {
				errs = append(errs, fmt.Errorf("outbox record failure of %s: %w", msg.ID, err))
			}
		} else {
			msg.Error = ""
			if err := o.apply(&msg, "SUCCEED"); err == nil {
				delivered++
			} else {
				errs = append(errs, fmt.Errorf("outbox record delivery of %s: %w", msg.ID, err))
			}
		}
		o.mu.Unlock()
	}
	return delivered, errors.Join(errs...)
}

// staleSide names, for every legal transition's (old, new) state pair, the
// side a crash between Write(new) and Remove(old) leaves behind as stale.
var staleSide = map[[2]string]string{
	{"queued", "sending"}: "queued",  // SEND
	{"sending", "sent"}:   "sending", // SUCCEED
	{"sending", "failed"}: "sending", // FAIL
	{"failed", "queued"}:  "failed",  // RETRY
}

// Recover repairs the store after an unclean shutdown:
//
//  1. A crash between apply's Write and Remove leaves the same id in two
//     states; the older copy is deleted (the transition table says which
//     side is older).
//  2. A message stranded in sending — the process died mid-delivery — is
//     moved to failed, since whether the wire delivery happened is
//     unknowable. Retry re-queues it deliberately rather than risking a
//     silent duplicate send.
//
// Call once at startup, before the delivery worker runs.
func (o *Outbox) Recover() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	inState := map[string][]string{}
	seen := map[string][]string{} // id -> states holding it
	for _, state := range domain.OutboxStates {
		ids, err := o.store.List(state)
		if err != nil {
			return fmt.Errorf("outbox recover: %w", err)
		}
		inState[state] = ids
		for _, id := range ids {
			seen[id] = append(seen[id], state)
		}
	}

	var errs []error
	stale := map[string]bool{} // state+"/"+id removed below
	for id, states := range seen {
		if len(states) < 2 {
			continue
		}
		for pair, old := range staleSide {
			if (pair[0] == states[0] && pair[1] == states[1]) || (pair[0] == states[1] && pair[1] == states[0]) {
				if err := o.store.Remove(old, id); err != nil {
					errs = append(errs, fmt.Errorf("outbox recover %s: %w", id, err))
				} else {
					stale[old+"/"+id] = true
				}
				break
			}
		}
	}

	for _, id := range inState["sending"] {
		if stale["sending/"+id] {
			continue
		}
		msg, err := o.store.Find(id)
		if err != nil {
			errs = append(errs, fmt.Errorf("outbox recover %s: %w", id, err))
			continue
		}
		if msg.State != "sending" {
			continue
		}
		msg.Error = "delivery interrupted by restart — outcome unknown; retry to re-send"
		if err := o.apply(&msg, "FAIL"); err != nil {
			errs = append(errs, fmt.Errorf("outbox recover %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// apply runs a lifecycle event through the domain statechart and persists
// the move. Caller holds the lock.
func (o *Outbox) apply(msg *domain.OutboundMessage, event string) error {
	next, err := domain.Transition(msg.State, event)
	if err != nil {
		return err
	}
	old := msg.State
	msg.State = next
	if err := o.store.Write(*msg); err != nil {
		return err
	}
	return o.store.Remove(old, msg.ID)
}

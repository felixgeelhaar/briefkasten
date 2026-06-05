package domain

import (
	"context"
	"errors"
)

// OutboundMessage is one message in the outbox.
type OutboundMessage struct {
	ID      string   `json:"id"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
	// State is the lifecycle state: queued, sending, sent, failed.
	State string `json:"state"`
	// Error holds the last delivery failure, when State is failed.
	Error string `json:"error,omitempty"`
	// Attempts counts delivery attempts.
	Attempts int `json:"attempts"`
}

// Validate enforces the message invariants.
func (m OutboundMessage) Validate() error {
	if len(m.To) == 0 {
		return errors.New("outbox: message needs at least one recipient")
	}
	return nil
}

// Sender delivers an outbound message — the outbound transport port.
type Sender interface {
	Send(ctx context.Context, msg OutboundMessage) error
}

// OutboxStore is the persistence port for the outbox: messages live in
// exactly one lifecycle state at a time.
type OutboxStore interface {
	// Write persists the message under its current state.
	Write(msg OutboundMessage) error
	// Remove deletes the message's record under the given state (the
	// message itself lives on under its new state after a move).
	Remove(state, id string) error
	// Find returns the message regardless of state.
	Find(id string) (OutboundMessage, error)
	// List returns the ids stored under one state.
	List(state string) ([]string, error)
}

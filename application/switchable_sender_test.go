package application_test

import (
	"context"
	"errors"
	"testing"

	"go.klarlabs.de/briefkasten/application"
	"go.klarlabs.de/briefkasten/domain"
)

// countingSender records how many messages it received.
type countingSender struct {
	n   int
	err error
}

func (c *countingSender) Send(_ context.Context, _ domain.OutboundMessage) error {
	if c.err != nil {
		return c.err
	}
	c.n++
	return nil
}

// TestSwitchableSenderSwap proves deliveries route to the current sender and a
// Swap repoints subsequent sends without disturbing the holder.
func TestSwitchableSenderSwap(t *testing.T) {
	a := &countingSender{}
	b := &countingSender{}
	sw := application.NewSwitchableSender(a)

	if err := sw.Send(context.Background(), domain.OutboundMessage{}); err != nil {
		t.Fatal(err)
	}
	sw.Swap(b)
	if err := sw.Send(context.Background(), domain.OutboundMessage{}); err != nil {
		t.Fatal(err)
	}
	if a.n != 1 || b.n != 1 {
		t.Errorf("routing wrong: a=%d b=%d, want 1/1", a.n, b.n)
	}

	// Errors from the current sender propagate.
	sw.Swap(&countingSender{err: errors.New("down")})
	if err := sw.Send(context.Background(), domain.OutboundMessage{}); err == nil {
		t.Error("expected the swapped sender's error to propagate")
	}
}

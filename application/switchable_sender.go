package application

import (
	"context"
	"sync"

	"go.klarlabs.de/briefkasten/domain"
)

// SwitchableSender is a Sender whose transport can be swapped at runtime
// (runtime reconfiguration), the outbound counterpart of Switchable. The outbox
// worker holds one instance for its lifetime; Swap atomically repoints it at a
// freshly-built sender, so a credentials or provider change takes effect on the
// next delivery without restarting the worker.
type SwitchableSender struct {
	mu     sync.RWMutex
	sender domain.Sender
}

// NewSwitchableSender wraps an initial sender.
func NewSwitchableSender(s domain.Sender) *SwitchableSender {
	return &SwitchableSender{sender: s}
}

// Swap replaces the sender for all subsequent deliveries.
func (s *SwitchableSender) Swap(next domain.Sender) {
	s.mu.Lock()
	s.sender = next
	s.mu.Unlock()
}

// Send delivers via the current sender.
func (s *SwitchableSender) Send(ctx context.Context, msg domain.OutboundMessage) error {
	s.mu.RLock()
	sender := s.sender
	s.mu.RUnlock()
	return sender.Send(ctx, msg)
}

var _ domain.Sender = (*SwitchableSender)(nil)

package briefkasten

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/felixgeelhaar/fortify/retry"
	"github.com/felixgeelhaar/fortify/timeout"
)

// SMTPConfig configures an SMTPSender.
type SMTPConfig struct {
	// Addr is the SMTP server address (host:port). Required.
	Addr string
	// From is the envelope and header sender. Required.
	From string
	// Username and Password authenticate via SASL PLAIN when set.
	Username string
	Password string
	// ImplicitTLS dials TLS directly (port 465 style). Default is
	// STARTTLS on a plain connection (port 587 style).
	ImplicitTLS bool
	// Insecure skips TLS entirely. For tests and local servers only.
	Insecure bool
	// TLSConfig optionally overrides the TLS client configuration.
	TLSConfig *tls.Config
	// OAuth2 switches authentication from SASL PLAIN to
	// XOAUTH2/OAUTHBEARER.
	OAuth2 *OAuth2Settings
}

// SMTPSender delivers outbound messages over SMTP (go-smtp), wrapped in
// fortify resilience: per-attempt timeout and exponential-backoff retry —
// transient SMTP failures (451) recover without reaching the outbox's
// failed state.
type SMTPSender struct {
	cfg SMTPConfig
	rt  retry.Retry[any]
	to  timeout.Timeout[any]
}

// NewSMTPSender validates the config and builds the sender.
func NewSMTPSender(cfg SMTPConfig) (*SMTPSender, error) {
	if cfg.Addr == "" {
		return nil, errors.New("smtp: Addr is required")
	}
	if cfg.From == "" {
		return nil, errors.New("smtp: From is required")
	}
	return &SMTPSender{
		cfg: cfg,
		rt: retry.New[any](retry.Config{
			MaxAttempts:  3,
			InitialDelay: 200 * time.Millisecond,
			Jitter:       true,
		}),
		to: timeout.New[any](timeout.Config{}),
	}, nil
}

// Send delivers the message, retrying transient failures.
func (s *SMTPSender) Send(ctx context.Context, msg OutboundMessage) error {
	_, err := s.rt.Execute(ctx, func(ctx context.Context) (any, error) {
		return s.to.Execute(ctx, 30*time.Second, func(context.Context) (any, error) {
			return nil, s.deliver(msg)
		})
	})
	return err
}

func (s *SMTPSender) deliver(msg OutboundMessage) error {
	c, err := s.dial()
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", s.cfg.Addr, err)
	}
	defer c.Close()

	if s.cfg.OAuth2 != nil {
		host, port := splitHostPort(s.cfg.Addr, 587)
		auth, err := s.cfg.OAuth2.saslClient(context.Background(), s.cfg.Username, host, port)
		if err != nil {
			return err
		}
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	} else if s.cfg.Username != "" {
		auth := sasl.NewPlainClient("", s.cfg.Username, s.cfg.Password)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	raw := buildRFC5322(s.cfg.From, msg, time.Now())
	if err := c.SendMail(s.cfg.From, msg.To, bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return c.Quit()
}

func (s *SMTPSender) dial() (*smtp.Client, error) {
	switch {
	case s.cfg.Insecure:
		return smtp.Dial(s.cfg.Addr)
	case s.cfg.ImplicitTLS:
		return smtp.DialTLS(s.cfg.Addr, s.cfg.TLSConfig)
	default:
		return smtp.DialStartTLS(s.cfg.Addr, s.cfg.TLSConfig)
	}
}

var _ Sender = (*SMTPSender)(nil)

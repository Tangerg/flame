package mcpserver

import (
	"errors"
	"time"
)

// HandshakeTimeout is the connection-handshake deadline policy for an MCP
// server. Its zero value is the explicit unbounded policy; a bounded policy can
// only be created through [NewHandshakeTimeout], so a raw duration never has to
// carry the additional meaning "disabled".
type HandshakeTimeout struct {
	mode     handshakeTimeoutMode
	duration time.Duration
}

type handshakeTimeoutMode uint8

const (
	handshakeTimeoutUnbounded handshakeTimeoutMode = iota
	handshakeTimeoutBounded
)

// NewHandshakeTimeout returns a bounded handshake policy.
func NewHandshakeTimeout(duration time.Duration) (HandshakeTimeout, error) {
	if duration < time.Second || duration%time.Second != 0 {
		return HandshakeTimeout{}, errors.New("mcpserver: bounded handshake timeout must be a positive whole number of seconds")
	}
	return HandshakeTimeout{mode: handshakeTimeoutBounded, duration: duration}, nil
}

// IsBounded reports whether the handshake has its own deadline in addition to
// the caller's context.
func (h HandshakeTimeout) IsBounded() bool {
	return h.mode == handshakeTimeoutBounded
}

// Duration returns the configured deadline and whether the policy is bounded.
func (h HandshakeTimeout) Duration() (time.Duration, bool) {
	if !h.IsBounded() {
		return 0, false
	}
	return h.duration, true
}

// Validate rejects forged or corrupted representations.
func (h HandshakeTimeout) Validate() error {
	switch h.mode {
	case handshakeTimeoutUnbounded:
		if h.duration != 0 {
			return errors.New("mcpserver: unbounded handshake timeout carries a duration")
		}
	case handshakeTimeoutBounded:
		if h.duration < time.Second || h.duration%time.Second != 0 {
			return errors.New("mcpserver: bounded handshake timeout must be a positive whole number of seconds")
		}
	default:
		return errors.New("mcpserver: unknown handshake timeout mode")
	}
	return nil
}

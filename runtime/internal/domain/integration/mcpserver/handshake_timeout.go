package mcpserver

import (
	"errors"
	"time"
)

type HandshakeTimeout struct {
	mode     handshakeTimeoutMode
	duration time.Duration
}

type handshakeTimeoutMode uint8

const (
	handshakeTimeoutUnbounded handshakeTimeoutMode = iota
	handshakeTimeoutBounded
)

func NewHandshakeTimeout(duration time.Duration) (HandshakeTimeout, error) {
	if duration < time.Second || duration%time.Second != 0 {
		return HandshakeTimeout{}, errors.New("mcpserver: bounded handshake timeout must be a positive whole number of seconds")
	}
	return HandshakeTimeout{mode: handshakeTimeoutBounded, duration: duration}, nil
}

func (h HandshakeTimeout) IsBounded() bool {
	return h.mode == handshakeTimeoutBounded
}

func (h HandshakeTimeout) Duration() (time.Duration, bool) {
	if !h.IsBounded() {
		return 0, false
	}
	return h.duration, true
}

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

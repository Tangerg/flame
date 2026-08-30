package commandreplay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type GuardKind string

const (
	GuardUnprotected GuardKind = "unprotected"
	GuardProtected   GuardKind = "protected"
)

// Guard binds one durable command identity to either an explicit lack of a
// replay promise or one exact Runtime store and deadline. Its zero value is
// invalid so queue-local state cannot be inferred from empty strings/times.
type Guard struct {
	kind      GuardKind
	namespace string
	until     time.Time
}

func UnprotectedGuard() Guard { return Guard{kind: GuardUnprotected} }

func NewProtectedGuard(namespace string, until time.Time) (Guard, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return Guard{}, errors.New("protected command replay guard namespace is empty")
	}
	if until.IsZero() {
		return Guard{}, errors.New("protected command replay guard deadline is empty")
	}
	return Guard{kind: GuardProtected, namespace: namespace, until: until.UTC()}, nil
}

func (g Guard) Validate() error {
	switch g.kind {
	case GuardUnprotected:
		if g.namespace != "" || !g.until.IsZero() {
			return errors.New("unprotected command replay guard carries protected state")
		}
		return nil
	case GuardProtected:
		if strings.TrimSpace(g.namespace) == "" || g.namespace != strings.TrimSpace(g.namespace) || g.until.IsZero() {
			return errors.New("protected command replay guard is incomplete")
		}
		if g.until.Location() != time.UTC {
			return errors.New("protected command replay guard deadline is not UTC")
		}
		return nil
	default:
		return fmt.Errorf("command replay guard has unknown kind %q", g.kind)
	}
}

func (g Guard) Kind() GuardKind { return g.kind }

func (g Guard) Protected() bool { return g.kind == GuardProtected }

func (g Guard) Namespace() string { return g.namespace }

func (g Guard) Until() time.Time { return g.until }

type guardJSON struct {
	Type      GuardKind  `json:"type"`
	Namespace string     `json:"namespace,omitempty"`
	Until     *time.Time `json:"until,omitempty"`
}

func (g Guard) MarshalJSON() ([]byte, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}
	wire := guardJSON{Type: g.kind}
	if g.kind == GuardProtected {
		wire.Namespace = g.namespace
		until := g.until
		wire.Until = &until
	}
	return json.Marshal(wire)
}

func (g *Guard) UnmarshalJSON(data []byte) error {
	if g == nil {
		return errors.New("decode command replay guard into nil receiver")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire guardJSON
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("decode command replay guard: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode command replay guard: trailing JSON value")
		}
		return fmt.Errorf("decode command replay guard trailing data: %w", err)
	}
	switch wire.Type {
	case GuardUnprotected:
		if wire.Namespace != "" || wire.Until != nil {
			return errors.New("unprotected command replay guard carries protected state")
		}
		*g = UnprotectedGuard()
		return nil
	case GuardProtected:
		if wire.Until == nil {
			return errors.New("protected command replay guard deadline is empty")
		}
		protected, err := NewProtectedGuard(wire.Namespace, *wire.Until)
		if err != nil {
			return err
		}
		*g = protected
		return nil
	default:
		return fmt.Errorf("command replay guard has unknown type %q", wire.Type)
	}
}

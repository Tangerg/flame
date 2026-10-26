package authoring

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"time"
)

type replayGuardKind string

const (
	guardUnprotected replayGuardKind = "unprotected"
	guardProtected   replayGuardKind = "protected"
)

// Guard binds one durable command identity to either an explicit lack of a
// replay promise or one exact Runtime store and deadline. Its zero value is
// invalid so queue-local state cannot be inferred from empty strings/times.
type ReplayGuard struct {
	kind      replayGuardKind
	namespace string
	until     time.Time
}

func UnprotectedGuard() ReplayGuard { return ReplayGuard{kind: guardUnprotected} }

func NewProtectedGuard(namespace string, until time.Time) (ReplayGuard, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return ReplayGuard{}, errors.New("protected command replay guard namespace is empty")
	}
	if until.IsZero() {
		return ReplayGuard{}, errors.New("protected command replay guard deadline is empty")
	}
	return ReplayGuard{kind: guardProtected, namespace: namespace, until: until.UTC()}, nil
}

func (g ReplayGuard) Validate() error {
	switch g.kind {
	case guardUnprotected:
		if g.namespace != "" || !g.until.IsZero() {
			return errors.New("unprotected command replay guard carries protected state")
		}
		return nil
	case guardProtected:
		if strings.TrimSpace(g.namespace) == "" || g.namespace != strings.TrimSpace(g.namespace) || g.until.IsZero() {
			return errors.New("protected command replay guard is incomplete")
		}
		return nil
	default:
		return fmt.Errorf("command replay guard has unknown kind %q", g.kind)
	}
}

func (g ReplayGuard) Protected() bool { return g.kind == guardProtected }

func (g ReplayGuard) Namespace() string { return g.namespace }

func (g ReplayGuard) Until() time.Time { return g.until }

type guardJSON struct {
	Type      replayGuardKind `json:"type"`
	Namespace string          `json:"namespace,omitzero"`
	Until     *time.Time      `json:"until,omitzero"`
}

func (g ReplayGuard) MarshalJSON() ([]byte, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}
	wire := guardJSON{Type: g.kind}
	if g.kind == guardProtected {
		wire.Namespace = g.namespace
		until := g.until
		wire.Until = &until
	}
	return json.Marshal(wire)
}

func (g *ReplayGuard) UnmarshalJSON(data []byte) error {
	var wire guardJSON
	if err := json.Unmarshal(data, &wire, json.RejectUnknownMembers(true)); err != nil {
		return fmt.Errorf("decode command replay guard: %w", err)
	}
	switch wire.Type {
	case guardUnprotected:
		if wire.Namespace != "" || wire.Until != nil {
			return errors.New("unprotected command replay guard carries protected state")
		}
		*g = UnprotectedGuard()
		return nil
	case guardProtected:
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

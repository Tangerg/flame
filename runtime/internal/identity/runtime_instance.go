// Package runtimeinstanceidentity owns the exact process-incarnation identity
// advertised by Flame's discovery and operational sidecars. It is deliberately
// separate from the durable SQLite idempotency namespace: restarting the same
// database must retain the latter and replace this value.
package runtimeinstanceidentity

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	// Prefix distinguishes an ephemeral Runtime process from durable resource
	// identities and storage namespaces.
	Prefix        = "runtime_"
	uuidTextBytes = 36
)

// ID is one exact Runtime process incarnation.
type ID struct{ text string }

// New mints a fresh process incarnation.
func New() ID {
	id, err := Parse(Prefix + uuid.NewString())
	if err != nil {
		panic(fmt.Sprintf("runtime instance identity: generated invalid identity: %v", err))
	}
	return id
}

// Parse accepts only the canonical lowercase UUID spelling emitted by New.
// Callers must never trim, case-fold, or otherwise repair identity material.
func Parse(text string) (ID, error) {
	if len(text) != len(Prefix)+uuidTextBytes || !strings.HasPrefix(text, Prefix) {
		return ID{}, errors.New("runtime instance identity must use the canonical runtime UUID form")
	}
	uuidText := text[len(Prefix):]
	parsed, err := uuid.Parse(uuidText)
	if err != nil || parsed.String() != uuidText {
		return ID{}, errors.New("runtime instance identity must use the canonical runtime UUID form")
	}
	return ID{text: text}, nil
}

func (i ID) String() string { return i.text }

// Validate reports whether this value is a constructed Runtime identity.
func (i ID) Validate() error {
	_, err := Parse(i.text)
	return err
}

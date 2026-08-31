package identity

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	// Prefix distinguishes an ephemeral Runtime process from durable resource
	// identities and storage namespaces.
	RuntimeInstancePrefix = "runtime_"
	uuidTextBytes         = 36
)

// RuntimeInstanceID is one exact Runtime process incarnation.
type RuntimeInstanceID struct{ text string }

// NewRuntimeInstance mints a fresh process incarnation.
func NewRuntimeInstance() RuntimeInstanceID {
	id, err := ParseRuntimeInstance(RuntimeInstancePrefix + uuid.NewString())
	if err != nil {
		panic(fmt.Sprintf("runtime instance identity: generated invalid identity: %v", err))
	}
	return id
}

// ParseRuntimeInstance accepts only the canonical lowercase UUID spelling emitted by NewRuntimeInstance.
// Callers must never trim, case-fold, or otherwise repair identity material.
func ParseRuntimeInstance(text string) (RuntimeInstanceID, error) {
	if len(text) != len(RuntimeInstancePrefix)+uuidTextBytes || !strings.HasPrefix(text, RuntimeInstancePrefix) {
		return RuntimeInstanceID{}, errors.New("runtime instance identity must use the canonical runtime UUID form")
	}
	uuidText := text[len(RuntimeInstancePrefix):]
	parsed, err := uuid.Parse(uuidText)
	if err != nil || parsed.String() != uuidText {
		return RuntimeInstanceID{}, errors.New("runtime instance identity must use the canonical runtime UUID form")
	}
	return RuntimeInstanceID{text: text}, nil
}

func (i RuntimeInstanceID) String() string { return i.text }

// Validate reports whether this value is a constructed Runtime identity.
func (i RuntimeInstanceID) Validate() error {
	_, err := ParseRuntimeInstance(i.text)
	return err
}

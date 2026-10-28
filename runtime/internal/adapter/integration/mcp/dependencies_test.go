package mcp

import (
	"errors"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"testing"
)

// A transport and even a cached dependency projection cannot stand in for a
// Qimu Use. Only the original Use may supply a durable activation snapshot.
func TestActivationRejectsSessionWithoutOriginalFrameworkUse(t *testing.T) {
	session := &sdk.ClientSession{}
	owner := &connections{lifetime: t.Context(), sessions: map[*sdk.ClientSession]*ownedSession{
		session: {bindings: &plugin.RequiredBindings{}},
	}}
	if _, err := owner.activation(session); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unapproved session supplied a durable binding: %v", err)
	}
}

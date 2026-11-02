package keybinding

import (
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
)

func TestConflictsUseParsedSequenceIdentity(t *testing.T) {
	bindings, err := Parse(map[settings.Action][]string{
		settings.ActionQuit:     {"g   g"},
		settings.ActionSessions: {"g g"},
	})
	if err == nil || len(bindings) != 0 {
		t.Fatalf("conflicting sequences admitted: %v, %v", bindings, err)
	}
}

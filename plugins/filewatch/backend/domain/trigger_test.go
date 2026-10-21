package filewatch

import (
	"errors"
	"testing"
)

func TestTriggerProtectsPathAndSelectionOwnership(t *testing.T) {
	draft := Draft{Title: "Inspect", Instructions: "Inspect the observed source.", Workspace: "/workspace", Paths: []string{"input.txt", "src"}, Provider: "openai", Model: "private-model"}
	trigger, err := NewTrigger(draft)
	if err != nil {
		t.Fatal(err)
	}
	draft.Paths[0] = "../escape"
	projection := trigger.Draft()
	projection.Paths[1] = "changed"
	if trigger.Draft().Paths[0] != "input.txt" || trigger.Draft().Paths[1] != "src" {
		t.Fatal("caller changed the admitted trigger")
	}
	for _, path := range []string{"../escape", "/absolute", "src/../input.txt"} {
		invalid := trigger.Draft()
		invalid.Paths = []string{path}
		if _, err := NewTrigger(invalid); !errors.Is(err, ErrInvalid) {
			t.Fatalf("admitted %q: %v", path, err)
		}
	}
	invalid := trigger.Draft()
	invalid.Model = ""
	if _, err := NewTrigger(invalid); !errors.Is(err, ErrInvalid) {
		t.Fatal("admitted an uncaptured model default")
	}
	configuration, err := Initial().Replace(1, new(trigger.Draft()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configuration.Replace(1, nil); !errors.Is(err, ErrRevision) {
		t.Fatal("stale configuration replaced current ownership")
	}
	disabled, err := configuration.Replace(2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, enabled := disabled.Trigger(); enabled {
		t.Fatal("disabled configuration retained an active trigger")
	}
}

func TestCursorRequiresMonotonicConnectionLocalIdentity(t *testing.T) {
	cursor, err := NewCursor("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if gap, err := cursor.Advance(1); err != nil || gap {
		t.Fatalf("initial event: gap=%v %v", gap, err)
	}
	if _, err := cursor.Advance(1); err == nil {
		t.Fatal("duplicate sequence was admitted")
	}
	if gap, err := cursor.Advance(4); err != nil || !gap {
		t.Fatalf("missing observations became continuous coverage: gap=%v %v", gap, err)
	}
}

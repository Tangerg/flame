package transcript

import (
	"errors"
	"testing"
	"time"
)

func TestReplacementRequiresOneValidItemIdentity(t *testing.T) {
	at := time.Unix(1, 0).UTC()
	expected, err := NewUserMessage(ItemIdentity{
		SessionID: "ses_1", RunID: "run_1", ItemID: "item_1", OccurredAt: at,
	}, []ContentBlock{{Kind: TextContent, Text: "before"}})
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewUserMessage(ItemIdentity{
		SessionID: "ses_1", RunID: "run_1", ItemID: "item_1", OccurredAt: at,
	}, []ContentBlock{{Kind: TextContent, Text: "after"}})
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := NewReplacement(expected, state)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Expected().ID() != expected.ID() || replacement.State().ID() != state.ID() {
		t.Fatalf("replacement = %+v", replacement)
	}

	foreign, err := NewUserMessage(ItemIdentity{
		SessionID: "ses_1", RunID: "run_2", ItemID: "item_1", OccurredAt: at,
	}, []ContentBlock{{Kind: TextContent, Text: "after"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewReplacement(expected, foreign); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("identity error = %v, want ErrIdentityConflict", err)
	}
	if _, err := NewReplacement(Item{}, state); err == nil {
		t.Fatal("NewReplacement accepted an invalid expected Item")
	}
}

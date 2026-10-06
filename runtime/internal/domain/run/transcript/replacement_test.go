package transcript

import (
	"errors"
	"testing"
	"time"
)

func TestReplaceRequiresOneValidItemIdentity(t *testing.T) {
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
	replacement, err := Replace(expected, func(Item) (Item, error) { return state, nil })
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
	if _, err := Replace(expected, func(Item) (Item, error) { return foreign, nil }); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("identity error = %v, want ErrIdentityConflict", err)
	}
	if _, err := Replace(Item{}, func(Item) (Item, error) { return state, nil }); err == nil {
		t.Fatal("Replace accepted an invalid expected Item")
	}
}

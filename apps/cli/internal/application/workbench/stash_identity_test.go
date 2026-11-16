package workbench

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
)

func TestStashIdentityCollisionPreservesOriginalAuthoringState(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, moveDraft := range []bool{false, true} {
			for _, capacity := range []int{1, 2} {
				t.Run(fmt.Sprintf("durable=%t/move_draft=%t/capacity=%d", durable, moveDraft, capacity), func(t *testing.T) {
					directory := t.TempDir()
					config := Config{
						StashCapacity: testCapacity(t, capacity),
						Random:        bytes.NewBufferString("1234567812345678abcdefgh"),
					}
					var store *Store
					var err error
					if durable {
						store, err = OpenDirectory(t, directory, config)
					} else {
						store, err = OpenMemory(config)
					}
					if err != nil {
						t.Fatal(err)
					}
					closeStore := store.Close
					t.Cleanup(func() { _ = closeStore() })
					first, err := store.StashPrompt(authoring.Message{Text: "original named prompt"})
					if err != nil {
						t.Fatal(err)
					}
					const sessionID = "session"
					draft := authoring.Message{Text: "retain the source draft"}
					if err := store.SaveDraft(sessionID, draft); err != nil {
						t.Fatal(err)
					}
					if moveDraft {
						_, err = store.StashDraft(sessionID, draft)
					} else {
						_, err = store.StashPrompt(authoring.Message{Text: "another named prompt"})
					}
					if err == nil {
						t.Error("a new prompt reused an occupied stash identity")
					}
					assertOriginal := func() {
						t.Helper()
						stashes := store.Stashes()
						if len(stashes) != 1 || stashes[0].ID != first.ID ||
							!stashes[0].Message.Equal(first.Message) || !stashes[0].CreatedAt.Equal(first.CreatedAt) {
							t.Errorf("collision changed the original stash: %+v", stashes)
						}
						if current, present := store.Draft(sessionID); !present || !current.Equal(draft) {
							t.Errorf("collision changed the source draft: %+v, present=%t", current, present)
						}
					}
					assertOriginal()
					if durable {
						if err := store.Close(); err != nil {
							t.Fatal(err)
						}
						store, err = OpenDirectory(t, directory, config)
						if err != nil {
							t.Fatalf("collision corrupted persisted authoring state: %v", err)
						}
						assertOriginal()
					}
					next, err := store.StashPrompt(authoring.Message{Text: "accepted after the refusal"})
					if err != nil {
						t.Fatalf("collision prevented later authoring: %v", err)
					}
					if next.ID == first.ID {
						t.Fatal("the next prompt reused the original identity")
					}
				})
			}
		}
	}
}

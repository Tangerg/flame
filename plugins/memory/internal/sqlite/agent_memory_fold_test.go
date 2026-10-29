package sqlite

import (
	"testing"
	"time"

	agentmemory "github.com/Tangerg/flame/plugins/memory/internal/domain"
)

func TestFoldPublicationPreservesProtectedRecords(t *testing.T) {
	store := newAgentMemoryStore(t)
	ctx := t.Context()
	now := time.Date(2026, 7, 19, 4, 0, 0, 0, time.UTC)
	facts := appendAgentFacts(t, store, "/repo", "2026-07-19", "pending", "pinned", "approved", "rejected")
	first := agentMemoryPublication(t, "/repo", agentmemory.State{}, facts[len(facts)-1].Sequence,
		[]string{"pending", "pinned", "approved", "rejected"}, now)
	if published, err := store.Reconcile(ctx, first); err != nil || !published {
		t.Fatalf("first fold: %t, %v", published, err)
	}
	items, err := store.List(ctx, agentmemory.ScopeProject, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	before := make(map[string]agentmemory.Item)
	for _, item := range items {
		switch item.Content() {
		case "pinned":
			item, err = store.Update(ctx, item.ID(), item.Revision(), nil, new(true), now)
		case "approved", "rejected":
			decision := agentmemory.ReviewApprove
			if item.Content() == "rejected" {
				decision = agentmemory.ReviewReject
			}
			err = store.Review(ctx, item.ID(), item.Revision(), decision, now)
			if err == nil {
				item, _, err = store.Get(ctx, item.ID())
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		before[item.Content()] = item
	}
	user, added, err := store.Add(ctx, agentmemory.ScopeProject, "/repo", "authored", now)
	if err != nil || !added {
		t.Fatalf("user fact: %t, %v", added, err)
	}
	before[user.Content()] = user
	state, err := store.State(ctx, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	nextFacts := appendAgentFacts(t, store, "/repo", "2026-07-20", "new")
	next := agentMemoryPublication(t, "/repo", state, nextFacts[0].Sequence,
		[]string{"new", "authored", "rejected"}, now.Add(time.Hour))
	if published, err := store.Reconcile(ctx, next); err != nil || !published {
		t.Fatalf("next fold: %t, %v", published, err)
	}
	for content, original := range before {
		item, found, err := store.Get(ctx, original.ID())
		if err != nil {
			t.Fatal(err)
		}
		if content == "pending" {
			if found {
				t.Fatal("obsolete unpinned automatic proposal survived")
			}
			continue
		}
		if !found || item.Content() != original.Content() || item.Revision() != original.Revision() || item.Status() != original.Status() || item.Pinned() != original.Pinned() {
			t.Fatalf("protected %s changed: %+v", content, item.Snapshot())
		}
	}
	visible, err := store.List(ctx, agentmemory.ScopeProject, "/repo")
	if err != nil || len(visible) != 4 {
		t.Fatalf("visible fold = %d, %v", len(visible), err)
	}
}

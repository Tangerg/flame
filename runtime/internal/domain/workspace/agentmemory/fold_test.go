package agentmemory

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestFoldProposesNewAndRespectsStatuses(t *testing.T) {
	existing := []Item{
		{ID: testItemID(t, 'a'), Content: "- a", Status: StatusPending},
		{ID: testItemID(t, 'b'), Content: "- b", Status: StatusActive},
		{ID: testItemID(t, 'c'), Content: "- c", Status: StatusRejected},
	}
	// The curator re-emits a/b/c and adds d (twice, plus a blank). Only the
	// genuinely new fact d becomes a proposal: a/b/c are already present in some
	// status, and a rejected tombstone (c) blocks re-proposal.
	plan, err := Fold(existing, []string{"- a", "- b", "- c", "- d", "- d", "  "})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(plan.InsertContents, []string{"- d"}) {
		t.Fatalf("InsertContents = %v, want [- d]", plan.InsertContents)
	}
	if len(plan.PruneIDs) != 0 {
		t.Fatalf("PruneIDs = %v, want none (nothing dropped)", plan.PruneIDs)
	}
}

func TestFoldPrunesStalePendingButKeepsActiveAndRejected(t *testing.T) {
	existing := []Item{
		{ID: testItemID(t, 'a'), Content: "- a", Status: StatusPending},
		{ID: testItemID(t, 'b'), Content: "- b", Status: StatusActive},
		{ID: testItemID(t, 'c'), Content: "- c", Status: StatusRejected},
	}
	// The curator drops a, b, and c. Only the pending proposal a is pruned:
	// active b is sticky (the user accepted it), rejected c stays a tombstone.
	plan, err := Fold(existing, []string{"- e"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(plan.PruneIDs, []ItemID{testItemID(t, 'a')}) {
		t.Fatalf("PruneIDs = %v, want [ia]", plan.PruneIDs)
	}
	if !slices.Equal(plan.InsertContents, []string{"- e"}) {
		t.Fatalf("InsertContents = %v, want [- e]", plan.InsertContents)
	}
}

func TestFoldEmpty(t *testing.T) {
	if plan, err := Fold(nil, nil); err != nil || len(plan.InsertContents) != 0 || len(plan.PruneIDs) != 0 {
		t.Fatalf("empty fold = %+v", plan)
	}
}

func TestFoldRejectsUnboundedOrInvalidCurationOutput(t *testing.T) {
	contents := make([]string, MaxCurationProposals+1)
	for index := range contents {
		contents[index] = fmt.Sprintf("fact %d", index)
	}
	if _, err := Fold(nil, contents); err == nil {
		t.Fatal("oversized curation result was accepted")
	}
	if _, err := Fold(nil, []string{strings.Repeat("界", MaxContentCharacters+1)}); err == nil {
		t.Fatal("invalid curation content was accepted")
	}
}

func TestFoldRejectsInvalidExistingProjection(t *testing.T) {
	tests := []struct {
		name string
		item Item
	}{
		{name: "identity", item: Item{Content: "fact", Status: StatusPending}},
		{name: "content", item: Item{ID: testItemID(t, '1'), Content: " fact ", Status: StatusPending}},
		{name: "status", item: Item{ID: testItemID(t, '1'), Content: "fact", Status: Status("unknown")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Fold([]Item{test.item}, nil); err == nil {
				t.Fatal("invalid existing fold projection was accepted")
			}
		})
	}
}

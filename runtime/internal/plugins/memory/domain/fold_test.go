package agentmemory

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestFoldProposesNewAndRespectsStatuses(t *testing.T) {
	existing := []Item{
		{id: testItemID(t, 'a'), content: "- a", status: StatusPending},
		{id: testItemID(t, 'b'), content: "- b", status: StatusActive},
		{id: testItemID(t, 'c'), content: "- c", status: StatusRejected},
	}

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
		{id: testItemID(t, 'a'), content: "- a", status: StatusPending},
		{id: testItemID(t, 'b'), content: "- b", status: StatusActive},
		{id: testItemID(t, 'c'), content: "- c", status: StatusRejected},
	}

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

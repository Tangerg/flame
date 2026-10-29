package agentmemory

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFoldProposesNewAndRespectsStatuses(t *testing.T) {
	existing := []Item{
		{id: testItemID(t, 'a'), content: "- a", origin: OriginAuto, status: StatusPending},
		{id: testItemID(t, 'b'), content: "- b", origin: OriginAuto, status: StatusActive},
		{id: testItemID(t, 'c'), content: "- c", origin: OriginAuto, status: StatusRejected},
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
		{id: testItemID(t, 'a'), content: "- a", origin: OriginAuto, status: StatusPending},
		{id: testItemID(t, 'b'), content: "- b", origin: OriginAuto, status: StatusActive},
		{id: testItemID(t, 'c'), content: "- c", origin: OriginAuto, status: StatusRejected},
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

func TestFoldOwnsCompletePruningEligibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		origin Origin
		status Status
		pinned bool
		prune  bool
	}{
		{"proposal", OriginAuto, StatusPending, false, true},
		{"pinned proposal", OriginAuto, StatusPending, true, false},
		{"approved", OriginAuto, StatusActive, false, false},
		{"rejected", OriginAuto, StatusRejected, false, false},
		{"authored", OriginUser, StatusActive, false, false},
		{"pinned authored", OriginUser, StatusActive, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proposal, err := NewProposal(testItemID(t, 'a'), "/repo", "retained fact", time.Unix(1, 0))
			if err != nil {
				t.Fatal(err)
			}
			snapshot := proposal.Snapshot()
			snapshot.Origin, snapshot.Status, snapshot.Pinned = tc.origin, tc.status, tc.pinned
			item, err := RestoreItem(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := Fold([]Item{item}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if pruned := slices.Contains(plan.PruneIDs, item.ID()); pruned != tc.prune {
				t.Fatalf("pruned=%t, want %t", pruned, tc.prune)
			}
		})
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

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
		foldTestItem(t, 'a', "- a", StatusPending),
		foldTestItem(t, 'b', "- b", StatusActive),
		foldTestItem(t, 'c', "- c", StatusRejected),
	}

	plan, err := foldPublication(t, existing, []string{"- a", "- b", "- c", "- d", "- d", "  "}).Fold(existing)
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
		foldTestItem(t, 'a', "- a", StatusPending),
		foldTestItem(t, 'b', "- b", StatusActive),
		foldTestItem(t, 'c', "- c", StatusRejected),
	}

	plan, err := foldPublication(t, existing, []string{"- e"}).Fold(existing)
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
	if plan, err := foldPublication(t, nil, nil).Fold(nil); err != nil || len(plan.InsertContents) != 0 || len(plan.PruneIDs) != 0 {
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
			plan, err := foldPublication(t, []Item{item}, nil).Fold([]Item{item})
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
	if _, err := NewPublication("/repo", State{}, 1, nil, contents, time.Unix(1, 0)); err == nil {
		t.Fatal("oversized curation result was accepted")
	}
	if _, err := NewPublication("/repo", State{}, 1, nil, []string{strings.Repeat("界", MaxContentCharacters+1)}, time.Unix(1, 0)); err == nil {
		t.Fatal("invalid curation content was accepted")
	}
}

func foldTestItem(t *testing.T, seed byte, content string, status Status) Item {
	t.Helper()
	item, err := NewProposal(testItemID(t, seed), "/repo", content, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusPending {
		decision := ReviewApprove
		if status == StatusRejected {
			decision = ReviewReject
		}
		item, err = item.Review(item.Revision(), decision, time.Unix(1, 0))
		if err != nil {
			t.Fatal(err)
		}
	}
	return item
}

func foldPublication(t *testing.T, observed []Item, contents []string) Publication {
	t.Helper()
	publication, err := NewPublication("/repo", State{}, 1, observed, contents, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	return publication
}

func TestPublicationPrunesOnlyUnchangedObservedProposals(t *testing.T) {
	first := foldTestItem(t, 'a', "observed", StatusPending)
	omitted := foldTestItem(t, 'b', "omitted", StatusPending)
	observed := []Item{first}
	publication := foldPublication(t, observed, nil)
	observed[0] = omitted
	plan, err := publication.Fold([]Item{first, omitted})
	if err != nil || !slices.Equal(plan.PruneIDs, []ItemID{first.ID()}) {
		t.Fatalf("observed fold = %+v, %v", plan, err)
	}
	pinned, err := first.Edit(first.Revision(), nil, new(true), time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	current := pinned.Item()
	unpinned, err := current.Edit(current.Revision(), nil, new(false), time.Unix(3, 0))
	if err != nil {
		t.Fatal(err)
	}
	plan, err = publication.Fold([]Item{unpinned.Item(), omitted})
	if err != nil || len(plan.PruneIDs) != 0 {
		t.Fatalf("revised fold = %+v, %v", plan, err)
	}
}

func TestPublicationBindsObservedAndCurrentItemsToItsProject(t *testing.T) {
	other, err := NewProposal(testItemID(t, 'a'), "/other", "foreign", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewPublication("/repo", State{}, 1, []Item{other}, nil, time.Unix(2, 0)); err == nil {
		t.Fatal("foreign basis accepted")
	}
	if _, err := foldPublication(t, nil, nil).Fold([]Item{other}); err == nil {
		t.Fatal("foreign current item accepted")
	}
	if _, err := (Publication{}).Fold(nil); err == nil {
		t.Fatal("zero publication accepted")
	}
}

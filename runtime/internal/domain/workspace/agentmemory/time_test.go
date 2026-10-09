package agentmemory

import (
	"math"
	"testing"
	"time"
)

func TestMemoryTimesRemainExactlyRepresentable(t *testing.T) {
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).UTC(), time.Unix(0, 0).UTC(), time.Unix(0, math.MaxInt64).UTC()} {
		if _, err := NewUserItem(testItemID(t, 'a'), ScopeUser, "", "content", at); err != nil {
			t.Fatalf("NewUserItem at %s: %v", at, err)
		}
		if _, err := (FactBatch{Project: "/repo", SessionID: "session", Day: at.Format(time.DateOnly), CapturedAt: at}).Normalize(); err != nil {
			t.Fatalf("FactBatch at %s: %v", at, err)
		}
		if _, err := NewPublication("/repo", State{}, 1, nil, at); err != nil {
			t.Fatalf("NewPublication at %s: %v", at, err)
		}
	}
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).Add(-time.Nanosecond), time.Unix(0, math.MaxInt64).Add(time.Nanosecond)} {
		if _, err := NewUserItem(testItemID(t, 'a'), ScopeUser, "", "content", at); err == nil {
			t.Errorf("NewUserItem at %s succeeded", at)
		}
		if _, err := (FactBatch{Project: "/repo", SessionID: "session", Day: at.Format(time.DateOnly), CapturedAt: at}).Normalize(); err == nil {
			t.Errorf("FactBatch at %s succeeded", at)
		}
		if err := (LedgerFact{Sequence: 1, Day: at.Format(time.DateOnly), Content: "fact", CapturedAt: at}).Validate(); err == nil {
			t.Errorf("LedgerFact at %s succeeded", at)
		}
		if _, err := NewPublication("/repo", State{}, 1, nil, at); err == nil {
			t.Errorf("NewPublication at %s succeeded", at)
		}
	}
	item, err := NewProposal(testItemID(t, 'b'), "/repo", "content", time.Unix(0, math.MaxInt64).UTC())
	if err != nil {
		t.Fatal(err)
	}
	outside := item.UpdatedAt().Add(time.Nanosecond)
	if _, _, err := item.Edit(new("edited"), nil, outside); err == nil {
		t.Error("Edit past exact range succeeded")
	}
	if _, err := item.Review(ReviewApprove, outside); err == nil {
		t.Error("Review past exact range succeeded")
	}
	if _, err := item.ActivateFromUser("user content", outside); err == nil {
		t.Error("ActivateFromUser past exact range succeeded")
	}
	snapshot := item.Snapshot()
	snapshot.UpdatedAt = outside
	if _, err := RestoreItem(snapshot); err == nil {
		t.Error("RestoreItem past exact range succeeded")
	}
}

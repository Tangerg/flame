package agentmemory

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
)

func TestItemRevisionFencesStaleIntent(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	item, err := NewProposal(NewItemID([ItemIDEntropyBytes]byte{1}), "/repo", "original", now)
	if err != nil {
		t.Fatal(err)
	}
	change, err := item.Edit(item.Revision(), new("corrected"), nil, now)
	if err != nil {
		t.Fatal(err)
	}
	current := change.Item()
	if current.Revision() != item.Revision()+1 {
		t.Fatal("authoring did not advance the item revision")
	}
	if _, err := current.Edit(item.Revision(), new("stale"), nil, now); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale edit: %v", err)
	}
	if _, err := current.Review(item.Revision(), ReviewReject, now); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale review: %v", err)
	}
	if err := current.ValidateRemoval(item.Revision()); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale removal: %v", err)
	}
	unchanged, err := current.Edit(current.Revision(), new(current.Content()), nil, now)
	if err != nil || unchanged.Changed() || unchanged.Item().Revision() != current.Revision() {
		t.Fatalf("no-op changed the revision: %+v, %v", unchanged, err)
	}
	cache, err := NewEmbeddingUpdate(current, "space", []float32{1, 0})
	if err != nil {
		t.Fatal(err)
	}
	cached, err := current.WithEmbedding(cache)
	if err != nil || cached.Revision() != current.Revision() || cached.WithoutEmbedding().Revision() != current.Revision() {
		t.Fatalf("derived cache changed authoring revision: %+v, %v", cached, err)
	}
}

func TestItemRevisionAdmissionAndExhaustion(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	item, err := NewProposal(NewItemID([ItemIDEntropyBytes]byte{1}), "/repo", "fact", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, revision := range []uint64{0, exactint.Maximum + 1} {
		snapshot := item.Snapshot()
		snapshot.Revision = revision
		if _, err := RestoreItem(snapshot); !errors.Is(err, ErrInvalidRevision) {
			t.Fatalf("restore revision %d: %v", revision, err)
		}
		if _, err := item.Edit(revision, nil, nil, now); !errors.Is(err, ErrInvalidRevision) {
			t.Fatalf("edit revision %d: %v", revision, err)
		}
	}
	snapshot := item.Snapshot()
	snapshot.Revision = exactint.Maximum
	last, err := RestoreItem(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := last.Edit(last.Revision(), new("changed"), nil, now); !errors.Is(err, exactint.ErrExhausted) {
		t.Fatalf("exhausted edit: %v", err)
	}
	if _, err := last.Review(last.Revision(), ReviewApprove, now); !errors.Is(err, exactint.ErrExhausted) {
		t.Fatalf("exhausted review: %v", err)
	}
	if _, err := last.ActivateFromUser("fact", now); !errors.Is(err, exactint.ErrExhausted) {
		t.Fatalf("exhausted activation: %v", err)
	}
}

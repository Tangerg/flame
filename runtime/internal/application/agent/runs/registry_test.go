package runs

import (
	"errors"
	"testing"
)

func TestRegistryRemovesCompletedRun(t *testing.T) {
	var r registry
	owner := testRunTreeOwner(t, nil)
	r.Open(Record{ID: "run_1"}, owner, func() error { return nil })

	e, ok := r.Get("run_1")
	if !ok || e.owner != owner {
		t.Fatalf("entry = %+v, ok=%v", e, ok)
	}

	closed, ok := r.RemoveSegment("run_1", "")
	if !ok || closed.owner != owner {
		t.Fatalf("removed entry = %+v, ok=%v", closed, ok)
	}
	if _, ok := r.Get("run_1"); ok {
		t.Fatal("removed run remains live")
	}
}

func TestRegistryOldSegmentCannotRemoveItsReplacement(t *testing.T) {
	var reg registry
	oldOwner := testRunTreeOwner(t, nil)
	newOwner := testRunTreeOwner(t, nil)
	reg.Open(Record{ID: "run_1", SegmentID: "segment_old"}, oldOwner, func() error { return nil })
	reg.Open(Record{ID: "run_1", SegmentID: "segment_new"}, newOwner, func() error { return nil })

	if removed, ok := reg.RemoveSegment("run_1", "segment_old"); ok {
		t.Fatalf("old Segment removed replacement: %+v", removed)
	}
	live, ok := reg.Get("run_1")
	if !ok || live.record.SegmentID != "segment_new" || live.owner != newOwner {
		t.Fatalf("replacement after old removal = %+v, found=%t", live, ok)
	}
	if removed, ok := reg.RemoveSegment("run_1", "segment_new"); !ok || removed.owner != newOwner {
		t.Fatalf("exact replacement removal = %+v, found=%t", removed, ok)
	}
}

func TestRegistryFailedOpeningPreservesExistingOwner(t *testing.T) {
	var reg registry
	owner := testRunTreeOwner(t, nil)
	if err := reg.Open(Record{ID: "run_1", SegmentID: "seg_old"}, owner, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	rejected := errors.New("opening rejected")
	if err := reg.Open(Record{ID: "run_1", SegmentID: "seg_new"}, testRunTreeOwner(t, nil), func() error { return rejected }); !errors.Is(err, rejected) {
		t.Fatalf("opening error = %v", err)
	}
	live, ok := reg.Running("run_1")
	if !ok || live.owner != owner || live.record.SegmentID != "seg_old" {
		t.Fatalf("failed opening replaced owner: %+v", live)
	}
}

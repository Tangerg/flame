package runs

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
)

func TestRegistryRemovesCompletedRun(t *testing.T) {
	var r registry
	started := time.Unix(42, 0).UTC()
	owner := testRunTreeOwner(t, nil)
	r.Open(Record{ID: "run_1", SessionID: "ses_1", CWD: "/repo", CreatedAt: started}, owner, func() error { return nil })

	e, ok := r.Get("run_1")
	if !ok || e.record.CreatedAt != started || e.owner != owner {
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

func TestRegistryCancelReason(t *testing.T) {
	var r registry
	r.Open(Record{ID: "run_1", SessionID: "ses_1"}, nil, func() error { return nil })
	e, ok := r.MarkCancel("run_1", "user asked")
	if !ok {
		t.Fatal("mark cancel must find the run")
	}
	if e.record.CancelReason != "user asked" {
		t.Fatalf("cancel reason = %q", e.record.CancelReason)
	}
	if _, ok := r.MarkCancel("missing", "x"); ok {
		t.Fatal("mark cancel must miss unknown runs")
	}
}

func TestRegistryOwnsRunCapabilities(t *testing.T) {
	var reg registry
	capabilities := run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Approval},
	}
	reg.Open(Record{ID: "run_1", Capabilities: capabilities}, nil, func() error { return nil })
	capabilities.InterruptKinds[0] = interrupt.Question

	first, ok := reg.Get("run_1")
	if !ok || first.record.Capabilities.InterruptKinds[0] != interrupt.Approval {
		t.Fatalf("stored capabilities followed caller mutation: %+v", first.record.Capabilities)
	}
	first.record.Capabilities.InterruptKinds[0] = interrupt.Question

	second, ok := reg.Get("run_1")
	if !ok || second.record.Capabilities.InterruptKinds[0] != interrupt.Approval {
		t.Fatalf("Get leaked stored capabilities ownership: %+v", second.record.Capabilities)
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

package delivery

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

type acceptanceReadUseCases struct {
	runUseCases
	record runs.CommandAcceptanceRecord
	reads  int
	err    error
}

func (s *acceptanceReadUseCases) LookupCommandAcceptance(context.Context, string) (runs.CommandAcceptanceRecord, error) {
	s.reads++
	return s.record, s.err
}

func TestRunAcceptanceReadsOriginalReferencesAndCurrentMaterial(t *testing.T) {
	accepted := runs.CommandAcceptance{
		Key: "resume-once", Fingerprint: "0000000000000000000000000000000000000000000000000000000000000000",
		Operation: runs.CommandResumeOpening, SessionID: "ses_1", RunID: "run_1", SegmentID: "seg_original",
	}
	current := testsupport.MustRestoreRun(run.Snapshot{ID: accepted.RunID, SessionID: accepted.SessionID, ActiveSegmentID: "seg_later"})
	for _, retired := range []bool{false, true} {
		reader := &acceptanceReadUseCases{record: runs.CommandAcceptanceRecord{Found: true, Acceptance: accepted}}
		if !retired {
			reader.record.CurrentRun = &current
		}
		handler := &Handler{runs: reader, idempotency: protocol.IdempotencyLimits{Namespace: testsupport.IdempotencyNamespace}}
		result, err := handler.GetRunAcceptance(t.Context(), protocol.GetRunAcceptanceRequest{Namespace: testsupport.IdempotencyNamespace, Key: accepted.Key, Operation: protocol.RunCommandResume})
		if err != nil || result.RunID != accepted.RunID || result.SegmentID != accepted.SegmentID || result.UserItemID != "" || (result.Run == nil) != retired {
			t.Fatalf("acceptance=%+v,%v", result, err)
		}
		if !retired && result.Run.ActiveSegmentID != "seg_later" {
			t.Fatalf("current Segment replaced the original reference: %+v", result)
		}
		if err := protocol.ValidateWireTree(*result); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunAcceptancePreservesLookupFailuresAndNamespace(t *testing.T) {
	unavailable := errors.New("store unavailable")
	reader := &acceptanceReadUseCases{err: unavailable}
	handler := &Handler{runs: reader, idempotency: protocol.IdempotencyLimits{Namespace: testsupport.IdempotencyNamespace}}
	request := protocol.GetRunAcceptanceRequest{Namespace: testsupport.AlternateIdempotencyNamespace, Key: "key", Operation: protocol.RunCommandResume}
	if _, err := handler.GetRunAcceptance(t.Context(), request); !errors.Is(err, protocol.ErrIdempotencyStoreMismatch) || reader.reads != 0 {
		t.Fatalf("namespace mismatch consulted storage: %v, reads=%d", err, reader.reads)
	}
	request.Namespace = testsupport.IdempotencyNamespace
	if _, err := handler.GetRunAcceptance(t.Context(), request); !errors.Is(err, unavailable) {
		t.Fatalf("read failure became notRecorded: %v", err)
	}
	reader.err = nil
	reader.record.OtherOperation = true
	if _, err := handler.GetRunAcceptance(t.Context(), request); !errors.Is(err, protocol.ErrIdempotencyConflict) {
		t.Fatalf("foreign key=%v", err)
	}
	reader.record = runs.CommandAcceptanceRecord{Found: true, Acceptance: runs.CommandAcceptance{Operation: runs.CommandStartOpening}}
	if _, err := handler.GetRunAcceptance(t.Context(), request); !errors.Is(err, protocol.ErrIdempotencyConflict) {
		t.Fatalf("wrong Run operation=%v", err)
	}
	reader.record = runs.CommandAcceptanceRecord{}
	result, err := handler.GetRunAcceptance(t.Context(), request)
	if err != nil || result.Type != protocol.RunAcceptanceNotRecorded {
		t.Fatalf("unused key=%+v,%v", result, err)
	}
}

type acceptedSubscriptionUseCases struct {
	runUseCases
	calls int
	err   error
}

func (s *acceptedSubscriptionUseCases) Subscribe(context.Context, runs.SubscribeRequest) (runs.Subscription, error) {
	s.calls++
	return runs.Subscription{}, s.err
}

func TestAcceptedOpeningRetainsItsIdentityWhenReplayIsUnavailable(t *testing.T) {
	for _, cause := range []error{runs.ErrReplayCursorInvalid, runs.ErrReplayUnavailable, runs.ErrStaleSegment, runs.ErrRunFinished} {
		subscriber := &acceptedSubscriptionUseCases{err: cause}
		handler := &Handler{runs: subscriber}
		events, err := handler.acceptedRunEvents(t.Context(), runs.StartResult{RunID: "run_1", SegmentID: "seg_original", Reopened: true})
		if err != nil {
			t.Fatal(err)
		}
		for event, err := range events {
			t.Fatalf("unavailable original stream was replaced: %+v,%v", event, err)
		}
	}
	defect := errors.New("subscription store unavailable")
	subscriber := &acceptedSubscriptionUseCases{err: defect}
	handler := &Handler{runs: subscriber}
	if _, err := handler.acceptedRunEvents(t.Context(), runs.StartResult{RunID: "run_1", SegmentID: "seg_original"}); !errors.Is(err, defect) {
		t.Fatalf("observation defect was suppressed: %v", err)
	}
	subscriber.calls = 0
	if _, err := handler.acceptedRunEvents(t.Context(), runs.StartResult{RunID: "run_1", SegmentID: "seg_original", Reopened: true, MaterialRetired: true}); err != nil || subscriber.calls != 0 {
		t.Fatalf("retired material was attached: %v, calls=%d", err, subscriber.calls)
	}
}

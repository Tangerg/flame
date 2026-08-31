package run

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/agent"
	"github.com/Tangerg/flame/cli/internal/commandreplay"
	"github.com/Tangerg/flame/cli/internal/mutation"
	"github.com/Tangerg/flame/cli/internal/retry"
	"github.com/Tangerg/flame/cli/internal/workbench"
)

type steerRuntimeStub struct {
	requests     []agent.SteerRun
	err          error
	afterRequest func()
}

func (s *steerRuntimeStub) SteerRun(_ context.Context, request agent.SteerRun) error {
	s.requests = append(s.requests, request.Clone())
	err := s.err
	if s.afterRequest != nil {
		s.afterRequest()
	}
	return err
}

func TestRecoverReplaysAndAcknowledgesTheExactDurableSteer(t *testing.T) {
	fixture := stagedSteer(t)
	store, pending := fixture.store, fixture.pending
	runtime := new(steerRuntimeStub)
	fixture.now = pending.StagedAt().Add(time.Minute)
	if err := RecoverSteers(t.Context(), runtime, store, fixture.policy(t), retry.ImmediateBackoff()); err != nil {
		t.Fatal(err)
	}
	if len(runtime.requests) != 1 || !runtime.requests[0].Equal(pending.Command()) {
		t.Fatalf("replayed requests = %+v", runtime.requests)
	}
	if _, found := store.PendingSteer(pending.SessionID()); found {
		t.Fatal("acknowledged steer remains pending")
	}
	history := store.History()
	if len(history) != 1 || !history[0].Equal(pending.Message()) {
		t.Fatalf("accepted steer history = %+v", history)
	}
}

func TestRecoverReturnsAttachmentsAfterAReplayableRefusal(t *testing.T) {
	fixture := stagedSteer(t)
	store, pending := fixture.store, fixture.pending
	runtime := &steerRuntimeStub{err: agent.ErrStaleSegment}
	fixture.now = pending.StagedAt().Add(time.Minute)
	if err := RecoverSteers(t.Context(), runtime, store, fixture.policy(t), retry.ImmediateBackoff()); err != nil {
		t.Fatal(err)
	}
	if _, found := store.PendingSteer(pending.SessionID()); found {
		t.Fatal("rejected steer remains pending")
	}
	draft, found, err := store.Draft(pending.SessionID())
	if err != nil || !found || len(draft.Attachments) != 1 ||
		draft.Attachments[0] != pending.Message().Attachments[0] {
		t.Fatalf("recovered draft = %+v, found %t, error %v", draft, found, err)
	}
}

func TestRecoverRefusesToGuessAtOrAfterTheReplayDeadline(t *testing.T) {
	for _, offset := range []time.Duration{0, time.Nanosecond} {
		t.Run(offset.String(), func(t *testing.T) {
			fixture := stagedSteer(t)
			store, pending := fixture.store, fixture.pending
			runtime := new(steerRuntimeStub)
			fixture.now = pending.Replay().Until().Add(offset)
			err := RecoverSteers(t.Context(), runtime, store, fixture.policy(t), retry.ImmediateBackoff())
			if err == nil {
				t.Fatal("expired replay unexpectedly succeeded")
			}
			if len(runtime.requests) != 0 {
				t.Fatalf("expired replay reached runtime: %+v", runtime.requests)
			}
			if durable, found := store.PendingSteer(pending.SessionID()); !found || !durable.Command().Equal(pending.Command()) {
				t.Fatalf("expired pending steer = %+v, found %t", durable, found)
			}
		})
	}
}

func TestDeliverPreservesACommandRejectedByAnotherRuntimeStore(t *testing.T) {
	fixture := stagedSteer(t)
	pending := fixture.pending
	runtime := &steerRuntimeStub{err: agent.ErrCommandStoreMismatch}
	fixture.now = pending.StagedAt().Add(time.Minute)
	result, err := DeliverSteer(t.Context(), runtime, pending, fixture.policy(t), retry.ImmediateBackoff())
	if !errors.Is(err, agent.ErrCommandStoreMismatch) || result.Outcome != mutation.Unknown {
		t.Fatalf("store mismatch settlement = outcome %v, error %v", result.Outcome, err)
	}
	if len(runtime.requests) != 1 {
		t.Fatalf("store mismatch attempts = %+v", runtime.requests)
	}
}

func TestRecoverStopsRetryingWhenTheReplayGuaranteeExpires(t *testing.T) {
	fixture := stagedSteer(t)
	store, pending := fixture.store, fixture.pending
	fixture.now = pending.Replay().Until().Add(-time.Nanosecond)
	runtime := new(steerRuntimeStub)
	runtime.err = agent.ErrDisconnected
	runtime.afterRequest = func() {
		fixture.now = pending.Replay().Until()
		runtime.err = nil
	}
	err := RecoverSteers(t.Context(), runtime, store, fixture.policy(t), retry.ImmediateBackoff())
	if !errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) {
		t.Fatalf("recovery error = %v", err)
	}
	if len(runtime.requests) != 1 {
		t.Fatalf("expired command reached runtime %d times", len(runtime.requests))
	}
}

func TestUnavailableRuntimeSeparatesFreshSteerDeliveryFromColdRecovery(t *testing.T) {
	store, err := workbench.OpenDirectory(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	attachment := agent.Attachment{
		ID: "att_notes", Kind: agent.AttachmentText, Name: "notes.txt",
		Path: filepath.Join(t.TempDir(), "notes.txt"), MimeType: "text/plain", Size: 5,
	}
	request := agent.SteerRun{
		CommandID: "cli_33333333333333333333333333333333",
		RunID:     "run_1", SegmentID: "seg_1",
		Message: agent.Message{Text: "inspect ownership", Attachments: []agent.Attachment{attachment}},
	}
	source := agent.Message{Text: "/steer inspect ownership", Attachments: []agent.Attachment{attachment}}
	if err := store.SaveDraft("ses_1", source); err != nil {
		t.Fatal(err)
	}
	policy := commandreplay.UnavailablePolicy()
	pending, err := StageSteer(store, "ses_1", request, source, policy)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &steerRuntimeStub{err: agent.ErrDisconnected}
	result, err := DeliverSteer(t.Context(), runtime, pending, policy, retry.ImmediateBackoff())
	if result.Outcome != mutation.Unknown || !errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) || len(runtime.requests) != 1 {
		t.Fatalf("fresh delivery = outcome %v, error %v, requests %+v", result.Outcome, err, runtime.requests)
	}

	runtime = new(steerRuntimeStub)
	err = RecoverSteers(t.Context(), runtime, store, policy, retry.ImmediateBackoff())
	if err == nil || len(runtime.requests) != 0 {
		t.Fatalf("cold recovery = %v, requests %+v", err, runtime.requests)
	}
	if durable, found := store.PendingSteer(pending.SessionID()); !found || !durable.Command().Equal(pending.Command()) {
		t.Fatalf("unprotected steer = %+v, found %t", durable, found)
	}
}

type steerFixture struct {
	store      *workbench.Store
	pending    workbench.PendingSteer
	capability commandreplay.Capability
	now        time.Time
}

func (f *steerFixture) policy(t *testing.T) commandreplay.Policy {
	t.Helper()
	policy, err := commandreplay.NewPolicyWithClock(f.capability, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func stagedSteer(t *testing.T) *steerFixture {
	t.Helper()
	store, err := workbench.OpenDirectory(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	stagedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	capability, err := commandreplay.NewCapability("runtime-test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &steerFixture{store: store, capability: capability, now: stagedAt}
	attachment := agent.Attachment{
		ID: "att_notes", Kind: agent.AttachmentText, Name: "notes.txt",
		Path: filepath.Join(t.TempDir(), "notes.txt"), MimeType: "text/plain", Size: 5,
	}
	request := agent.SteerRun{
		CommandID: "cli_22222222222222222222222222222222",
		RunID:     "run_1", SegmentID: "seg_1",
		Message: agent.Message{Text: "inspect the parser", Attachments: []agent.Attachment{attachment}},
	}
	source := agent.Message{Text: "/steer inspect the parser", Attachments: []agent.Attachment{attachment}}
	if saveDraftErr := store.SaveDraft("ses_1", source); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	pending, err := StageSteer(store, "ses_1", request, source, fixture.policy(t))
	if err != nil {
		t.Fatal(err)
	}
	fixture.pending = pending
	return fixture
}

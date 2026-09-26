package run

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

type steerRuntimeStub struct {
	requests     []agent.SteerRun
	err          error
	afterRequest func()
}

func (s *steerRuntimeStub) SteerRun(_ context.Context, request agent.SteerRun) (protocol.SteerRunResponse, error) {
	s.requests = append(s.requests, request.Clone())
	err := s.err
	if s.afterRequest != nil {
		s.afterRequest()
	}
	return protocol.SteerRunResponse{UserItemID: "item_steer"}, err
}

func TestRecoverReplaysAndAcknowledgesTheExactDurableSteer(t *testing.T) {
	fixture := stagedSteer(t)
	store, pending := fixture.store, fixture.pending
	runtime := new(steerRuntimeStub)
	fixture.now = pending.StagedAt().Add(time.Minute)
	accepted, err := RecoverSteers(t.Context(), runtime, store, fixture.policy(t), fastBackoff(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(accepted) != 1 || accepted[0].Receipt.UserItemID != "item_steer" ||
		accepted[0].Pending.SessionID() != pending.SessionID() || !accepted[0].Pending.Command().Equal(pending.Command()) {
		t.Fatalf("recovered acceptance receipts = %+v", accepted)
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
	if _, err := RecoverSteers(t.Context(), runtime, store, fixture.policy(t), fastBackoff(t)); err != nil {
		t.Fatal(err)
	}
	if _, found := store.PendingSteer(pending.SessionID()); found {
		t.Fatal("rejected steer remains pending")
	}
	draft, found := store.Draft(pending.SessionID())
	if !found || len(draft.Attachments) != 1 ||
		draft.Attachments[0] != pending.Message().Attachments[0] {
		t.Fatalf("recovered draft = %+v, found %t", draft, found)
	}
}

func TestRecoverRefusesToGuessAtOrAfterTheReplayDeadline(t *testing.T) {
	for _, offset := range []time.Duration{0, time.Nanosecond} {
		t.Run(offset.String(), func(t *testing.T) {
			fixture := stagedSteer(t)
			store, pending := fixture.store, fixture.pending
			runtime := new(steerRuntimeStub)
			fixture.now = pending.Replay().Until().Add(offset)
			_, err := RecoverSteers(t.Context(), runtime, store, fixture.policy(t), fastBackoff(t))
			if !errors.Is(err, ErrSteerReplayUnavailable) {
				t.Fatalf("expired replay error = %v, want ErrSteerReplayUnavailable", err)
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

func TestRecoverContinuesPastAnUnreplayableSteer(t *testing.T) {
	fixture := stagedSteer(t)
	expired := fixture.pending
	secondStagedAt := expired.StagedAt().Add(45 * time.Minute)
	guard, err := fixture.policy(t).NewGuardAt(secondStagedAt)
	if err != nil {
		t.Fatal(err)
	}
	secondCommand := agent.SteerRun{
		CommandID: "cli_44444444444444444444444444444444",
		RunID:     "run_2",
		SegmentID: "seg_2",
		Message:   agent.Message{Text: "recover safely"},
	}
	second, err := workbench.NewPendingSteer("ses_2", secondCommand, secondStagedAt, guard)
	if err != nil {
		t.Fatal(err)
	}
	secondSource := agent.Message{Text: "/steer recover safely"}
	if err := fixture.store.SaveDraft(second.SessionID(), secondSource); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.StagePendingSteer(second, secondSource, prepareSteerTestInput(t, fixture.store, second.Command())); err != nil {
		t.Fatal(err)
	}

	fixture.now = expired.StagedAt().Add(90 * time.Minute)
	runtime := new(steerRuntimeStub)
	_, err = RecoverSteers(
		t.Context(), runtime, fixture.store, fixture.policy(t), fastBackoff(t),
	)
	if !errors.Is(err, ErrSteerReplayUnavailable) {
		t.Fatalf("recovery error = %v, want deferred expired steer", err)
	}
	if len(runtime.requests) != 1 || !runtime.requests[0].Equal(second.Command()) {
		t.Fatalf("safely replayable requests = %+v, want second steer", runtime.requests)
	}
	if durable, found := fixture.store.PendingSteer(expired.SessionID()); !found ||
		!durable.Command().Equal(expired.Command()) {
		t.Fatalf("expired steer = %+v, found %t", durable, found)
	}
	if _, found := fixture.store.PendingSteer(second.SessionID()); found {
		t.Fatal("replayed second steer remains pending")
	}
}

func TestDeliverPreservesACommandRejectedByAnotherRuntimeStore(t *testing.T) {
	fixture := stagedSteer(t)
	pending := fixture.pending
	runtime := &steerRuntimeStub{err: agent.ErrCommandStoreMismatch}
	fixture.now = pending.StagedAt().Add(time.Minute)
	result, err := DeliverSteer(t.Context(), runtime, pending, fixture.policy(t), fastBackoff(t))
	if !errors.Is(err, agent.ErrCommandStoreMismatch) || result.Outcome != mutation.Unknown {
		t.Fatalf("store mismatch settlement = outcome %v, error %v", result.Outcome, err)
	}
	if len(runtime.requests) != 1 {
		t.Fatalf("store mismatch attempts = %+v", runtime.requests)
	}
}

func TestSteerConflictCannotRetireAnUncertainOriginalCommand(t *testing.T) {
	fixture := stagedSteer(t)
	runtime := &steerRuntimeStub{err: agent.ErrCommandConflict}
	result, err := DeliverSteer(t.Context(), runtime, fixture.pending, fixture.policy(t), fastBackoff(t))
	if result.Outcome != mutation.Unknown || !errors.Is(err, agent.ErrCommandConflict) {
		t.Fatalf("conflict settlement = %s, %v", result.Outcome, err)
	}
	if _, err := RecoverSteers(t.Context(), runtime, fixture.store, fixture.policy(t), fastBackoff(t)); !errors.Is(err, agent.ErrCommandConflict) {
		t.Fatalf("conflict recovery = %v", err)
	}
	pending, found := fixture.store.PendingSteer(fixture.pending.SessionID())
	if !found || !pending.Command().Equal(fixture.pending.Command()) {
		t.Fatal("conflict discarded or replaced the original steer")
	}
	if _, found := fixture.store.Draft(fixture.pending.SessionID()); found {
		t.Fatal("conflict returned uncertain attachments to the editable draft")
	}
}

func TestUnpreparedLegacySteerHasUnknownOutcomeWithoutADispatch(t *testing.T) {
	fixture := stagedSteer(t)
	command := fixture.pending.Command()
	command.Input = nil
	legacy, err := workbench.NewPendingSteer(fixture.pending.SessionID(), command, fixture.pending.StagedAt(), fixture.pending.Replay())
	if err != nil {
		t.Fatal(err)
	}
	runtime := new(steerRuntimeStub)
	result, err := DeliverSteer(t.Context(), runtime, legacy, fixture.policy(t), fastBackoff(t))
	if result.Outcome != mutation.Unknown || !errors.Is(err, agent.ErrCommandInputUnavailable) || len(runtime.requests) != 0 {
		t.Fatalf("legacy steer = %s, %v, %d dispatches", result.Outcome, err, len(runtime.requests))
	}
}

func TestDeliverRetainsTheExactSteerAcceptanceReceipt(t *testing.T) {
	fixture := stagedSteer(t)
	result, err := DeliverSteer(t.Context(), new(steerRuntimeStub), fixture.pending, fixture.policy(t), fastBackoff(t))
	if err != nil || result.Outcome != mutation.Confirmed || result.Receipt.UserItemID != "item_steer" {
		t.Fatalf("steer acceptance = %+v, error %v", result, err)
	}
}

func TestMissingSteerReceiptCannotRejectOrRetryAcceptedInput(t *testing.T) {
	fixture := stagedSteer(t)
	runtime := &steerRuntimeStub{err: agent.ErrSteerReceiptUnavailable}
	result, err := DeliverSteer(t.Context(), runtime, fixture.pending, fixture.policy(t), fastBackoff(t))
	if !errors.Is(err, agent.ErrSteerReceiptUnavailable) || result.Outcome != mutation.Unknown || len(runtime.requests) != 1 {
		t.Fatalf("missing receipt settlement = %+v, error %v, attempts %d", result, err, len(runtime.requests))
	}
	_, err = RecoverSteers(t.Context(), runtime, fixture.store, fixture.policy(t), fastBackoff(t))
	if !errors.Is(err, agent.ErrSteerReceiptUnavailable) {
		t.Fatalf("missing receipt recovery = %v", err)
	}
	if _, found := fixture.store.PendingSteer(fixture.pending.SessionID()); !found {
		t.Fatal("missing receipt retired the command journal")
	}
	if _, found := fixture.store.Draft(fixture.pending.SessionID()); found {
		t.Fatal("missing receipt returned accepted attachments for resending")
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
	_, err := RecoverSteers(t.Context(), runtime, store, fixture.policy(t), fastBackoff(t))
	if !errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) {
		t.Fatalf("recovery error = %v", err)
	}
	if len(runtime.requests) != 1 {
		t.Fatalf("expired command reached runtime %d times", len(runtime.requests))
	}
}

func TestUnavailableRuntimeSeparatesFreshSteerDeliveryFromColdRecovery(t *testing.T) {
	store, err := openTestWorkbench(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	attachment := agent.Attachment{
		ID: "att_notes", Kind: protocol.ContentBlockText, Name: "notes.txt",
		Path: filepath.Join(t.TempDir(), "notes.txt"), MimeType: "text/plain", Size: 5,
	}
	request := agent.SteerRun{
		CommandID: "cli_33333333333333333333333333333333",
		RunID:     "run_1", SegmentID: "seg_1",
		Message: agent.Message{Text: "inspect ownership", Attachments: []agent.Attachment{attachment}},
		Input: []protocol.ContentBlock{
			{Type: protocol.ContentBlockText, Text: "inspect ownership"},
			{Type: protocol.ContentBlockText, Text: "fixture attachment"},
		},
	}
	source := agent.Message{Text: "/steer inspect ownership", Attachments: []agent.Attachment{attachment}}
	if err := store.SaveDraft("ses_1", source); err != nil {
		t.Fatal(err)
	}
	policy := unavailableReplayPolicy(t)
	pending, err := StageSteer(store, "ses_1", request, source, policy, prepareSteerTestInput(t, store, request))
	if err != nil {
		t.Fatal(err)
	}
	runtime := &steerRuntimeStub{err: agent.ErrDisconnected}
	result, err := DeliverSteer(t.Context(), runtime, pending, policy, fastBackoff(t))
	if result.Outcome != mutation.Unknown || !errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) || len(runtime.requests) != 1 {
		t.Fatalf("fresh delivery = outcome %v, error %v, requests %+v", result.Outcome, err, runtime.requests)
	}

	runtime = new(steerRuntimeStub)
	_, err = RecoverSteers(t.Context(), runtime, store, policy, fastBackoff(t))
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

func (f *steerFixture) policy(t *testing.T) mutation.ReplayPolicy {
	t.Helper()
	policy, err := mutation.NewReplayPolicy(f.capability, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func stagedSteer(t *testing.T) *steerFixture {
	t.Helper()
	store, err := openTestWorkbench(t.TempDir())
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
		ID: "att_notes", Kind: protocol.ContentBlockText, Name: "notes.txt",
		Path: filepath.Join(t.TempDir(), "notes.txt"), MimeType: "text/plain", Size: 5,
	}
	request := agent.SteerRun{
		CommandID: "cli_22222222222222222222222222222222",
		RunID:     "run_1", SegmentID: "seg_1",
		Message: agent.Message{Text: "inspect the parser", Attachments: []agent.Attachment{attachment}},
		Input: []protocol.ContentBlock{
			{Type: protocol.ContentBlockText, Text: "inspect the parser"},
			{Type: protocol.ContentBlockText, Text: "fixture attachment"},
		},
	}
	source := agent.Message{Text: "/steer inspect the parser", Attachments: []agent.Attachment{attachment}}
	if saveDraftErr := store.SaveDraft("ses_1", source); saveDraftErr != nil {
		t.Fatal(saveDraftErr)
	}
	pending, err := StageSteer(store, "ses_1", request, source, fixture.policy(t), prepareSteerTestInput(t, store, request))
	if err != nil {
		t.Fatal(err)
	}
	fixture.pending = pending
	return fixture
}

// fastBackoff is an ordinary bounded schedule whose floor is short enough that
// a retry loop finishes within a test. Production configures the same shape.
func fastBackoff(t testing.TB) retry.Backoff {
	t.Helper()
	backoff, err := retry.NewBackoff(time.Nanosecond, time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	return backoff
}

func prepareSteerTestInput(t *testing.T, store *workbench.Store, request agent.SteerRun) *workbench.PreparedInput {
	t.Helper()
	input, err := store.PrepareInput(t.Context(), request.Message, request.Input)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

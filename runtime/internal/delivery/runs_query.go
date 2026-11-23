package delivery

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func (s *Handler) GetRun(ctx context.Context, in protocol.GetRunRequest) (*protocol.RunRef, error) {
	runID, err := runParameter(in.RunID)
	if err != nil {
		return nil, err
	}
	run, found, err := s.queries.Run(ctx, runID)
	switch {
	case err != nil:
		return nil, err
	case !found:
		return nil, protocol.ErrRunNotFound
	}
	if run.Lineage().IsChild() {
		if err := s.requireFeature(ctx, protocol.FeatureChildRuns); err != nil {
			return nil, err
		}
	}
	ref := presentRun(run)
	return &ref, nil
}

var runCommandOperations = wireVocabulary[runs.CommandOperation, protocol.RunCommandOperation]{
	{runs.CommandStartOpening, protocol.RunCommandStart},
	{runs.CommandResumeOpening, protocol.RunCommandResume},
	{runs.CommandSteerInput, protocol.RunCommandSteer},
}

// A start lookup that carries its request must match the accepted request
// exactly; a different request under the same key is a conflict.
func checkStartAcceptanceReplay(ctx context.Context, in protocol.GetRunAcceptanceRequest, record runs.CommandAcceptanceRecord) error {
	if in.Operation != protocol.RunCommandStart {
		return protocol.ErrInvalidParams
	}
	if record.Use != runs.CommandKeyAccepted {
		return nil
	}
	capabilities, _ := clientCapabilitiesFrom(ctx)
	fingerprint, err := runCommandFingerprint(RunsStart, *in.Request, capabilities)
	if err != nil {
		return err
	}
	if record.Acceptance.Fingerprint != fingerprint {
		return protocol.ErrIdempotencyConflict
	}
	return nil
}

func (s *Handler) GetRunAcceptance(ctx context.Context, in protocol.GetRunAcceptanceRequest) (*protocol.RunAcceptance, error) {
	if in.Namespace != s.idempotency.Namespace {
		return nil, protocol.ErrIdempotencyStoreMismatch
	}
	commandKey := operation.ScopedKey(ctx, in.Key)
	record, err := s.runs.LookupCommandAcceptance(ctx, commandKey)
	if err != nil {
		return nil, err
	}
	operation, known := runCommandOperations.domain(in.Operation)
	if !known {
		return nil, protocol.ErrInvalidParams
	}
	if record.Use == runs.CommandKeyForeign || (record.Use == runs.CommandKeyAccepted && record.Acceptance.Operation != operation) {
		return nil, protocol.ErrIdempotencyConflict
	}
	if in.Request != nil {
		if err := checkStartAcceptanceReplay(ctx, in, record); err != nil {
			return nil, err
		}
	}
	response := presentRunAcceptance(record)
	if record.Use != runs.CommandKeyAccepted && in.Operation == protocol.RunCommandSteer {
		response.CandidateUserItemID = runs.SteerItemID(commandKey).String()
	}
	return response, nil
}

func presentRunAcceptance(record runs.CommandAcceptanceRecord) *protocol.RunAcceptance {
	if record.Use != runs.CommandKeyAccepted {
		return &protocol.RunAcceptance{Type: protocol.RunAcceptanceNotRecorded}
	}
	response := &protocol.RunAcceptance{
		Type: protocol.RunAcceptanceRetired, RunID: record.Acceptance.RunID.String(),
		SegmentID: record.Acceptance.SegmentID.String(), SessionID: record.Acceptance.SessionID.String(),
		UserItemID: record.Acceptance.UserItemID.String(),
	}
	for _, block := range record.Acceptance.SteerInput {
		response.Input = append(response.Input, presentContent(block))
	}
	if record.AppliedItem != nil {
		item := presentItem(*record.AppliedItem)
		response.AppliedItem = &item
	}
	if record.CurrentRun != nil {
		current := presentRun(*record.CurrentRun)
		response.Type, response.Run = protocol.RunAcceptanceAccepted, &current
	}
	return response
}

func (s *Handler) ListRuns(ctx context.Context, in protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
	sessionID, parseErr := optionalSessionParameter(in.SessionID)
	if parseErr != nil {
		return nil, parseErr
	}
	limit, err := pagination.ParseLimit(in.Limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	statuses, err := runStatusesFromWire(in.Statuses)
	if err != nil {
		return nil, err
	}
	page, err := s.queries.ListRunPage(ctx, sessions.RunPageFilter{
		SessionID:          sessionID,
		Statuses:           statuses,
		IncludeDescendants: in.IncludeDescendants,
	}, in.Cursor, limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	out := make([]protocol.RunRef, 0, len(page.Rows))
	for _, run := range page.Rows {
		out = append(out, presentRun(run))
	}
	return protocol.NewPageWithCursor(out, page.NextCursor), nil
}

// Go's decoder puts any string into a named string type, so an unknown value must
// be refused rather than dropped, which would silently widen the page.
func runStatusesFromWire(statuses []protocol.RunStatus) ([]run.State, error) {
	out := make([]run.State, 0, len(statuses))
	for _, status := range statuses {
		switch status {
		case protocol.RunStatusRunning:
			out = append(out, run.Running)
		case protocol.RunStatusWaiting:
			out = append(out, run.Waiting)
		case protocol.RunStatusFinished:
			out = append(out, run.Finished)
		default:
			return nil, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("statuses contains unknown run status %q", status))
		}
	}
	return out, nil
}

func (s *Handler) ListInterrupts(ctx context.Context, in protocol.ListInterruptsRequest) (*protocol.Page[protocol.PendingInterruptSet], error) {
	sessionID, parseErr := optionalSessionParameter(in.SessionID)
	if parseErr != nil {
		return nil, parseErr
	}
	limit, err := pagination.ParseLimit(in.Limit)
	if err != nil {
		return nil, wireInterruptPageError(wirePageError(err))
	}
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	rootRunID, err := wireIdentity("rootRunId", in.RootRunID, identity.ParseOptionalRun)
	if err != nil {
		return nil, err
	}
	page, err := s.queries.ListPendingInterruptPage(ctx, sessionID, rootRunID, caller, in.Cursor, limit)
	if err != nil {
		return nil, wireInterruptPageError(wirePageError(err))
	}
	out := make([]protocol.PendingInterruptSet, 0, len(page.Rows))
	for _, set := range page.Rows {
		presented := protocol.PendingInterruptSet{
			RootRunID:  set.Pending.RootRunID.String(),
			SessionID:  set.SessionID.String(),
			Interrupts: presentInterrupts(set.Interrupts),
			CreatedAt:  set.Pending.CreatedAt,
		}
		if err := protocol.ValidateWireTree(presented); err != nil {
			return nil, fmt.Errorf("present pending interrupt set %q: %w", set.Pending.RootRunID, err)
		}
		out = append(out, presented)
	}
	return protocol.NewPageWithCursor(out, page.NextCursor), nil
}

func wireInterruptPageError(err error) error {
	if uncovered, ok := errors.AsType[*run.InsufficientCapabilitiesError](err); ok {
		return capabilityGap(uncovered.Missing)
	}
	switch {
	case errors.Is(err, transcript.ErrNotRoot):
		return protocol.NewFailure(protocol.ErrRunNotRoot, err.Error(), err)
	default:
		return err
	}
}

func (s *Handler) SubscribeRun(ctx context.Context, in protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, nil, err
	}
	segmentID, err := wireIdentity(segmentIDField, in.SegmentID, identity.ParseSegment)
	if err != nil {
		return nil, nil, err
	}
	runID, err := runParameter(in.RunID)
	if err != nil {
		return nil, nil, err
	}
	request := runs.SubscribeRequest{
		RunID:              runID,
		SegmentID:          segmentID,
		Cursor:             strings.TrimPrefix(AfterEventIDFrom(ctx), protocol.IDPrefixEvent),
		CallerCapabilities: caller,
	}
	var snapshot *protocol.SessionSnapshot
	var attached runs.Subscription
	if in.Snapshot {
		if request.Cursor != "" {
			return nil, nil, protocol.NewFailure(protocol.ErrInvalidParams, "snapshot subscription cannot replay a cursor")
		}
		attached, err = s.runStreams.SubscribeSnapshot(ctx, request, func(ctx context.Context, sessionID identity.SessionID) error {
			var readErr error
			snapshot, readErr = s.GetSessionSnapshot(ctx, protocol.GetSessionSnapshotRequest{SessionID: sessionID.String(), IncludeDescendants: true})
			return readErr
		})
	} else {
		attached, err = s.runStreams.Subscribe(ctx, request)
	}

	if err != nil {
		return nil, nil, wireLiveSegmentError(err)
	}
	var head *string
	if attached.HeadCursor != "" {
		framed := protocol.IDPrefixEvent + attached.HeadCursor
		head = &framed
	}
	return &protocol.SubscribeRunResponse{
		RunID: in.RunID, SegmentID: attached.Record.SegmentID.String(), HeadEventID: head, Snapshot: snapshot,
	}, mapRunEvents(attached.Events), nil
}

func wireLiveSegmentError(err error) error {
	if uncovered, ok := errors.AsType[*run.InsufficientCapabilitiesError](err); ok {
		return capabilityGap(uncovered.Missing)
	}
	switch {
	case errors.Is(err, runs.ErrRunNotFound):
		return protocol.ErrRunNotFound
	case errors.Is(err, transcript.ErrNotRoot):
		return protocol.NewFailure(protocol.ErrRunNotRoot, err.Error(), err)
	case errors.Is(err, runs.ErrRunWaiting):
		return protocol.NewFailure(protocol.ErrRunWaiting, err.Error(), err)
	case errors.Is(err, runs.ErrRunFinished):
		return protocol.NewFailure(protocol.ErrRunFinished, err.Error(), err)
	case errors.Is(err, runs.ErrStaleSegment):
		return protocol.NewFailure(protocol.ErrStaleSegment, err.Error(), err)
	case errors.Is(err, runs.ErrReplayCursorInvalid):
		return protocol.NewFailure(protocol.ErrReplayCursorInvalid, err.Error(), err)
	case errors.Is(err, runs.ErrReplayUnavailable):
		return protocol.NewFailure(protocol.ErrReplayUnavailable, err.Error(), err)
	default:
		return err
	}
}

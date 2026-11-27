package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type Opening struct {
	segment conversation.SegmentStream
}

func (o Opening) RunID() string { return o.segment.RunID }

func (o Opening) Stream() (conversation.SegmentStream, bool) {
	return o.segment, o.segment.Events != nil
}

type OpeningSource interface {
	GetRunAcceptance(context.Context, protocol.GetRunAcceptanceRequest) (protocol.RunAcceptance, error)
	StartRun(context.Context, authoring.StartRun) (conversation.SegmentStream, error)
	ResumeRun(context.Context, conversation.ResumeRun) (conversation.SegmentStream, error)
}

func openStart(ctx context.Context, source OpeningSource, command authoring.StartRun, guard authoring.ReplayGuard, policy mutation.ReplayPolicy) (Opening, error) {
	return openStartCommand(ctx, source, command, guard, policy, func(ctx context.Context) (conversation.SegmentStream, error) { return source.StartRun(ctx, command) })
}

func OpenPendingStart(ctx context.Context, source OpeningSource, store *workbench.Store, pending workbench.PendingRun, policy mutation.ReplayPolicy) (Opening, error) {
	if store == nil {
		return Opening{}, workbench.ErrUnavailable
	}
	if pending.Dispatch == nil {
		return Opening{}, fmt.Errorf("queued prompt %q has no dispatch", pending.EntryID)
	}
	return openStartCommand(ctx, source, pending.Command, pending.Dispatch.Replay, policy, replayPending(store, pending.ReplayCommand, source.StartRun))
}

func replayPending[C any](store *workbench.Store, replay func() (C, error), send func(context.Context, C) (conversation.SegmentStream, error)) func(context.Context) (conversation.SegmentStream, error) {
	return func(ctx context.Context) (conversation.SegmentStream, error) {
		if err := store.DispatchReady(); err != nil {
			return conversation.SegmentStream{}, err
		}
		command, err := replay()
		if err != nil {
			return conversation.SegmentStream{}, err
		}
		return send(ctx, command)
	}
}

func openStartCommand(ctx context.Context, source OpeningSource, command authoring.StartRun, guard authoring.ReplayGuard, policy mutation.ReplayPolicy, dispatch func(context.Context) (conversation.SegmentStream, error)) (Opening, error) {
	return openCommand(ctx, source, protocol.GetRunAcceptanceRequest{
		Namespace: guard.Namespace(), Key: string(command.CommandID), Operation: protocol.RunCommandStart,
	}, guard, policy, func(receipt protocol.RunAcceptance) error {
		if receipt.SessionID != command.SessionID || receipt.UserItemID == "" {
			return fmt.Errorf("start receipt does not match the submitted session and opening input")
		}
		return nil
	}, dispatch, conversation.SegmentStream.ValidateStart)
}

func openResume(ctx context.Context, source OpeningSource, command conversation.ResumeRun, guard authoring.ReplayGuard, policy mutation.ReplayPolicy) (Opening, error) {
	return openResumeCommand(ctx, source, command, guard, policy, func(ctx context.Context) (conversation.SegmentStream, error) { return source.ResumeRun(ctx, command) })
}

func OpenPendingResume(ctx context.Context, source OpeningSource, store *workbench.Store, pending workbench.PendingResume, policy mutation.ReplayPolicy) (Opening, error) {
	if store == nil {
		return Opening{}, workbench.ErrUnavailable
	}
	return openResumeCommand(ctx, source, pending.Command, pending.Replay, policy, replayPending(store, pending.ReplayCommand, source.ResumeRun))
}

func openResumeCommand(ctx context.Context, source OpeningSource, command conversation.ResumeRun, guard authoring.ReplayGuard, policy mutation.ReplayPolicy, dispatch func(context.Context) (conversation.SegmentStream, error)) (Opening, error) {
	return openCommand(ctx, source, protocol.GetRunAcceptanceRequest{
		Namespace: guard.Namespace(), Key: string(command.CommandID), Operation: protocol.RunCommandResume,
	}, guard, policy, func(receipt protocol.RunAcceptance) error {
		if receipt.RunID != command.RunID || (receipt.UserItemID != "") != (command.Message != nil) {
			return fmt.Errorf("resume receipt does not match the submitted run and input")
		}
		return nil
	}, dispatch, func(stream conversation.SegmentStream) error {
		return stream.ValidateResume(command.RunID, command.Message)
	})
}

func openCommand(
	ctx context.Context,
	source OpeningSource,
	query protocol.GetRunAcceptanceRequest,
	guard authoring.ReplayGuard,
	policy mutation.ReplayPolicy,
	corresponds func(protocol.RunAcceptance) error,
	dispatch func(context.Context) (conversation.SegmentStream, error),
	validate func(conversation.SegmentStream) error,
) (Opening, error) {
	if err := context.Cause(ctx); err != nil {
		return Opening{}, err
	}
	if !policy.SameStore(guard) {
		return Opening{}, mutation.ErrReplayGuaranteeUnavailable
	}
	lookup := func() (protocol.RunAcceptance, error) {
		receipt, err := source.GetRunAcceptance(ctx, query)
		if err != nil {
			return protocol.RunAcceptance{}, fmt.Errorf("%w: query run acceptance: %w", authoring.ErrCommandOutcomeUnknown, err)
		}
		return receipt, nil
	}
	receipt, err := lookup()
	if err != nil {
		return Opening{}, err
	}
	if receipt.Type != protocol.RunAcceptanceNotRecorded {
		return recordedOpening(receipt, corresponds)
	}
	if err := mutation.ReplayAdmission(policy, guard)(); err != nil {
		return Opening{}, err
	}

	observationCtx, release := context.WithCancel(ctx)
	stream, err := dispatch(observationCtx)
	if err == nil {
		err = validate(stream)
		if err == nil {
			return Opening{segment: releaseWhenDone(stream, release)}, nil
		}
		err = errors.Join(conversation.ErrInvalidOpeningObservation, err)
	}
	release()
	if !errors.Is(err, conversation.ErrInvalidOpeningObservation) && !errors.Is(err, protocol.ErrInvalidResponse) {
		return Opening{}, err
	}
	// A malformed observation cannot authorize a second command or a cancellation.
	receipt, lookupErr := lookup()
	if lookupErr != nil {
		return Opening{}, errors.Join(lookupErr, err)
	}
	if receipt.Type == protocol.RunAcceptanceNotRecorded {
		return Opening{}, errors.Join(authoring.ErrCommandOutcomeUnknown, err)
	}
	return recordedOpening(receipt, corresponds)
}

func recordedOpening(receipt protocol.RunAcceptance, corresponds func(protocol.RunAcceptance) error) (Opening, error) {
	if err := corresponds(receipt); err != nil {
		return Opening{}, fmt.Errorf("%w: %w", authoring.ErrCommandOutcomeUnknown, err)
	}
	switch receipt.Type {
	case protocol.RunAcceptanceAccepted:
		if err := errors.Join(protocol.ValidateRunID(receipt.RunID), protocol.ValidateSegmentID(receipt.SegmentID)); err != nil {
			return Opening{}, errors.Join(authoring.ErrCommandOutcomeUnknown, err)
		}
		if receipt.UserItemID != "" {
			if err := protocol.ValidateItemID(receipt.UserItemID); err != nil {
				return Opening{}, errors.Join(authoring.ErrCommandOutcomeUnknown, err)
			}
		}
		return Opening{segment: conversation.SegmentStream{RunID: receipt.RunID, SegmentID: receipt.SegmentID, UserItemID: receipt.UserItemID}}, nil
	case protocol.RunAcceptanceRetired:
		return Opening{}, conversation.ErrOpeningRetired
	default:
		return Opening{}, fmt.Errorf("%w: unknown run acceptance type %q", authoring.ErrCommandOutcomeUnknown, receipt.Type)
	}
}

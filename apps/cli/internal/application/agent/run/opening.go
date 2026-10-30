package run

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type OpeningSource interface {
	GetRunAcceptance(context.Context, protocol.GetRunAcceptanceRequest) (protocol.RunAcceptance, error)
	StartRun(context.Context, authoring.StartRun) (conversation.SegmentStream, error)
	ResumeRun(context.Context, conversation.ResumeRun) (conversation.SegmentStream, error)
}

func openStart(ctx context.Context, source OpeningSource, command authoring.StartRun, guard authoring.ReplayGuard, policy mutation.ReplayPolicy) (conversation.SegmentStream, error) {
	return openStartCommand(ctx, source, command, guard, policy, func() (conversation.SegmentStream, error) { return source.StartRun(ctx, command) })
}

func OpenPendingStart(ctx context.Context, source OpeningSource, store *workbench.Store, pending workbench.PendingRun, policy mutation.ReplayPolicy) (conversation.SegmentStream, error) {
	if store == nil {
		return conversation.SegmentStream{}, workbench.ErrUnavailable
	}
	return openStartCommand(ctx, source, pending.Command, pending.Replay, policy, func() (conversation.SegmentStream, error) {
		if err := store.DispatchReady(); err != nil {
			return conversation.SegmentStream{}, err
		}
		command, err := pending.ReplayCommand()
		if err != nil {
			return conversation.SegmentStream{}, err
		}
		return source.StartRun(ctx, command)
	})
}

func openStartCommand(ctx context.Context, source OpeningSource, command authoring.StartRun, guard authoring.ReplayGuard, policy mutation.ReplayPolicy, dispatch func() (conversation.SegmentStream, error)) (conversation.SegmentStream, error) {
	return openCommand(ctx, source, protocol.GetRunAcceptanceRequest{
		Namespace: guard.Namespace(), Key: string(command.CommandID), Operation: protocol.RunCommandStart,
	}, guard, policy, func(receipt protocol.RunAcceptance) error {
		if receipt.SessionID != command.SessionID || receipt.UserItemID == "" {
			return fmt.Errorf("start receipt does not match the submitted session and opening input")
		}
		return nil
	}, dispatch)
}

func openResume(ctx context.Context, source OpeningSource, command conversation.ResumeRun, guard authoring.ReplayGuard, policy mutation.ReplayPolicy) (conversation.SegmentStream, error) {
	return openResumeCommand(ctx, source, command, guard, policy, func() (conversation.SegmentStream, error) { return source.ResumeRun(ctx, command) })
}

func OpenPendingResume(ctx context.Context, source OpeningSource, store *workbench.Store, pending workbench.PendingResume, policy mutation.ReplayPolicy) (conversation.SegmentStream, error) {
	if store == nil {
		return conversation.SegmentStream{}, workbench.ErrUnavailable
	}
	return openResumeCommand(ctx, source, pending.Command, pending.Replay, policy, func() (conversation.SegmentStream, error) {
		if err := store.DispatchReady(); err != nil {
			return conversation.SegmentStream{}, err
		}
		command, err := pending.ReplayCommand()
		if err != nil {
			return conversation.SegmentStream{}, err
		}
		return source.ResumeRun(ctx, command)
	})
}

func openResumeCommand(ctx context.Context, source OpeningSource, command conversation.ResumeRun, guard authoring.ReplayGuard, policy mutation.ReplayPolicy, dispatch func() (conversation.SegmentStream, error)) (conversation.SegmentStream, error) {
	return openCommand(ctx, source, protocol.GetRunAcceptanceRequest{
		Namespace: guard.Namespace(), Key: string(command.CommandID), Operation: protocol.RunCommandResume,
	}, guard, policy, func(receipt protocol.RunAcceptance) error {
		if receipt.RunID != command.RunID || (receipt.UserItemID != "") != (command.Message != nil) {
			return fmt.Errorf("resume receipt does not match the submitted run and input")
		}
		return nil
	}, dispatch)
}

// A receipt lookup never renews delivery admission. The original Segment is
// historical identity; observation recovers current material through the
// existing snapshot boundary rather than fabricating its lifecycle events.
func openCommand(
	ctx context.Context,
	source OpeningSource,
	query protocol.GetRunAcceptanceRequest,
	guard authoring.ReplayGuard,
	policy mutation.ReplayPolicy,
	corresponds func(protocol.RunAcceptance) error,
	dispatch func() (conversation.SegmentStream, error),
) (conversation.SegmentStream, error) {
	if err := context.Cause(ctx); err != nil {
		return conversation.SegmentStream{}, err
	}
	if !policy.SameStore(guard) {
		return conversation.SegmentStream{}, mutation.ErrReplayGuaranteeUnavailable
	}
	receipt, err := source.GetRunAcceptance(ctx, query)
	if err != nil {
		return conversation.SegmentStream{}, fmt.Errorf("%w: query run acceptance: %w", authoring.ErrCommandOutcomeUnknown, err)
	}
	if receipt.Type == protocol.RunAcceptanceNotRecorded {
		if err := mutation.ReplayAdmission(policy, guard)(); err != nil {
			return conversation.SegmentStream{}, err
		}
		return dispatch()
	}
	if err := corresponds(receipt); err != nil {
		return conversation.SegmentStream{}, fmt.Errorf("%w: %w", authoring.ErrCommandOutcomeUnknown, err)
	}
	stream := conversation.SegmentStream{RunID: receipt.RunID, SegmentID: receipt.SegmentID, UserItemID: receipt.UserItemID}
	switch receipt.Type {
	case protocol.RunAcceptanceAccepted:
		stream.Events = func(yield func(conversation.RunEvent, error) bool) {
			yield(conversation.RunEvent{}, conversation.ErrReplayUnavailable)
		}
		return stream, nil
	case protocol.RunAcceptanceRetired:
		return stream, conversation.NewRetiredOpeningError(stream)
	default:
		return conversation.SegmentStream{}, fmt.Errorf("%w: unknown run acceptance type %q", authoring.ErrCommandOutcomeUnknown, receipt.Type)
	}
}

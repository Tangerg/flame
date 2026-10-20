package run

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

type OpeningSource interface {
	GetRunAcceptance(context.Context, protocol.GetRunAcceptanceRequest) (protocol.RunAcceptance, error)
	StartRun(context.Context, prompt.StartRun) (conversation.SegmentStream, error)
	ResumeRun(context.Context, conversation.ResumeRun) (conversation.SegmentStream, error)
}

func OpenStart(ctx context.Context, source OpeningSource, command prompt.StartRun, guard replay.Guard, policy mutation.ReplayPolicy) (conversation.SegmentStream, error) {
	return openStart(ctx, source, command, guard, policy, func() (conversation.SegmentStream, error) { return source.StartRun(ctx, command) })
}

func OpenPendingStart(ctx context.Context, source OpeningSource, pending workbench.PendingRun, policy mutation.ReplayPolicy) (conversation.SegmentStream, error) {
	return openStart(ctx, source, pending.Command, pending.Replay, policy, func() (conversation.SegmentStream, error) {
		command, err := pending.ReplayCommand()
		if err != nil {
			return conversation.SegmentStream{}, err
		}
		return source.StartRun(ctx, command)
	})
}

func openStart(ctx context.Context, source OpeningSource, command prompt.StartRun, guard replay.Guard, policy mutation.ReplayPolicy, dispatch func() (conversation.SegmentStream, error)) (conversation.SegmentStream, error) {
	return openCommand(ctx, source, protocol.GetRunAcceptanceRequest{
		Namespace: guard.Namespace(), Key: string(command.CommandID), Operation: protocol.RunCommandStart,
	}, guard, policy, func(receipt protocol.RunAcceptance) error {
		if receipt.SessionID != command.SessionID || receipt.UserItemID == "" {
			return fmt.Errorf("start receipt does not match the submitted session and opening input")
		}
		return nil
	}, dispatch)
}

func OpenResume(ctx context.Context, source OpeningSource, command conversation.ResumeRun, guard replay.Guard, policy mutation.ReplayPolicy) (conversation.SegmentStream, error) {
	return openResume(ctx, source, command, guard, policy, func() (conversation.SegmentStream, error) { return source.ResumeRun(ctx, command) })
}

func OpenPendingResume(ctx context.Context, source OpeningSource, pending workbench.PendingResume, policy mutation.ReplayPolicy) (conversation.SegmentStream, error) {
	return openResume(ctx, source, pending.Command, pending.Replay, policy, func() (conversation.SegmentStream, error) {
		command, err := pending.ReplayCommand()
		if err != nil {
			return conversation.SegmentStream{}, err
		}
		return source.ResumeRun(ctx, command)
	})
}

func openResume(ctx context.Context, source OpeningSource, command conversation.ResumeRun, guard replay.Guard, policy mutation.ReplayPolicy, dispatch func() (conversation.SegmentStream, error)) (conversation.SegmentStream, error) {
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
	guard replay.Guard,
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
		return conversation.SegmentStream{}, fmt.Errorf("%w: query run acceptance: %w", replay.ErrCommandOutcomeUnknown, err)
	}
	if receipt.Type == protocol.RunAcceptanceNotRecorded {
		if err := mutation.ReplayAdmission(policy, guard)(); err != nil {
			return conversation.SegmentStream{}, err
		}
		return dispatch()
	}
	if err := corresponds(receipt); err != nil {
		return conversation.SegmentStream{}, fmt.Errorf("%w: %w", replay.ErrCommandOutcomeUnknown, err)
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
		return conversation.SegmentStream{}, fmt.Errorf("%w: unknown run acceptance type %q", replay.ErrCommandOutcomeUnknown, receipt.Type)
	}
}

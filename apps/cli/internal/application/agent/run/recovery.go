package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const sessionAttachAttempts = 8

type RecoverySource interface {
	SessionReader
	SubscribeRun(context.Context, conversation.SubscribeRun) (conversation.SegmentStream, error)
}

type Recovery struct {
	Snapshot conversation.SessionSnapshot
	Run      conversation.Run
	Stream   conversation.SegmentStream
}

func RecoveryRequired(err error) bool {
	return errors.Is(err, conversation.ErrStaleSegment) ||
		errors.Is(err, conversation.ErrRunWaiting) ||
		errors.Is(err, conversation.ErrRunFinished) ||
		errors.Is(err, conversation.ErrReplayCursorInvalid) ||
		errors.Is(err, conversation.ErrReplayUnavailable)
}

func RecoverSegment(ctx context.Context, source RecoverySource, sessionID, runID string) (Recovery, error) {
	first, run, err := read(ctx, source, sessionID, runID)
	if err != nil || run.Status != protocol.RunStatusRunning {
		return Recovery{Snapshot: first, Run: run}, err
	}
	return attach(ctx, source, sessionID, run)
}

func AttachSession(ctx context.Context, source RecoverySource, sessionID string) (Recovery, error) {
	for range sessionAttachAttempts {
		first, err := readSnapshot(ctx, source, sessionID)
		if err != nil {
			return Recovery{}, err
		}
		run, ok := first.ActiveRun()
		if !ok || run.Status != protocol.RunStatusRunning {
			return stateWithoutStream(first), nil
		}

		recovered, err := attach(ctx, source, sessionID, run)
		if err != nil {
			if RecoveryRequired(err) {
				continue
			}
			return Recovery{}, err
		}
		return recovered, nil
	}
	return Recovery{}, fmt.Errorf("%w: session %s did not hold a stable active segment", conversation.ErrStaleSegment, sessionID)
}

func attach(ctx context.Context, source RecoverySource, sessionID string, run conversation.Run) (Recovery, error) {
	streamCtx, release := context.WithCancel(ctx)
	stream, err := source.SubscribeRun(streamCtx, conversation.SubscribeRun{
		SessionID: sessionID, RunID: run.ID, SegmentID: run.ActiveSegmentID, Snapshot: true,
	})
	if err != nil {
		release()
		return Recovery{}, err
	}
	if err := stream.ValidateSubscription(); err != nil {
		release()
		return Recovery{}, fmt.Errorf("recover run: %w", err)
	}
	if stream.Snapshot == nil {
		release()
		return Recovery{}, errors.New("recover run: subscription omitted the requested snapshot")
	}
	snapshot := *stream.Snapshot
	if err := snapshot.Validate(); err != nil {
		release()
		return Recovery{}, fmt.Errorf("recover run: %w", err)
	}
	current, active := snapshot.ActiveRun()
	if snapshot.Session.ID != sessionID || !active || current.Status != protocol.RunStatusRunning ||
		current.ID != run.ID || stream.RunID != current.ID || current.ActiveSegmentID != stream.SegmentID ||
		stream.SegmentID != run.ActiveSegmentID {
		release()
		return Recovery{}, errors.New("recover run: subscription snapshot does not match the requested running root")
	}
	return Recovery{Snapshot: snapshot, Run: current, Stream: releaseWhenDone(stream, release)}, nil
}

func releaseWhenDone(stream conversation.SegmentStream, release context.CancelFunc) conversation.SegmentStream {
	events := stream.Events
	stream.Events = func(yield func(conversation.RunEvent, error) bool) {
		defer release()
		events(yield)
	}
	return stream
}

func stateWithoutStream(snapshot conversation.SessionSnapshot) Recovery {
	run, ok := snapshot.ActiveRun()
	if !ok {
		run, _ = snapshot.LatestRun()
	}
	return Recovery{Snapshot: snapshot, Run: run}
}

func read(ctx context.Context, source SessionReader, sessionID, runID string) (conversation.SessionSnapshot, conversation.Run, error) {
	snapshot, err := readSnapshot(ctx, source, sessionID)
	if err != nil {
		return conversation.SessionSnapshot{}, conversation.Run{}, err
	}
	run, ok := snapshot.RunByID(runID)
	if !ok {
		return conversation.SessionSnapshot{}, conversation.Run{}, fmt.Errorf("%w: %s", conversation.ErrRunNotFound, runID)
	}
	return snapshot, run, nil
}

func readSnapshot(ctx context.Context, source SessionReader, sessionID string) (conversation.SessionSnapshot, error) {
	snapshot, err := source.GetSession(ctx, sessionID)
	if err != nil {
		return conversation.SessionSnapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return conversation.SessionSnapshot{}, fmt.Errorf("recover run: %w", err)
	}
	return snapshot, nil
}

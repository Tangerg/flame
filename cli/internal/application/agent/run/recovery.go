package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

const sessionAttachAttempts = 8

// RecoverySource is the narrow runtime surface needed for cold recovery.
type RecoverySource interface {
	SessionReader
	SubscribeRun(context.Context, agent.SubscribeRun) (agent.SegmentStream, error)
}

// Recovery is a coherent cold projection and, while its run is still executing,
// a stream attached before the final read. Stream is empty for waiting and
// finished runs.
type Recovery struct {
	Snapshot agent.SessionSnapshot
	Run      agent.Run
	Stream   agent.SegmentStream
}

// RecoveryRequired reports whether a failed segment subscription must be reconciled
// from durable reads instead of retried with the same cursor.
func RecoveryRequired(err error) bool {
	return errors.Is(err, agent.ErrStaleSegment) ||
		errors.Is(err, agent.ErrRunWaiting) ||
		errors.Is(err, agent.ErrRunFinished) ||
		errors.Is(err, agent.ErrReplayCursorInvalid) ||
		errors.Is(err, agent.ErrReplayUnavailable)
}

// RecoverSegment follows the runtime's attach-then-read rule. For a live run it first
// attaches at the current segment head and only then performs the durable read,
// preventing an unobserved gap between the snapshot and later stream events.
func RecoverSegment(ctx context.Context, source RecoverySource, sessionID, runID string) (Recovery, error) {
	first, run, err := read(ctx, source, sessionID, runID)
	if err != nil || run.Status != agent.RunStatusRunning {
		return Recovery{Snapshot: first, Run: run}, err
	}
	stream, release, err := attach(ctx, source, run)
	if err != nil {
		return Recovery{}, err
	}
	second, current, err := read(ctx, source, sessionID, runID)
	if err != nil {
		release()
		return Recovery{}, err
	}
	if current.Status != agent.RunStatusRunning {
		release()
		return Recovery{Snapshot: second, Run: current}, nil
	}
	if current.ActiveSegmentID != stream.SegmentID {
		release()
		return Recovery{}, fmt.Errorf("%w: run %s changed from segment %s to %s during recovery", agent.ErrStaleSegment, runID, stream.SegmentID, current.ActiveSegmentID)
	}
	return Recovery{Snapshot: second, Run: current, Stream: releaseWhenDone(stream, release)}, nil
}

// AttachSession obtains a coherent session projection and, when its current
// root Run is executing, a cursorless tail that was attached before the final
// cold read. It retries when another client crosses a Run or Segment boundary
// during that handshake.
func AttachSession(ctx context.Context, source RecoverySource, sessionID string) (Recovery, error) {
	for range sessionAttachAttempts {
		first, err := readSnapshot(ctx, source, sessionID)
		if err != nil {
			return Recovery{}, err
		}
		run, ok := first.ActiveRun()
		if !ok || run.Status != agent.RunStatusRunning {
			return stateWithoutStream(first), nil
		}

		stream, release, err := attach(ctx, source, run)
		if err != nil {
			if RecoveryRequired(err) {
				continue
			}
			return Recovery{}, err
		}
		second, err := readSnapshot(ctx, source, sessionID)
		if err != nil {
			release()
			return Recovery{}, err
		}
		current, active := second.ActiveRun()
		if !active || current.Status != agent.RunStatusRunning {
			release()
			return stateWithoutStream(second), nil
		}
		if current.ID != stream.RunID || current.ActiveSegmentID != stream.SegmentID {
			release()
			continue
		}
		return Recovery{
			Snapshot: second,
			Run:      current,
			Stream:   releaseWhenDone(stream, release),
		}, nil
	}
	return Recovery{}, fmt.Errorf("%w: session %s did not hold a stable active segment", agent.ErrStaleSegment, sessionID)
}

func attach(ctx context.Context, source RecoverySource, run agent.Run) (agent.SegmentStream, context.CancelFunc, error) {
	streamCtx, release := context.WithCancel(ctx)
	stream, err := source.SubscribeRun(streamCtx, agent.SubscribeRun{RunID: run.ID, SegmentID: run.ActiveSegmentID})
	if err != nil {
		release()
		return agent.SegmentStream{}, nil, err
	}
	if err := stream.ValidateSubscription(); err != nil {
		release()
		return agent.SegmentStream{}, nil, fmt.Errorf("recover run: %w", err)
	}
	return stream, release, nil
}

func releaseWhenDone(stream agent.SegmentStream, release context.CancelFunc) agent.SegmentStream {
	events := stream.Events
	stream.Events = func(yield func(agent.RunEvent, error) bool) {
		defer release()
		events(yield)
	}
	return stream
}

func stateWithoutStream(snapshot agent.SessionSnapshot) Recovery {
	run, ok := snapshot.ActiveRun()
	if !ok {
		run, _ = snapshot.LatestRun()
	}
	return Recovery{Snapshot: snapshot, Run: run}
}

func read(ctx context.Context, source SessionReader, sessionID, runID string) (agent.SessionSnapshot, agent.Run, error) {
	snapshot, err := readSnapshot(ctx, source, sessionID)
	if err != nil {
		return agent.SessionSnapshot{}, agent.Run{}, err
	}
	run, ok := snapshot.RunByID(runID)
	if !ok {
		return agent.SessionSnapshot{}, agent.Run{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, runID)
	}
	return snapshot, run, nil
}

func readSnapshot(ctx context.Context, source SessionReader, sessionID string) (agent.SessionSnapshot, error) {
	snapshot, err := source.GetSession(ctx, sessionID)
	if err != nil {
		return agent.SessionSnapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return agent.SessionSnapshot{}, fmt.Errorf("recover run: %w", err)
	}
	return snapshot, nil
}

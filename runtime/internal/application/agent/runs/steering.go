package runs

import (
	"context"
	"errors"
	"fmt"

	corechat "github.com/Tangerg/scope/core/chat"
)

// Steer addresses the Segment the command names through execution control.
//
// It resolves through the same authority a subscribe does
// ([Coordinator.addressLiveSegment]), so "this run is waiting" or "that segment
// has been replaced" is one answer with one spelling rather than two entry
// points each guessing from the live registry.
func (c *Coordinator) Steer(ctx context.Context, cmd SteerCommand) (string, error) {
	cmd = cmd.clone()
	live, _, err := c.addressLiveSegment(ctx, cmd.RunID, cmd.ExpectedSegmentID)
	if err != nil {
		return "", err
	}
	// A resumed Run is durably Running once its acceptance opening commits, before
	// its executor activates. Wait on that Segment's activation gate so a steer into
	// the window is not misread as a finished Run, then re-resolve the exact Segment
	// and owner: activation may have terminalized the Run or replaced its Segment.
	if err := live.owner.awaitActivation(ctx); err != nil {
		return "", err
	}
	live, durable, err := c.addressLiveSegment(ctx, cmd.RunID, cmd.ExpectedSegmentID)
	if err != nil {
		return "", err
	}
	rec := live.record
	message, err := MaterializeUserMessage(cmd.Input)
	if err != nil {
		return "", err
	}
	if admitErr := c.models.AdmitInput(durable.ModelSelection(), []corechat.Message{message}); admitErr != nil {
		return "", fmt.Errorf("%w: %w", ErrUnsupportedMedia, admitErr)
	}
	itemID, err := c.steering.SubmitSteer(ctx, ExecutorRef{SessionID: durable.SessionID(), ExecutorID: rec.ExecutorID}, cmd.Input)
	if err != nil {
		if errors.Is(err, ErrExecutorNotLive) {
			// Execution ended between resolving the record and delivering: the Run is
			// finishing, which is the same thing the durable record would say a moment
			// from now.
			return "", fmt.Errorf("%w: %w", ErrRunFinished, err)
		}
		return "", err
	}
	return itemID, nil
}

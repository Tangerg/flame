package runs

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/google/uuid"
)

type PreparedSteer struct {
	ItemID     string
	Input      []transcript.ContentBlock
	Acceptance *CommandAcceptance
}

func (p PreparedSteer) Clone() PreparedSteer {
	p.Input = transcript.CloneContent(p.Input)
	if p.Acceptance != nil {
		p.Acceptance = new(p.Acceptance.Clone())
	}
	return p
}

// Steer addresses the Segment the command names through execution control.
//
// It resolves through the same authority a subscribe does
// ([Coordinator.addressLiveSegment]), so "this run is waiting" or "that segment
// has been replaced" is one answer with one spelling rather than two entry
// points each guessing from the live registry.
func (c *Coordinator) Steer(ctx context.Context, cmd SteerCommand) (string, error) {
	cmd = cmd.clone()
	if cmd.Acceptance != nil {
		if err := cmd.Acceptance.Validate(); err != nil {
			return "", err
		}
		record, err := c.commandAcceptances.Lookup(ctx, cmd.Acceptance.Key)
		if err != nil {
			return "", err
		}
		if record.OtherOperation || record.Found && (record.Acceptance.Operation != CommandSteerInput || record.Acceptance.Fingerprint != cmd.Acceptance.Fingerprint) {
			return "", ErrCommandKeyConflict
		}
		if record.Found {
			return record.Acceptance.UserItemID, nil
		}
	}
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
	prepared := PreparedSteer{ItemID: NewItemID(uuid.NewString()), Input: cmd.Input}
	if cmd.Acceptance != nil {
		prepared.ItemID = NewItemID(fmt.Sprintf("%x", sha256.Sum256([]byte("steer:"+cmd.Acceptance.Key))))
		prepared.Acceptance = &CommandAcceptance{Key: cmd.Acceptance.Key, Fingerprint: cmd.Acceptance.Fingerprint, Operation: CommandSteerInput, SessionID: durable.SessionID(), RunID: durable.ID(), SegmentID: cmd.ExpectedSegmentID, UserItemID: prepared.ItemID, SteerInput: transcript.CloneContent(cmd.Input)}
	}
	itemID, err := c.steering.SubmitSteer(ctx, ExecutorRef{SessionID: durable.SessionID(), ExecutorID: rec.ExecutorID}, prepared)
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

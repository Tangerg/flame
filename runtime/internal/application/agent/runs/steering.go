package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type PreparedSteer struct {
	itemID  identity.ItemID
	input   []transcript.ContentBlock
	command *SteerCommandBinding
}

type SteerCommandBinding struct {
	CommandIdentity
	SessionID identity.SessionID
	RunID     identity.RunID
	SegmentID identity.SegmentID
}

func PrepareLocalSteer(itemID identity.ItemID, input []transcript.ContentBlock) (PreparedSteer, error) {
	if err := itemID.Validate(); err != nil {
		return PreparedSteer{}, err
	}
	if _, err := MaterializeUserMessage(input); err != nil {
		return PreparedSteer{}, err
	}
	return PreparedSteer{itemID: itemID, input: transcript.CloneContent(input)}, nil
}

func (b SteerCommandBinding) Prepare(input []transcript.ContentBlock) (PreparedSteer, error) {
	itemID := SteerItemID(b.Key)
	if err := b.acceptance(itemID, input).Validate(); err != nil {
		return PreparedSteer{}, err
	}
	return PreparedSteer{itemID: itemID, input: transcript.CloneContent(input), command: &b}, nil
}

func (p PreparedSteer) ItemID() identity.ItemID { return p.itemID }

func (p PreparedSteer) Input() []transcript.ContentBlock { return transcript.CloneContent(p.input) }

func (p PreparedSteer) Command() (SteerCommandBinding, bool) {
	if p.command == nil {
		return SteerCommandBinding{}, false
	}
	return *p.command, true
}

func (p PreparedSteer) Acceptance() (CommandAcceptance, bool) {
	if p.command == nil {
		return CommandAcceptance{}, false
	}
	return p.command.acceptance(p.itemID, p.Input()), true
}

func (b SteerCommandBinding) acceptance(itemID identity.ItemID, input []transcript.ContentBlock) CommandAcceptance {
	return CommandAcceptance{
		Key: b.Key, Fingerprint: b.Fingerprint, Operation: CommandSteerInput,
		SessionID: b.SessionID, RunID: b.RunID, SegmentID: b.SegmentID, UserItemID: itemID, SteerInput: input,
	}
}

func (p PreparedSteer) Equal(other PreparedSteer) bool {
	if p.itemID != other.itemID || !slices.EqualFunc(p.input, other.input, transcript.ContentBlock.Equal) {
		return false
	}
	left, leftBound := p.Command()
	right, rightBound := other.Command()
	return leftBound == rightBound && left == right
}

func (c *Coordinator) Steer(ctx context.Context, cmd SteerCommand) (identity.ItemID, error) {
	cmd = cmd.clone()
	if cmd.Identity != nil {
		if err := cmd.Identity.Validate(); err != nil {
			return identity.ItemID{}, err
		}
		record, err := c.commandAcceptances.Lookup(ctx, cmd.Identity.Key)
		if err != nil {
			return identity.ItemID{}, err
		}
		if record.Conflicts(CommandSteerInput, cmd.Identity.Fingerprint) {
			return identity.ItemID{}, ErrCommandKeyConflict
		}
		if record.Use == CommandKeyAccepted {
			return record.Acceptance.UserItemID, nil
		}
	}
	live, _, err := c.segments.addressLiveSegment(ctx, cmd.RunID, cmd.ExpectedSegmentID)
	if err != nil {
		return identity.ItemID{}, err
	}
	if err := live.owner.awaitActivation(ctx); err != nil {
		return identity.ItemID{}, err
	}
	live, durable, err := c.segments.addressLiveSegment(ctx, cmd.RunID, cmd.ExpectedSegmentID)
	if err != nil {
		return identity.ItemID{}, err
	}
	rec := live.record
	var prepared PreparedSteer
	if cmd.Identity == nil {
		prepared, err = PrepareLocalSteer(c.newItemID(), cmd.Input)
	} else {
		binding := SteerCommandBinding{
			CommandIdentity: *cmd.Identity,
			SessionID:       durable.SessionID(), RunID: durable.ID(), SegmentID: cmd.ExpectedSegmentID,
		}
		prepared, err = binding.Prepare(cmd.Input)
	}
	if err != nil {
		return identity.ItemID{}, err
	}
	itemID, err := c.steering.SubmitSteer(ctx, ExecutorRef{SessionID: durable.SessionID(), ExecutorID: rec.ExecutorID}, prepared)
	if err != nil {
		if errors.Is(err, ErrExecutorNotLive) {
			return identity.ItemID{}, fmt.Errorf("%w: %w", ErrRunFinished, err)
		}
		return identity.ItemID{}, err
	}
	return itemID, nil
}

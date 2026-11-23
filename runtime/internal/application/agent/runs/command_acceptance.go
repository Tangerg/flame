package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	framevalue "github.com/Tangerg/frame/value"
)

var ErrCommandKeyConflict = errors.New("runs: idempotency key is already bound to another command")

const goalRoundKeyPrefix = "flame.goal-round/"

func ReservedCommandKey(key string) bool { return strings.HasPrefix(key, goalRoundKeyPrefix) }

func GoalRoundIdentity(sessionID flameidentity.SessionID, incarnation goalref.IncarnationID, ordinal uint64, instructions string) *CommandIdentity {
	scope := sessionID.String() + "\x00" + incarnation.String() + "\x00" + strconv.FormatUint(ordinal, 10)
	return &CommandIdentity{Key: goalRoundKeyPrefix + framevalue.Hash([]byte(scope)).String(), Fingerprint: framevalue.Hash([]byte(scope + "\x00" + instructions)).String()}
}

func startOperation(key string, goalIncarnation goalref.IncarnationID) CommandOperation {
	if ReservedCommandKey(key) && !goalIncarnation.IsZero() {
		return CommandGoalRoundOpening
	}
	return CommandStartOpening
}

type CommandOperation string

const (
	CommandStartOpening     CommandOperation = "start"
	CommandGoalRoundOpening CommandOperation = "goalRound"
	CommandResumeOpening    CommandOperation = "resume"
	CommandSteerInput       CommandOperation = "steer"
)

func (o CommandOperation) Valid() bool {
	return o == CommandStartOpening || o == CommandGoalRoundOpening || o == CommandResumeOpening || o == CommandSteerInput
}

type CommandAcceptance struct {
	Key         string
	Fingerprint string
	Operation   CommandOperation
	SessionID   flameidentity.SessionID
	RunID       flameidentity.RunID
	SegmentID   flameidentity.SegmentID
	UserItemID  flameidentity.ItemID
	SteerInput  []transcript.ContentBlock
}

func (a CommandAcceptance) Validate() error {
	if a.Key == "" {
		return errors.New("runs: command acceptance has no idempotency key")
	}
	if _, err := framevalue.ParseDigest(a.Fingerprint); err != nil {
		return fmt.Errorf("runs: command acceptance fingerprint: %w", err)
	}
	if ReservedCommandKey(a.Key) != (a.Operation == CommandGoalRoundOpening) {
		return errors.New("runs: command key cannot claim another opening namespace")
	}
	if a.Operation == CommandGoalRoundOpening && !a.UserItemID.IsZero() {
		return errors.New("runs: autonomous opening cannot claim a user Item")
	}
	if !a.Operation.Valid() {
		return fmt.Errorf("runs: command acceptance has unknown operation %q", a.Operation)
	}
	if err := errors.Join(a.SessionID.Validate(), a.RunID.Validate(), a.SegmentID.Validate()); err != nil {
		return err
	}
	if a.Operation == CommandStartOpening || a.Operation == CommandSteerInput {
		if err := a.UserItemID.Validate(); err != nil {
			return err
		}
	}
	if a.Operation == CommandSteerInput {
		_, err := MaterializeUserMessage(a.SteerInput)
		return err
	}
	if len(a.SteerInput) != 0 {
		return errors.New("runs: opening acceptance contains steer input")
	}
	return nil
}

func (a CommandAcceptance) Clone() CommandAcceptance {
	a.SteerInput = transcript.CloneContent(a.SteerInput)
	return a
}

func (a CommandAcceptance) Equal(b CommandAcceptance) bool {
	return a.Key == b.Key && a.Fingerprint == b.Fingerprint && a.Operation == b.Operation && a.SessionID == b.SessionID && a.RunID == b.RunID && a.SegmentID == b.SegmentID && a.UserItemID == b.UserItemID && slices.EqualFunc(a.SteerInput, b.SteerInput, transcript.ContentBlock.Equal)
}

type CommandIdentity struct {
	Key         string
	Fingerprint string
}

func (c CommandIdentity) Validate() error {
	if c.Key == "" {
		return errors.New("runs: command identity has no idempotency key")
	}
	if _, err := framevalue.ParseDigest(c.Fingerprint); err != nil {
		return fmt.Errorf("runs: command identity fingerprint: %w", err)
	}
	return nil
}

type CommandKeyUse uint8

const (
	CommandKeyUnused CommandKeyUse = iota
	CommandKeyAccepted
	CommandKeyForeign
)

type CommandAcceptanceRecord struct {
	Use         CommandKeyUse
	Acceptance  CommandAcceptance
	CurrentRun  *run.Run
	AppliedItem *transcript.Item
}

func (r CommandAcceptanceRecord) Conflicts(operation CommandOperation, fingerprint string) bool {
	switch r.Use {
	case CommandKeyForeign:
		return true
	case CommandKeyAccepted:
		return r.Acceptance.Operation != operation || r.Acceptance.Fingerprint != fingerprint
	default:
		return false
	}
}

type commandAcceptanceReader interface {
	Lookup(ctx context.Context, key string) (CommandAcceptanceRecord, error)
}

func (c *Coordinator) LookupCommandAcceptance(ctx context.Context, key string) (CommandAcceptanceRecord, error) {
	if key == "" {
		return CommandAcceptanceRecord{}, nil
	}
	return c.commandAcceptances.Lookup(ctx, key)
}

func AcceptCommand(key, requestFingerprint string, opening OpeningCommit) (CommandAcceptance, error) {
	acceptance := CommandAcceptance{
		Key:         key,
		Fingerprint: requestFingerprint,
	}
	if admission, present := opening.Admission(); present {
		acceptance.Operation = startOperation(key, admission.GoalIncarnationID)
	} else if _, present := opening.Resume(); present {
		acceptance.Operation = CommandResumeOpening
	} else {
		return CommandAcceptance{}, errors.New("runs: command acceptance has no opening")
	}
	sessionID, runID, segmentID, err := opening.OwnerIdentity()
	if err != nil {
		return CommandAcceptance{}, err
	}
	acceptance.SessionID, acceptance.RunID, acceptance.SegmentID = sessionID, runID, segmentID
	userItemID, err := opening.UserInputItemID()
	if err != nil {
		return CommandAcceptance{}, err
	}
	acceptance.UserItemID = userItemID
	if err := acceptance.Validate(); err != nil {
		return CommandAcceptance{}, err
	}
	return acceptance, nil
}

func lookupAcceptedCommand(ctx context.Context, acceptances commandAcceptanceReader, identity CommandIdentity, operation CommandOperation) (StartResult, bool, error) {
	record, err := acceptances.Lookup(ctx, identity.Key)
	if err != nil {
		return StartResult{}, false, err
	}
	if record.Conflicts(operation, identity.Fingerprint) {
		return StartResult{}, false, ErrCommandKeyConflict
	}
	if record.Use != CommandKeyAccepted {
		return StartResult{}, false, nil
	}
	accepted := record.Acceptance
	return StartResult{
		RunID: accepted.RunID, SegmentID: accepted.SegmentID, SessionID: accepted.SessionID,
		UserItemID: accepted.UserItemID, Opening: reopening(record),
	}, true, nil
}

func reopening(record CommandAcceptanceRecord) Opening {
	if record.CurrentRun == nil {
		return OpeningRetired
	}
	return OpeningReopened
}

package runs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/resourceid"
)

// ErrCommandKeyConflict reports that a command's idempotency key is already
// bound to a different request or to another operation entirely.
var ErrCommandKeyConflict = errors.New("runs: idempotency key is already bound to another command")

// CommandOperation names a Run-owned external command whose acceptance commits
// with its opening or input-admission boundary.
type CommandOperation string

const (
	// CommandStartOpening is a fresh root admission accepted by runs.start.
	CommandStartOpening  CommandOperation = "start"
	CommandResumeOpening CommandOperation = "resume"
	CommandSteerInput    CommandOperation = "steer"
)

// Valid reports whether o is an operation whose acceptance this build records.
func (o CommandOperation) Valid() bool {
	return o == CommandStartOpening || o == CommandResumeOpening || o == CommandSteerInput
}

// CommandAcceptance binds one external command to its original product references.
// Start and Resume commit it with their opening; Steer commits it with the Scope
// Signal head and pending input context. SteerInput is immutable authored content,
// not an application status. Only the exact completed User Item proves application.
// The store owns the namespace and one global key; Fingerprint pins request semantics.
type CommandAcceptance struct {
	Key         string
	Fingerprint string
	Operation   CommandOperation
	SessionID   string
	RunID       string
	SegmentID   string
	UserItemID  string
	SteerInput  []transcript.ContentBlock
}

// Validate proves the acceptance names a legal operation and complete command
// identity. It does not reach storage: the record's primary key enforces the one
// global key constraint, and the material reference is a storage-boundary fact.
func (a CommandAcceptance) Validate() error {
	if a.Key == "" {
		return errors.New("runs: command acceptance has no idempotency key")
	}
	if _, err := fingerprint.ParseDigest(a.Fingerprint); err != nil {
		return fmt.Errorf("runs: command acceptance fingerprint: %w", err)
	}
	if !a.Operation.Valid() {
		return fmt.Errorf("runs: command acceptance has unknown operation %q", a.Operation)
	}
	if err := resourceid.ValidateSession(a.SessionID); err != nil {
		return err
	}
	if err := resourceid.ValidateRun(a.RunID); err != nil {
		return err
	}
	if err := resourceid.ValidateSegment(a.SegmentID); err != nil {
		return err
	}
	if a.UserItemID != "" || a.Operation == CommandStartOpening || a.Operation == CommandSteerInput {
		if err := resourceid.ValidateItem(a.UserItemID); err != nil {
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
	return a.Key == b.Key && a.Fingerprint == b.Fingerprint && a.Operation == b.Operation && a.SessionID == b.SessionID && a.RunID == b.RunID && a.SegmentID == b.SegmentID && a.UserItemID == b.UserItemID && slices.EqualFunc(a.SteerInput, b.SteerInput, func(x, y transcript.ContentBlock) bool {
		return x.Kind == y.Kind && x.Text == y.Text && x.MediaType == y.MediaType && bytes.Equal(x.Bytes, y.Bytes)
	})
}

// CommandIdentity is the external command reference the delivery boundary binds
// to a Run command before admission: the global idempotency key and the request
// fingerprint that pins the exact accepted request. The namespace is the store's
// own identity, validated at the delivery boundary.
type CommandIdentity struct {
	Key         string
	Fingerprint string
}

// Validate reports whether the identity can address and pin one command.
func (c CommandIdentity) Validate() error {
	if c.Key == "" {
		return errors.New("runs: command identity has no idempotency key")
	}
	if _, err := fingerprint.ParseDigest(c.Fingerprint); err != nil {
		return fmt.Errorf("runs: command identity fingerprint: %w", err)
	}
	return nil
}

// CommandAcceptanceRecord is the read-only projection of a command-record lookup
// by key. At most one of Found or OtherOperation is true; neither means the key
// is unused. CurrentRun is read consistently through the original material
// reference; deleting and importing the same Run ID cannot restore that reference.
type CommandAcceptanceRecord struct {
	Acceptance     CommandAcceptance
	Found          bool
	CurrentRun     *run.Run
	OtherOperation bool
	AppliedItem    *transcript.Item
}

// CommandAcceptanceReader observes original command facts without admitting,
// completing, or repairing a record.
type CommandAcceptanceReader interface {
	Lookup(ctx context.Context, key string) (CommandAcceptanceRecord, error)
}

// LookupCommandAcceptance reads the immutable acceptance and original material.
// An absent key is a not-recorded observation, not a failure or dispatch authority.
func (c *Coordinator) LookupCommandAcceptance(ctx context.Context, key string) (CommandAcceptanceRecord, error) {
	if key == "" {
		return CommandAcceptanceRecord{}, nil
	}
	return c.commandAcceptances.Lookup(ctx, key)
}

// AcceptCommand derives the acceptance for key and fingerprint from the opening
// that an admission produced, so the two cannot describe different Runs.
func AcceptCommand(key, requestFingerprint string, opening OpeningCommit) (CommandAcceptance, error) {
	acceptance := CommandAcceptance{
		Key:         key,
		Fingerprint: requestFingerprint,
	}
	if _, present := opening.Admission(); present {
		acceptance.Operation = CommandStartOpening
	} else if _, present := opening.Resume(); present {
		acceptance.Operation = CommandResumeOpening
	} else {
		return CommandAcceptance{}, errors.New("runs: command acceptance has no opening")
	}
	sessionID, runID, segmentID, err := opening.ownerIdentity()
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

func (c *Coordinator) lookupAcceptedCommand(ctx context.Context, identity CommandIdentity, operation CommandOperation) (StartResult, bool, error) {
	record, err := c.commandAcceptances.Lookup(ctx, identity.Key)
	if err != nil {
		return StartResult{}, false, err
	}
	if record.OtherOperation {
		return StartResult{}, false, ErrCommandKeyConflict
	}
	if !record.Found {
		return StartResult{}, false, nil
	}
	accepted := record.Acceptance
	if accepted.Fingerprint != identity.Fingerprint || accepted.Operation != operation {
		return StartResult{}, false, ErrCommandKeyConflict
	}
	return StartResult{
		RunID: accepted.RunID, SegmentID: accepted.SegmentID, SessionID: accepted.SessionID,
		UserItemID: accepted.UserItemID, Reopened: true, MaterialRetired: record.CurrentRun == nil,
	}, true, nil
}

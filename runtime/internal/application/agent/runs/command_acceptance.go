package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

// ErrCommandKeyConflict reports that a command's idempotency key is already
// bound to a different request or to another operation entirely.
var ErrCommandKeyConflict = errors.New("runs: idempotency key is already bound to another command")

// CommandOperation names the externally initiated Run command whose acceptance
// an opening transaction records. It is a closed value so a record cannot carry
// an operation this build does not open.
type CommandOperation string

const (
	// CommandStartOpening is a fresh root admission accepted by runs.start.
	CommandStartOpening  CommandOperation = "start"
	CommandResumeOpening CommandOperation = "resume"
)

// Valid reports whether o is an operation whose acceptance this build records.
func (o CommandOperation) Valid() bool { return o == CommandStartOpening || o == CommandResumeOpening }

// CommandAcceptance is the immutable fact that Runtime accepted one externally
// initiated Run command and bound it to the exact opening it produced. It is the
// external-command peer of [AutomationStart]'s acceptance: the acceptance use
// case commits it in the same transaction as the Run opening, so a lost response
// never splits "this command was accepted" from "this Run was opened".
//
// Key is the caller's original idempotency key within the store's own namespace;
// the namespace is the store's identity, validated at the delivery boundary.
// The fingerprint pins the exact accepted request semantics. Operation, Session,
// Run, Segment and UserItem are the owner-provided references the opening wrote.
// It carries no Run status, completion time, or model output: those are projected
// from their own owners at lookup.
type CommandAcceptance struct {
	Key         string
	Fingerprint string
	Operation   CommandOperation
	SessionID   string
	RunID       string
	SegmentID   string
	UserItemID  string
}

// Validate proves the acceptance names a legal operation and complete opening
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
	if a.UserItemID != "" || a.Operation == CommandStartOpening {
		return resourceid.ValidateItem(a.UserItemID)
	}
	return nil
}

// CommandIdentity is the external command reference the delivery boundary binds
// to a Start before admission: the global idempotency key and the request
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
}

// CommandAcceptanceReader resolves a previously accepted command by its key so a
// retry returns the original opening rather than admitting a second Run. It only
// reads owner-provided facts; it never admits, completes, or repairs a record.
type CommandAcceptanceReader interface {
	Lookup(ctx context.Context, key string) (CommandAcceptanceRecord, error)
}

// LookupCommandAcceptance reports what an opening recorded for one external
// command key. It is the read peer of the acceptance the opening transaction owns:
// a client that lost a start ack resolves the key here instead of opening a second
// Run. An absent key is the lookup's own not-recorded answer, not a failure.
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
	if admission, present := opening.Admission(); present {
		acceptance.Operation = CommandStartOpening
		acceptance.SessionID, acceptance.RunID, acceptance.SegmentID = admission.SessionID, admission.RunID, admission.SegmentID
	} else if resume, present := opening.Resume(); present {
		root := resume.Runs[len(resume.Runs)-1].State()
		acceptance.Operation = CommandResumeOpening
		acceptance.SessionID, acceptance.RunID, acceptance.SegmentID = root.SessionID(), root.ID(), root.ActiveSegmentID()
	} else {
		return CommandAcceptance{}, errors.New("runs: command acceptance has no opening")
	}
	for _, event := range opening.events {
		for _, item := range event.Items {
			if item.Kind() != transcript.UserMessage || item.RunID() != acceptance.RunID {
				continue
			}
			if acceptance.UserItemID != "" {
				return CommandAcceptance{}, errors.New("runs: command opening repeats its user input")
			}
			acceptance.UserItemID = item.ID()
		}
	}
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

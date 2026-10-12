package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
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
	// CommandStartOpening is a fresh root/child admission accepted by runs.start.
	CommandStartOpening CommandOperation = "start"
)

// Valid reports whether o is an operation whose acceptance this build records.
func (o CommandOperation) Valid() bool { return o == CommandStartOpening }

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
	return resourceid.ValidateItem(a.UserItemID)
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
// is unused. MaterialAvailable reports whether the original Run material is still
// referenced by the acceptance.
type CommandAcceptanceRecord struct {
	Acceptance        CommandAcceptance
	Found             bool
	MaterialAvailable bool
	OtherOperation    bool
}

// CommandAcceptanceReader resolves a previously accepted command by its key so a
// retry returns the original opening rather than admitting a second Run. It only
// reads owner-provided facts; it never admits, completes, or repairs a record.
type CommandAcceptanceReader interface {
	Lookup(ctx context.Context, key string) (CommandAcceptanceRecord, error)
}

// AcceptCommand derives the acceptance for key and fingerprint from the opening
// that an admission produced, so the two cannot describe different Runs.
func AcceptCommand(key, requestFingerprint string, opening OpeningCommit, userItemID string) (CommandAcceptance, error) {
	admission, present := opening.Admission()
	if !present {
		return CommandAcceptance{}, errors.New("runs: command acceptance has no Run admission")
	}
	acceptance := CommandAcceptance{
		Key:         key,
		Fingerprint: requestFingerprint,
		Operation:   CommandStartOpening,
		SessionID:   admission.SessionID,
		RunID:       admission.RunID,
		SegmentID:   admission.SegmentID,
		UserItemID:  userItemID,
	}
	if err := acceptance.Validate(); err != nil {
		return CommandAcceptance{}, err
	}
	return acceptance, nil
}

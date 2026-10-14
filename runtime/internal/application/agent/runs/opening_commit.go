package runs

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/optional"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// OpeningCommit is the atomic acceptance write-set for one fresh admission or
// one continuation.
type OpeningCommit struct {
	commitID           runtimeidentity.CommitID
	admit              *run.Draft
	resume             *run.TreeResumeDraft
	resumeAcceptance   *ResumeAcceptance
	initialSession     *session.Session
	sessionReplacement *session.Replacement
	automation         *AutomationStart
	commandAcceptance  *CommandAcceptance
	events             []EventCommit
}

// NewAdmissionOpeningCommit binds a fresh Run admission to every root-owned
// Session or Automation fact and opening projection written in its transaction.
func NewAdmissionOpeningCommit(
	commitID runtimeidentity.CommitID,
	admit run.Draft,
	initialSession *session.Session,
	sessionReplacement *session.Replacement,
	automation *AutomationStart,
	events []EventCommit,
) (OpeningCommit, error) {
	admit = cloneOpeningAdmission(admit)
	opening := OpeningCommit{commitID: commitID, admit: &admit, events: cloneEventCommits(events)}
	opening.initialSession = optional.Clone(initialSession)
	opening.sessionReplacement = optional.Clone(sessionReplacement)
	opening.automation = optional.Clone(automation)
	if err := opening.Validate(); err != nil {
		return OpeningCommit{}, err
	}
	return opening, nil
}

// WithCommandAcceptance binds an externally initiated command's immutable
// acceptance to this opening, so the acceptance record commits
// in the same transaction as the Run it names. It returns a new validated value
// and never mutates the receiver.
func (o OpeningCommit) WithCommandAcceptance(acceptance CommandAcceptance) (OpeningCommit, error) {
	next := o
	next.commandAcceptance = &acceptance
	if err := next.Validate(); err != nil {
		return OpeningCommit{}, err
	}
	return next, nil
}

// NewResumeOpeningCommit binds one complete tree continuation to the opening
// projections written in the same transaction.
func NewResumeOpeningCommit(
	commitID runtimeidentity.CommitID,
	resume run.TreeResumeDraft,
	events []EventCommit,
) (OpeningCommit, error) {
	resume = cloneOpeningResume(resume)
	opening := OpeningCommit{
		commitID: commitID, resume: &resume, events: cloneEventCommits(events),
	}
	if err := opening.Validate(); err != nil {
		return OpeningCommit{}, err
	}
	return opening, nil
}

// WithResumeAcceptance binds the answer write-set a continuation opening accepts
// in the same transaction: the consumed open interrupt set and its settled Item
// replacements commit with the resumed tree. It returns a new validated value and
// never mutates the receiver.
func (o OpeningCommit) WithResumeAcceptance(acceptance ResumeAcceptance) (OpeningCommit, error) {
	if o.resume == nil {
		return OpeningCommit{}, errors.New("runs: resume acceptance requires a tree continuation")
	}
	next := o
	next.resumeAcceptance = &acceptance
	if err := next.Validate(); err != nil {
		return OpeningCommit{}, err
	}
	return next, nil
}

func cloneOpeningAdmission(admit run.Draft) run.Draft {
	admit.Capabilities = admit.Capabilities.Clone()
	return admit
}

func cloneOpeningResume(resume run.TreeResumeDraft) run.TreeResumeDraft {
	resume.Runs = slices.Clone(resume.Runs)
	return resume
}

// Validate proves that the opening is exactly one fresh admission or one tree
// continuation and that every accompanying projection is limited to transcript
// Items and/or provider conversation messages. Those projections are deliberately
// independent: an application-authored Goal instruction may feed future model
// context without becoming a user-visible Item. Persistence Port implementations
// may reject unavailable stores or concurrent state changes, but they do not
// reinterpret this application write-set.
func (o OpeningCommit) Validate() error {
	if err := o.commitID.Validate(); err != nil {
		return fmt.Errorf("runs: opening: %w", err)
	}
	if (o.admit == nil) == (o.resume == nil) {
		return errors.New("runs: opening requires exactly one admission action")
	}
	if o.admit != nil {
		if err := o.validateAdmission(); err != nil {
			return err
		}
	} else {
		if err := o.resume.Validate(); err != nil {
			return fmt.Errorf("runs: opening resume: %w", err)
		}
		if o.initialSession != nil || o.sessionReplacement != nil || o.automation != nil {
			return errors.New("runs: resumed opening carries fresh-run facts")
		}
		if err := o.validateResumeAcceptance(); err != nil {
			return err
		}
	}
	if err := o.validateCommandAcceptance(); err != nil {
		return err
	}
	return o.validateEvents()
}

func (o OpeningCommit) validateAdmission() error {
	if err := o.admit.Validate(); err != nil {
		return fmt.Errorf("runs: opening admission: %w", err)
	}
	if o.resumeAcceptance != nil {
		return errors.New("runs: fresh admission carries a resume acceptance")
	}
	if o.admit.Lineage().IsChild() &&
		(o.initialSession != nil || o.sessionReplacement != nil || o.automation != nil ||
			o.commandAcceptance != nil) {
		return errors.New("runs: child opening carries root admission facts")
	}
	if o.initialSession != nil {
		if o.initialSession.ID() != o.admit.SessionID || o.initialSession.Revision() != 1 {
			return errors.New("runs: opening initial Session differs from admitted Run")
		}
	}
	if o.sessionReplacement != nil {
		if err := o.sessionReplacement.Validate(); err != nil {
			return fmt.Errorf("runs: invalid opening Session replacement: %w", err)
		}
		if o.sessionReplacement.ExpectedRevision() == 0 ||
			o.sessionReplacement.State().ID() != o.admit.SessionID {
			return errors.New("runs: opening Session replacement differs from admitted Run")
		}
	}
	if o.initialSession != nil && o.sessionReplacement != nil {
		return errors.New("runs: opening cannot insert and replace the same Session")
	}
	if o.automation != nil {
		if o.initialSession == nil {
			return errors.New("runs: automation opening has no initial Session")
		}
		if o.automation.RunID != o.admit.RunID || o.automation.SessionID != o.admit.SessionID {
			return errors.New("runs: automation opening differs from admitted Run")
		}
	}
	return nil
}

func (o OpeningCommit) validateCommandAcceptance() error {
	if o.commandAcceptance == nil {
		return nil
	}
	expected, err := AcceptCommand(o.commandAcceptance.Key, o.commandAcceptance.Fingerprint, o)
	if err != nil {
		return err
	}
	if !expected.Equal(*o.commandAcceptance) {
		return errors.New("runs: command acceptance differs from its opening")
	}
	return nil
}

// validateResumeAcceptance proves the answer write-set names the exact tree this
// continuation resumes.
func (o OpeningCommit) validateResumeAcceptance() error {
	if o.resumeAcceptance == nil {
		return nil
	}
	if err := o.resumeAcceptance.Validate(); err != nil {
		return fmt.Errorf("runs: opening resume acceptance: %w", err)
	}
	if len(o.resume.Runs) == 0 {
		return errors.New("runs: resumed opening has no Runs")
	}
	root := o.resume.Runs[len(o.resume.Runs)-1].State()
	if o.resumeAcceptance.SessionID() != o.resume.SessionID() ||
		o.resumeAcceptance.Pending().RootRunID != root.ID() {
		return errors.New("runs: opening resume acceptance differs from the resumed tree")
	}
	return nil
}

func (o OpeningCommit) validateEvents() error {
	for index, commit := range o.events {
		if !commit.CommitID.IsZero() {
			return fmt.Errorf("runs: opening event[%d] carries a top-level event commit identity", index)
		}
		if err := commit.Validate(); err != nil {
			return fmt.Errorf("runs: opening event[%d]: %w", index, err)
		}
		if err := o.validateEventOwner(commit); err != nil {
			return fmt.Errorf("runs: opening event[%d]: %w", index, err)
		}
		if err := validateOpeningProjection(commit); err != nil {
			return fmt.Errorf("runs: opening event[%d]: %w", index, err)
		}
	}
	return nil
}

// validateOpeningProjection limits admission/resume projections to the facts
// that can exist before execution begins. Operational observations belong to
// later authoritative EventCommits, even when they name the same Segment.
func validateOpeningProjection(commit EventCommit) error {
	if commit.State != StateUnchanged || commit.Run != nil ||
		commit.ObsoleteCheckpointRootID != "" {
		return errors.New("opening projection carries lifecycle facts")
	}
	if len(commit.ModelInvocations) != 0 || len(commit.ToolInvocations) != 0 || commit.Progress != nil {
		return errors.New("opening projection carries execution observations")
	}
	if len(commit.Items) == 0 && len(commit.ConversationMessages) == 0 {
		return errors.New("opening projection has no transcript or conversation facts")
	}
	return nil
}

func (o OpeningCommit) validateEventOwner(commit EventCommit) error {
	if o.admit != nil {
		if commit.SessionID != o.admit.SessionID {
			return errors.New("event Session differs from admitted Run")
		}
		if commit.RunID == o.admit.RunID {
			if commit.SegmentID != o.admit.SegmentID {
				return errors.New("admitted Run event belongs to another Segment")
			}
			return nil
		}
		lineage := o.admit.Lineage()
		if lineage.IsChild() && commit.RunID == lineage.ParentRunID {
			return nil
		}
		return errors.New("event belongs to a Run outside the admission")
	}
	if commit.SessionID != o.resume.SessionID() {
		return errors.New("event Session differs from resumed tree")
	}
	for _, resumed := range o.resume.Runs {
		if commit.RunID == resumed.State().ID() {
			if commit.SegmentID != resumed.State().ActiveSegmentID() {
				return errors.New("resumed Run event belongs to another Segment")
			}
			return nil
		}
	}
	return errors.New("event belongs to a Run outside the resumed tree")
}

func (o OpeningCommit) CommitID() runtimeidentity.CommitID { return o.commitID }

func (o OpeningCommit) Admission() (run.Draft, bool) {
	if o.admit == nil {
		return run.Draft{}, false
	}
	return cloneOpeningAdmission(*o.admit), true
}

func (o OpeningCommit) Resume() (run.TreeResumeDraft, bool) {
	if o.resume == nil {
		return run.TreeResumeDraft{}, false
	}
	return cloneOpeningResume(*o.resume), true
}

func (o OpeningCommit) ResumeAcceptance() (ResumeAcceptance, bool) {
	if o.resumeAcceptance == nil {
		return ResumeAcceptance{}, false
	}
	return *o.resumeAcceptance, true
}

func (o OpeningCommit) InitialSession() (session.Session, bool) {
	return optional.Present(o.initialSession)
}

func (o OpeningCommit) SessionReplacement() (session.Replacement, bool) {
	return optional.Present(o.sessionReplacement)
}

func (o OpeningCommit) Automation() (AutomationStart, bool) {
	return optional.Present(o.automation)
}

func (o OpeningCommit) CommandAcceptance() (CommandAcceptance, bool) {
	return optional.Present(o.commandAcceptance)
}

// ownerIdentity is the one rule for which Session, Run, and Segment an opening
// opens, so the acceptance record, the commit receipt, and the input-item scan
// cannot name different Runs for it.
func (o OpeningCommit) ownerIdentity() (sessionID, runID, segmentID string, err error) {
	if o.admit != nil {
		return o.admit.SessionID, o.admit.RunID, o.admit.SegmentID, nil
	}
	if o.resume != nil && len(o.resume.Runs) > 0 {
		root := o.resume.Runs[len(o.resume.Runs)-1].State()
		return root.SessionID(), root.ID(), root.ActiveSegmentID(), nil
	}
	return "", "", "", errors.New("runs: opening has no owner")
}

func (o OpeningCommit) OwnerIdentity() (sessionID, runID, segmentID string, err error) {
	return o.ownerIdentity()
}

// UserInputItemID is the one rule for which Item an opening accepted as its input,
// so the acceptance record and the continuation binding cannot name different
// Items. More than one root user input is a fault, not silently narrowed to one.
func (o OpeningCommit) UserInputItemID() (string, error) {
	_, root, _, err := o.ownerIdentity()
	if err != nil {
		return "", err
	}
	var id string
	for _, event := range o.events {
		for _, item := range event.Items {
			if item.Kind() != transcript.UserMessage || item.RunID() != root {
				continue
			}
			if id != "" {
				return "", errors.New("runs: opening repeats its user input")
			}
			id = item.ID()
		}
	}
	return id, nil
}

// Events returns isolated opening projections in their canonical order.
func (o OpeningCommit) Events() []EventCommit { return cloneEventCommits(o.events) }

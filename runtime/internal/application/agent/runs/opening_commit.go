package runs

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// OpeningCommit is the atomic acceptance write-set for one fresh admission or
// one continuation.
type OpeningCommit struct {
	commitID           runtimeidentity.CommitID
	admit              *run.Draft
	resume             *run.TreeResumeDraft
	initialSession     *session.Session
	sessionReplacement *session.Replacement
	scheduleFiring     string
	manualScheduleRun  *schedule.RunRecord
	events             []EventCommit
}

// NewAdmissionOpeningCommit binds a fresh Run admission to every root-owned
// Session or Schedule fact and opening projection written in its transaction.
func NewAdmissionOpeningCommit(
	commitID runtimeidentity.CommitID,
	admit run.Draft,
	initialSession *session.Session,
	sessionReplacement *session.Replacement,
	scheduleFiring string,
	manualScheduleRun *schedule.RunRecord,
	events []EventCommit,
) (OpeningCommit, error) {
	admit = cloneOpeningAdmission(admit)
	opening := OpeningCommit{
		commitID: commitID, admit: &admit,
		scheduleFiring: scheduleFiring, events: cloneEventCommits(events),
	}
	if initialSession != nil {
		value := *initialSession
		opening.initialSession = &value
	}
	if sessionReplacement != nil {
		value := *sessionReplacement
		opening.sessionReplacement = &value
	}
	if manualScheduleRun != nil {
		value := *manualScheduleRun
		opening.manualScheduleRun = &value
	}
	if err := opening.Validate(); err != nil {
		return OpeningCommit{}, err
	}
	return opening, nil
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
		if o.initialSession != nil || o.sessionReplacement != nil || o.scheduleFiring != "" || o.manualScheduleRun != nil {
			return errors.New("runs: resumed opening carries fresh-run facts")
		}
	}
	return o.validateEvents()
}

func (o OpeningCommit) validateAdmission() error {
	if err := o.admit.Validate(); err != nil {
		return fmt.Errorf("runs: opening admission: %w", err)
	}
	if o.admit.Lineage().IsChild() &&
		(o.initialSession != nil || o.sessionReplacement != nil || o.scheduleFiring != "" || o.manualScheduleRun != nil) {
		return errors.New("runs: child opening carries root admission facts")
	}
	if o.initialSession != nil {
		if err := o.initialSession.Validate(); err != nil {
			return fmt.Errorf("runs: opening initial Session: %w", err)
		}
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
	if (o.scheduleFiring != "" || o.manualScheduleRun != nil) && o.initialSession == nil {
		return errors.New("runs: schedule opening has no initial Session")
	}
	if o.scheduleFiring != "" {
		if err := schedule.ValidateOccurrenceID(o.scheduleFiring); err != nil {
			return fmt.Errorf("runs: opening schedule firing: %w", err)
		}
	}
	if o.manualScheduleRun != nil {
		if err := o.manualScheduleRun.Validate(); err != nil {
			return fmt.Errorf("runs: opening manual schedule Run: %w", err)
		}
		if o.scheduleFiring != "" {
			return errors.New("runs: opening mixes scheduled occurrence and manual schedule Run")
		}
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
	if commit.State != StateUnchanged || commit.Outcome != "" || commit.Run != nil ||
		commit.GoalRun != nil || commit.ObsoleteCheckpointRootID != "" {
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
	if commit.SessionID != o.resume.SessionID {
		return errors.New("event Session differs from resumed tree")
	}
	for _, resumed := range o.resume.Runs {
		if commit.RunID == resumed.RunID {
			if commit.SegmentID != resumed.SegmentID {
				return errors.New("resumed Run event belongs to another Segment")
			}
			return nil
		}
	}
	return errors.New("event belongs to a Run outside the resumed tree")
}

// CommitID returns the stable admission or resume transaction identity.
func (o OpeningCommit) CommitID() runtimeidentity.CommitID { return o.commitID }

// Admission returns an isolated fresh-Run admission when this is an admission opening.
func (o OpeningCommit) Admission() (run.Draft, bool) {
	if o.admit == nil {
		return run.Draft{}, false
	}
	return cloneOpeningAdmission(*o.admit), true
}

// Resume returns an isolated tree continuation when this is a resume opening.
func (o OpeningCommit) Resume() (run.TreeResumeDraft, bool) {
	if o.resume == nil {
		return run.TreeResumeDraft{}, false
	}
	return cloneOpeningResume(*o.resume), true
}

// InitialSession returns the new Session inserted with a root admission.
func (o OpeningCommit) InitialSession() (session.Session, bool) {
	if o.initialSession == nil {
		return session.Session{}, false
	}
	return *o.initialSession, true
}

// SessionReplacement returns the existing Session revision written with an admission.
func (o OpeningCommit) SessionReplacement() (session.Replacement, bool) {
	if o.sessionReplacement == nil {
		return session.Replacement{}, false
	}
	return *o.sessionReplacement, true
}

// ScheduleFiring returns the occurrence accepted with a scheduled admission.
func (o OpeningCommit) ScheduleFiring() string { return o.scheduleFiring }

// ManualScheduleRun returns the manual Schedule execution recorded with an admission.
func (o OpeningCommit) ManualScheduleRun() (schedule.RunRecord, bool) {
	if o.manualScheduleRun == nil {
		return schedule.RunRecord{}, false
	}
	return *o.manualScheduleRun, true
}

// Events returns isolated opening projections in their canonical order.
func (o OpeningCommit) Events() []EventCommit { return cloneEventCommits(o.events) }

package runs

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/exactint"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// OpeningCommit is the atomic acceptance write-set for one fresh admission or
// one continuation.
type OpeningCommit struct {
	// CommitID identifies the complete admission/resume transaction, including
	// every opening projection. It is not an EventCommit identity.
	CommitID           runtimeidentity.CommitID
	Admit              *run.Draft
	Resume             *run.TreeResumeDraft
	InitialSession     *session.Session
	SessionReplacement *SessionReplacement
	ScheduleFiring     string
	ManualScheduleRun  *schedule.RunRecord
	Events             []EventCommit
}

// SessionReplacement is the exact Session aggregate replacement committed with
// the Run admission whose configured model produced it.
type SessionReplacement struct {
	ExpectedRevision uint64
	State            session.Session
}

// Validate proves that the replacement advances the same Session once.
func (s SessionReplacement) Validate(sessionID string) error {
	if err := s.State.Validate(); err != nil {
		return fmt.Errorf("runs: invalid Session replacement: %w", err)
	}
	if s.State.ID() != sessionID {
		return errors.New("runs: opening Session replacement differs from admitted Run")
	}
	if s.ExpectedRevision == 0 || exactint.Follows(s.ExpectedRevision, s.State.Revision()) != nil {
		return errors.New("runs: opening Session replacement does not advance one revision")
	}
	return nil
}

// Validate proves that the opening is exactly one fresh admission or one tree
// continuation and that every accompanying projection is limited to transcript
// Items and/or provider conversation messages. Those projections are deliberately
// independent: an application-authored Goal instruction may feed future model
// context without becoming a user-visible Item. Persistence Port implementations
// may reject unavailable stores or concurrent state changes, but they do not
// reinterpret this application write-set.
func (o OpeningCommit) Validate() error {
	if err := o.CommitID.Validate(); err != nil {
		return fmt.Errorf("runs: opening: %w", err)
	}
	if (o.Admit == nil) == (o.Resume == nil) {
		return errors.New("runs: opening requires exactly one admission action")
	}
	if o.Admit != nil {
		if err := o.validateAdmission(); err != nil {
			return err
		}
	} else {
		if err := o.Resume.Validate(); err != nil {
			return fmt.Errorf("runs: opening resume: %w", err)
		}
		if o.InitialSession != nil || o.SessionReplacement != nil || o.ScheduleFiring != "" || o.ManualScheduleRun != nil {
			return errors.New("runs: resumed opening carries fresh-run facts")
		}
	}
	return o.validateEvents()
}

func (o OpeningCommit) validateAdmission() error {
	if err := o.Admit.Validate(); err != nil {
		return fmt.Errorf("runs: opening admission: %w", err)
	}
	if o.Admit.Lineage().IsChild() &&
		(o.InitialSession != nil || o.SessionReplacement != nil || o.ScheduleFiring != "" || o.ManualScheduleRun != nil) {
		return errors.New("runs: child opening carries root admission facts")
	}
	if o.InitialSession != nil {
		if err := o.InitialSession.Validate(); err != nil {
			return fmt.Errorf("runs: opening initial Session: %w", err)
		}
		if o.InitialSession.ID() != o.Admit.SessionID || o.InitialSession.Revision() != 1 {
			return errors.New("runs: opening initial Session differs from admitted Run")
		}
	}
	if o.SessionReplacement != nil {
		if err := o.SessionReplacement.Validate(o.Admit.SessionID); err != nil {
			return err
		}
	}
	if o.InitialSession != nil && o.SessionReplacement != nil {
		return errors.New("runs: opening cannot insert and replace the same Session")
	}
	if (o.ScheduleFiring != "" || o.ManualScheduleRun != nil) && o.InitialSession == nil {
		return errors.New("runs: schedule opening has no initial Session")
	}
	if o.ScheduleFiring != "" {
		if err := schedule.ValidateOccurrenceID(o.ScheduleFiring); err != nil {
			return fmt.Errorf("runs: opening schedule firing: %w", err)
		}
	}
	if o.ManualScheduleRun != nil {
		if err := o.ManualScheduleRun.Validate(); err != nil {
			return fmt.Errorf("runs: opening manual schedule Run: %w", err)
		}
		if o.ScheduleFiring != "" {
			return errors.New("runs: opening mixes scheduled occurrence and manual schedule Run")
		}
	}
	return nil
}

func (o OpeningCommit) validateEvents() error {
	for index, commit := range o.Events {
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
	if o.Admit != nil {
		if commit.SessionID != o.Admit.SessionID {
			return errors.New("event Session differs from admitted Run")
		}
		if commit.RunID == o.Admit.RunID {
			if commit.SegmentID != o.Admit.SegmentID {
				return errors.New("admitted Run event belongs to another Segment")
			}
			return nil
		}
		lineage := o.Admit.Lineage()
		if lineage.IsChild() && commit.RunID == lineage.ParentRunID {
			return nil
		}
		return errors.New("event belongs to a Run outside the admission")
	}
	if commit.SessionID != o.Resume.SessionID {
		return errors.New("event Session differs from resumed tree")
	}
	for _, resumed := range o.Resume.Runs {
		if commit.RunID == resumed.RunID {
			if commit.SegmentID != resumed.SegmentID {
				return errors.New("resumed Run event belongs to another Segment")
			}
			return nil
		}
	}
	return errors.New("event belongs to a Run outside the resumed tree")
}

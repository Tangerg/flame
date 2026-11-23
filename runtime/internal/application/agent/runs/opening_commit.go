package runs

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

type OpeningCommit struct {
	commitID          CommitID
	action            openingAction
	commandAcceptance *CommandAcceptance
	events            []EventCommit
}

type openingAction interface {
	validate() error
	owner() (identity.SessionID, identity.RunID, identity.SegmentID, error)
	ownsEvent(EventCommit) error
}

type admissionOpening struct {
	draft   run.Draft
	session OpeningSessionWrite
	actor   *operation.Actor
}

type resumeOpening struct {
	draft      run.TreeResumeDraft
	acceptance *ResumeAcceptance
}

type OpeningSessionWrite interface{ openingSessionWrite() }

type SessionInsert struct{ Session session.Session }

type SessionSave struct{ Replacement session.Replacement }

func (SessionInsert) openingSessionWrite() {}
func (SessionSave) openingSessionWrite()   {}

func NewAdmissionOpeningCommit(
	commitID CommitID,
	admit run.Draft,
	sessionWrite OpeningSessionWrite,
	actor *operation.Actor,
	events []EventCommit,
) (OpeningCommit, error) {
	action := admissionOpening{draft: cloneOpeningAdmission(admit), session: sessionWrite}
	if actor != nil {
		action.actor = new(*actor)
	}
	opening := OpeningCommit{commitID: commitID, action: action, events: cloneEventCommits(events)}
	if err := opening.Validate(); err != nil {
		return OpeningCommit{}, err
	}
	return opening, nil
}

func (o OpeningCommit) WithCommandAcceptance(acceptance CommandAcceptance) (OpeningCommit, error) {
	next := o
	next.commandAcceptance = &acceptance
	if err := next.Validate(); err != nil {
		return OpeningCommit{}, err
	}
	return next, nil
}

func NewResumeOpeningCommit(
	commitID CommitID,
	resume run.TreeResumeDraft,
	events []EventCommit,
) (OpeningCommit, error) {
	opening := OpeningCommit{
		commitID: commitID, action: resumeOpening{draft: cloneOpeningResume(resume)}, events: cloneEventCommits(events),
	}
	if err := opening.Validate(); err != nil {
		return OpeningCommit{}, err
	}
	return opening, nil
}

func (o OpeningCommit) WithResumeAcceptance(acceptance ResumeAcceptance) (OpeningCommit, error) {
	resume, ok := o.action.(resumeOpening)
	if !ok {
		return OpeningCommit{}, errors.New("runs: resume acceptance requires a tree continuation")
	}
	resume.acceptance = &acceptance
	next := o
	next.action = resume
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

func (o OpeningCommit) Validate() error {
	if err := o.commitID.Validate(); err != nil {
		return fmt.Errorf("runs: opening: %w", err)
	}
	if o.action == nil {
		return errors.New("runs: opening requires exactly one admission action")
	}
	if err := o.action.validate(); err != nil {
		return err
	}
	if err := o.validateCommandAcceptance(); err != nil {
		return err
	}
	return o.validateEvents()
}

func (a admissionOpening) validate() error {
	if a.actor != nil {
		if err := a.actor.Validate(); err != nil {
			return err
		}
	}
	if err := a.draft.Validate(); err != nil {
		return fmt.Errorf("runs: opening admission: %w", err)
	}
	if a.draft.Lineage().IsChild() && (a.session != nil || a.actor != nil) {
		return errors.New("runs: child opening carries root admission facts")
	}
	switch write := a.session.(type) {
	case SessionInsert:
		if write.Session.ID() != a.draft.SessionID || write.Session.Revision() != 1 {
			return errors.New("runs: opening initial Session differs from admitted Run")
		}
	case SessionSave:
		if err := write.Replacement.Validate(); err != nil {
			return fmt.Errorf("runs: invalid opening Session replacement: %w", err)
		}
		if _, replacing := write.Replacement.Expected(); !replacing || write.Replacement.State().ID() != a.draft.SessionID {
			return errors.New("runs: opening Session replacement differs from admitted Run")
		}
	}
	return nil
}

func (r resumeOpening) validate() error {
	if err := r.draft.Validate(); err != nil {
		return fmt.Errorf("runs: opening resume: %w", err)
	}
	if r.acceptance == nil {
		return nil
	}
	if len(r.acceptance.itemChanges) == 0 {
		return errors.New("runs: resume acceptance has no settled Items")
	}
	_, rootID, _, err := r.owner()
	if err != nil {
		return err
	}
	if r.acceptance.SessionID() != r.draft.SessionID() || r.acceptance.expected.RootRunID != rootID {
		return errors.New("runs: opening resume acceptance differs from the resumed tree")
	}
	return nil
}

func (o OpeningCommit) validateCommandAcceptance() error {
	if o.commandAcceptance == nil {
		return nil
	}
	if admission, ok := o.action.(admissionOpening); ok && admission.draft.Lineage().IsChild() {
		return errors.New("runs: child opening carries root admission facts")
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

func (o OpeningCommit) validateEvents() error {
	for index, commit := range o.events {
		if !commit.CommitID.IsZero() {
			return fmt.Errorf("runs: opening event[%d] carries a top-level event commit identity", index)
		}
		if err := commit.Validate(); err != nil {
			return fmt.Errorf("runs: opening event[%d]: %w", index, err)
		}
		if err := o.action.ownsEvent(commit); err != nil {
			return fmt.Errorf("runs: opening event[%d]: %w", index, err)
		}
		if err := validateOpeningProjection(commit); err != nil {
			return fmt.Errorf("runs: opening event[%d]: %w", index, err)
		}
	}
	return nil
}

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

func (a admissionOpening) ownsEvent(commit EventCommit) error {
	if commit.SessionID != a.draft.SessionID {
		return errors.New("event Session differs from admitted Run")
	}
	if commit.RunID == a.draft.RunID {
		if commit.SegmentID != a.draft.SegmentID {
			return errors.New("admitted Run event belongs to another Segment")
		}
		return nil
	}
	lineage := a.draft.Lineage()
	if lineage.IsChild() && commit.RunID == lineage.ParentRunID() {
		return nil
	}
	return errors.New("event belongs to a Run outside the admission")
}

func (r resumeOpening) ownsEvent(commit EventCommit) error {
	if commit.SessionID != r.draft.SessionID() {
		return errors.New("event Session differs from resumed tree")
	}
	for _, resumed := range r.draft.Runs {
		if commit.RunID == resumed.State().ID() {
			if commit.SegmentID != resumed.State().ActiveSegmentID() {
				return errors.New("resumed Run event belongs to another Segment")
			}
			return nil
		}
	}
	return errors.New("event belongs to a Run outside the resumed tree")
}

func (o OpeningCommit) CommitID() CommitID { return o.commitID }

func (o OpeningCommit) Admission() (run.Draft, bool) {
	admission, ok := o.action.(admissionOpening)
	if !ok {
		return run.Draft{}, false
	}
	return cloneOpeningAdmission(admission.draft), true
}

func (o OpeningCommit) Resume() (run.TreeResumeDraft, bool) {
	resume, ok := o.action.(resumeOpening)
	if !ok {
		return run.TreeResumeDraft{}, false
	}
	return cloneOpeningResume(resume.draft), true
}

func (o OpeningCommit) ResumeAcceptance() (ResumeAcceptance, bool) {
	resume, ok := o.action.(resumeOpening)
	if !ok || resume.acceptance == nil {
		return ResumeAcceptance{}, false
	}
	return *resume.acceptance, true
}

func (o OpeningCommit) SessionWrite() OpeningSessionWrite {
	admission, _ := o.action.(admissionOpening)
	return admission.session
}

func (o OpeningCommit) Actor() (operation.Actor, bool) {
	admission, ok := o.action.(admissionOpening)
	if !ok || admission.actor == nil {
		return operation.Actor{}, false
	}
	return *admission.actor, true
}

func (o OpeningCommit) CommandAcceptance() (CommandAcceptance, bool) {
	if o.commandAcceptance == nil {
		return CommandAcceptance{}, false
	}
	return *o.commandAcceptance, true
}

func (o OpeningCommit) OwnerIdentity() (sessionID identity.SessionID, runID identity.RunID, segmentID identity.SegmentID, err error) {
	if o.action == nil {
		return identity.SessionID{}, identity.RunID{}, identity.SegmentID{}, errors.New("runs: opening has no owner")
	}
	return o.action.owner()
}

func (a admissionOpening) owner() (identity.SessionID, identity.RunID, identity.SegmentID, error) {
	return a.draft.SessionID, a.draft.RunID, a.draft.SegmentID, nil
}

func (r resumeOpening) owner() (identity.SessionID, identity.RunID, identity.SegmentID, error) {
	if len(r.draft.Runs) == 0 {
		return identity.SessionID{}, identity.RunID{}, identity.SegmentID{}, errors.New("runs: resumed opening has no Runs")
	}
	root := r.draft.Runs[len(r.draft.Runs)-1].State()
	return root.SessionID(), root.ID(), root.ActiveSegmentID(), nil
}

func (o OpeningCommit) UserInputItemID() (identity.ItemID, error) {
	_, root, _, err := o.OwnerIdentity()
	if err != nil {
		return identity.ItemID{}, err
	}
	var id identity.ItemID
	for _, event := range o.events {
		for _, write := range event.Items {
			item := write.Item()
			if item.Kind() != transcript.UserMessage || item.RunID() != root {
				continue
			}
			if !id.IsZero() {
				return identity.ItemID{}, errors.New("runs: opening repeats its user input")
			}
			id = item.ID()
		}
	}
	return id, nil
}

func (o OpeningCommit) Events() []EventCommit { return cloneEventCommits(o.events) }

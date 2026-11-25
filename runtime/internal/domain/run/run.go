package run

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
)

var ErrIdentityConflict = errors.New("run: identity conflict")

// The zero value is absence, so it cannot be confused with a real count, including
// zero.
type MessageMark struct {
	count int
	known bool
}

func UnknownMessageMark() MessageMark { return MessageMark{} }

func MessageMarkAt(count int) MessageMark { return MessageMark{count: count, known: true} }

func (m MessageMark) Count() (int, bool) { return m.count, m.known }

func (m MessageMark) String() string {
	if !m.known {
		return "unknown"
	}
	return strconv.Itoa(m.count)
}

type Run struct {
	sessionID         identity.SessionID
	id                identity.RunID
	lineage           Lineage
	modelSelection    flamemodel.Selection
	goalIncarnationID goalref.IncarnationID
	lifecycle         Lifecycle
	metrics           Metrics
	contextTokens     int64
	capabilities      Capabilities
	createdAt         time.Time
	updatedAt         time.Time
}

type Snapshot struct {
	SessionID         identity.SessionID
	ID                identity.RunID
	Lineage           Lineage
	ModelSelection    flamemodel.Selection
	GoalIncarnationID goalref.IncarnationID
	Lifecycle         Lifecycle
	Metrics           Metrics
	ContextTokens     int64
	Capabilities      Capabilities
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func Admit(draft Draft) (Run, error) {
	if err := draft.Validate(); err != nil {
		return Run{}, err
	}
	return Run{
		sessionID: draft.SessionID, id: draft.RunID, lineage: draft.Lineage(),
		modelSelection: draft.ModelSelection, goalIncarnationID: draft.GoalIncarnationID,
		lifecycle: ActiveSegment{ID: draft.SegmentID}, capabilities: draft.Capabilities.Clone(),
		createdAt: draft.CreatedAt.UTC(), updatedAt: draft.CreatedAt.UTC(),
	}, nil
}

func Restore(snapshot Snapshot) (Run, error) {
	run := Run{
		sessionID: snapshot.SessionID, id: snapshot.ID, lineage: snapshot.Lineage,
		modelSelection: snapshot.ModelSelection, goalIncarnationID: snapshot.GoalIncarnationID,
		metrics: snapshot.Metrics, contextTokens: snapshot.ContextTokens,
		capabilities: snapshot.Capabilities.Clone(),
		createdAt:    snapshot.CreatedAt.UTC(), updatedAt: snapshot.UpdatedAt.UTC(),
	}
	if err := run.validateIdentity(); err != nil {
		return Run{}, err
	}
	if err := run.validateAccrual(); err != nil {
		return Run{}, err
	}
	if err := run.validateRootOwnership(); err != nil {
		return Run{}, err
	}
	switch lifecycle := snapshot.Lifecycle.(type) {
	case ActiveSegment:
		if err := lifecycle.ID.Validate(); err != nil {
			return Run{}, fmt.Errorf("run: active Segment: %w", err)
		}
	case Parked:
	case Termination:
		return run.withTermination(lifecycle)
	default:
		return Run{}, fmt.Errorf("run: invalid lifecycle %T", lifecycle)
	}
	run.lifecycle = snapshot.Lifecycle
	return run, nil
}

func (r Run) Fork(sessionID identity.SessionID, id identity.RunID, lineage Lineage) (Run, error) {
	if !r.State().IsTerminal() {
		return Run{}, errors.New("run: only a terminal Run can be forked")
	}
	if r.lineage.IsRoot() != lineage.IsRoot() {
		return Run{}, errors.New("run: fork changes root/child lineage kind")
	}
	if err := sessionID.Validate(); err != nil {
		return Run{}, fmt.Errorf("run: %w", err)
	}
	if err := id.Validate(); err != nil {
		return Run{}, fmt.Errorf("run: %w", err)
	}
	if err := lineage.Validate(id); err != nil {
		return Run{}, err
	}
	r.sessionID, r.id, r.lineage = sessionID, id, lineage
	r.goalIncarnationID = goalref.IncarnationID{}
	return r, nil
}

func (r Run) Equal(other Run) bool {
	return r.sameIdentity(other) && r.sameProgress(other) &&
		r.createdAt.Equal(other.createdAt) && r.updatedAt.Equal(other.updatedAt)
}

func (r Run) sameIdentity(other Run) bool {
	return r.sessionID == other.sessionID && r.id == other.id && r.lineage == other.lineage &&
		r.modelSelection.Equal(other.modelSelection) && r.goalIncarnationID == other.goalIncarnationID &&
		r.capabilities.Equal(other.capabilities)
}

func (r Run) sameProgress(other Run) bool {
	if !r.metrics.Equal(other.metrics) || r.contextTokens != other.contextTokens {
		return false
	}
	if terminal, ok := r.lifecycle.(Termination); ok {
		otherTerminal, ok := other.lifecycle.(Termination)
		return ok && terminal.equal(otherTerminal)
	}
	return r.lifecycle == other.lifecycle
}

func (r Run) validateIdentity() error {
	if err := r.id.Validate(); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	if err := r.sessionID.Validate(); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	switch {
	case r.createdAt.IsZero():
		return errors.New("run: creation time is required")
	case r.updatedAt.IsZero():
		return errors.New("run: update time is required")
	case r.updatedAt.Before(r.createdAt):
		return errors.New("run: update time precedes creation")
	}
	if err := instant.Validate(r.createdAt, r.updatedAt); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	if err := r.modelSelection.ValidateExact(); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return r.lineage.Validate(r.id)
}

func (r Run) validateAccrual() error {
	if err := r.metrics.Validate(); err != nil {
		return err
	}
	if r.contextTokens < 0 {
		return errors.New("run: context tokens must not be negative")
	}
	return r.capabilities.Validate()
}

func (r Run) validateRootOwnership() error {
	if r.lineage.IsRoot() {
		return nil
	}
	switch {
	case !r.goalIncarnationID.IsZero():
		return errors.New("run: child carries a root Goal incarnation")
	case !r.capabilities.IsEmpty():
		return errors.New("run: child carries capabilities its root owns")
	}
	return nil
}

func (r Run) withTermination(t Termination) (Run, error) {
	if err := ValidateEnding(t.Ending); err != nil {
		return Run{}, err
	}
	if err := validateUnresolvedEffects(t.UnresolvedEffects); err != nil {
		return Run{}, err
	}
	switch {
	case t.FinishedAt.IsZero():
		return Run{}, errors.New("run: terminal Run has no finish time")
	case t.FinishedAt.Before(r.createdAt):
		return Run{}, errors.New("run: finish time precedes creation")
	case r.updatedAt.Before(t.FinishedAt):
		return Run{}, errors.New("run: update time precedes finish")
	case t.MessageMark.known && t.MessageMark.count < 0:
		return Run{}, fmt.Errorf("run: terminal Run has message watermark %s", t.MessageMark)
	case t.MessageMark.known && r.lineage.IsChild():
		return Run{}, errors.New("run: child carries the conversation watermark its root owns")
	}
	if err := instant.Validate(t.FinishedAt); err != nil {
		return Run{}, fmt.Errorf("run: %w", err)
	}
	t.FinishedAt = t.FinishedAt.UTC()
	t.UnresolvedEffects = slices.Clone(t.UnresolvedEffects)
	r.lifecycle = t
	return r, nil
}

func (r Run) AdvanceProgress(metrics Metrics, contextTokens int64, updatedAt time.Time) (Run, error) {
	if !r.State().IsOpen() {
		return Run{}, fmt.Errorf("run: cannot advance progress from state %q", r.State())
	}
	if err := metrics.ValidateAdvanceFrom(r.metrics); err != nil {
		return Run{}, err
	}
	if contextTokens < 0 {
		return Run{}, errors.New("run: context tokens must not be negative")
	}
	if err := r.validateTransitionTime(updatedAt); err != nil {
		return Run{}, err
	}
	r.metrics = metrics
	if contextTokens > 0 {
		r.contextTokens = contextTokens
	}
	r.updatedAt = updatedAt.UTC()
	return r, nil
}

func (r Run) Suspend(updatedAt time.Time) (Run, error) {
	if r.State() != Running {
		return Run{}, fmt.Errorf("run: cannot suspend %s Run", r.State())
	}
	if err := r.validateTransitionTime(updatedAt); err != nil {
		return Run{}, err
	}
	r.lifecycle, r.updatedAt = Parked{}, updatedAt.UTC()
	return r, nil
}

func (r Run) Resume(segmentID identity.SegmentID, resumedAt time.Time) (Run, error) {
	if r.State() != Waiting {
		return Run{}, fmt.Errorf("run: cannot resume %s Run", r.State())
	}
	if err := segmentID.Validate(); err != nil {
		return Run{}, fmt.Errorf("run: continuation Segment: %w", err)
	}
	if err := r.validateTransitionTime(resumedAt); err != nil {
		return Run{}, err
	}
	r.lifecycle, r.updatedAt = ActiveSegment{ID: segmentID}, resumedAt.UTC()
	return r, nil
}

func (r Run) Terminate(termination Termination) (Run, error) {
	switch state := r.State(); state {
	case Running:
	case Waiting:
		switch termination.Ending.(type) {
		case Canceled, Lost:
		default:
			return Run{}, fmt.Errorf("run: %T cannot terminate a waiting Run", termination.Ending)
		}
	default:
		return Run{}, fmt.Errorf("run: cannot terminate %s Run", state)
	}
	if !termination.FinishedAt.IsZero() && termination.FinishedAt.Before(r.updatedAt) {
		return Run{}, errors.New("run: finish time precedes last update")
	}
	r.updatedAt = termination.FinishedAt.UTC()
	return r.withTermination(termination)
}

func (r Run) RebaseConversation(compaction conversation.Compaction) (Run, error) {
	if !r.State().IsTerminal() || r.lineage.IsChild() {
		return r, nil
	}
	count, known := r.MessageMark().Count()
	if !known {
		return Run{}, errors.New("run: terminal root Run has no message watermark")
	}
	mark, err := compaction.RebaseMessageMark(count)
	if err != nil {
		return Run{}, err
	}
	return r.WithMessageMark(mark)
}

func (r Run) WithMessageMark(messageMark int) (Run, error) {
	if r.lineage.IsChild() {
		return Run{}, errors.New("run: a child Run has no conversation watermark of its own")
	}
	terminal, ok := r.lifecycle.(Termination)
	if !ok {
		return Run{}, errors.New("run: only terminal Run can resolve message watermark")
	}
	if messageMark < 0 {
		return Run{}, errors.New("run: message watermark must not be negative")
	}
	terminal.MessageMark = MessageMarkAt(messageMark)
	r.lifecycle = terminal
	return r, nil
}

func (r Run) validateTransitionTime(at time.Time) error {
	if at.IsZero() {
		return errors.New("run: transition time is required")
	}
	if err := instant.Validate(at); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	at = at.UTC()
	if at.Before(r.updatedAt) {
		return errors.New("run: transition time precedes last update")
	}
	return nil
}

func (r Run) ID() identity.RunID                       { return r.id }
func (r Run) SessionID() identity.SessionID            { return r.sessionID }
func (r Run) Lineage() Lineage                         { return r.lineage }
func (r Run) ModelSelection() flamemodel.Selection     { return r.modelSelection }
func (r Run) GoalIncarnationID() goalref.IncarnationID { return r.goalIncarnationID }
func (r Run) State() State {
	if r.lifecycle == nil {
		return ""
	}
	return r.lifecycle.state()
}

func (r Run) ActiveSegmentID() identity.SegmentID {
	active, _ := r.lifecycle.(ActiveSegment)
	return active.ID
}

func (r Run) Executes(segmentID identity.SegmentID) bool {
	return !segmentID.IsZero() && r.ActiveSegmentID() == segmentID
}

func (r Run) termination() Termination {
	terminal, _ := r.lifecycle.(Termination)
	return terminal
}

func (r Run) Outcome() (Outcome, bool) {
	ending, ok := r.Ending()
	if !ok {
		return "", false
	}
	return ending.Outcome(), true
}

func (r Run) Ending() (Ending, bool) {
	terminal, ok := r.lifecycle.(Termination)
	return terminal.Ending, ok
}

func (r Run) CancellationDetail() string {
	canceled, _ := r.termination().Ending.(Canceled)
	return canceled.Detail
}

func (r Run) Canceled() bool {
	_, canceled := r.termination().Ending.(Canceled)
	return canceled
}

func (r Run) Failure() (Failure, bool) {
	failed, ok := r.termination().Ending.(Failed)
	return failed.Failure, ok
}

func (r Run) Loss() (Loss, bool) {
	lost, ok := r.termination().Ending.(Lost)
	return lost.Loss, ok
}

func (r Run) Metrics() Metrics     { return r.metrics }
func (r Run) ContextTokens() int64 { return r.contextTokens }

func (r Run) Capabilities() Capabilities { return r.capabilities.Clone() }
func (r Run) CreatedAt() time.Time       { return r.createdAt }
func (r Run) FinishedAt() time.Time      { return r.termination().FinishedAt }
func (r Run) UpdatedAt() time.Time       { return r.updatedAt }
func (r Run) MessageMark() MessageMark   { return r.termination().MessageMark }

func (r Run) UnresolvedEffects() []UnresolvedEffect {
	return slices.Clone(r.termination().UnresolvedEffects)
}

func (r Run) CheckResumeCapabilities(caller Capabilities) error {
	if gap := r.capabilities.MissingFrom(caller); !gap.IsEmpty() {
		return &InsufficientCapabilitiesError{Missing: gap}
	}
	return nil
}

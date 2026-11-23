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
	state             State
	activeSegmentID   identity.SegmentID
	ending            Ending
	unresolvedEffects []UnresolvedEffect
	metrics           Metrics
	contextTokens     int64
	capabilities      Capabilities
	createdAt         time.Time
	finishedAt        time.Time
	updatedAt         time.Time
	messageMark       MessageMark
}

type Snapshot struct {
	SessionID         identity.SessionID
	ID                identity.RunID
	Lineage           Lineage
	ModelSelection    flamemodel.Selection
	GoalIncarnationID goalref.IncarnationID
	State             State
	ActiveSegmentID   identity.SegmentID
	Ending            Ending
	UnresolvedEffects []UnresolvedEffect
	Metrics           Metrics
	ContextTokens     int64
	Capabilities      Capabilities
	CreatedAt         time.Time
	FinishedAt        time.Time
	UpdatedAt         time.Time
	MessageMark       MessageMark
}

func Admit(draft Draft) (Run, error) {
	if err := draft.Validate(); err != nil {
		return Run{}, err
	}
	return Restore(Snapshot{
		SessionID: draft.SessionID, ID: draft.RunID, Lineage: draft.Lineage(),
		ModelSelection: draft.ModelSelection, GoalIncarnationID: draft.GoalIncarnationID,
		State: Running, ActiveSegmentID: draft.SegmentID,
		Capabilities: draft.Capabilities, CreatedAt: draft.CreatedAt.UTC(),
		UpdatedAt: draft.CreatedAt.UTC(), MessageMark: UnknownMessageMark(),
	})
}

func Restore(snapshot Snapshot) (Run, error) {
	run := Run{
		sessionID: snapshot.SessionID, id: snapshot.ID, lineage: snapshot.Lineage,
		modelSelection: snapshot.ModelSelection, goalIncarnationID: snapshot.GoalIncarnationID,
		state: snapshot.State, activeSegmentID: snapshot.ActiveSegmentID,
		ending: snapshot.Ending, unresolvedEffects: slices.Clone(snapshot.UnresolvedEffects), metrics: snapshot.Metrics,
		contextTokens: snapshot.ContextTokens,
		capabilities:  snapshot.Capabilities.Clone(),
		createdAt:     snapshot.CreatedAt.UTC(), finishedAt: snapshot.FinishedAt.UTC(),
		updatedAt: snapshot.UpdatedAt.UTC(), messageMark: snapshot.MessageMark,
	}
	if err := run.validate(); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (r Run) Snapshot() Snapshot {
	return Snapshot{
		SessionID: r.sessionID, ID: r.id, Lineage: r.lineage,
		ModelSelection: r.modelSelection, GoalIncarnationID: r.goalIncarnationID,
		State: r.state, ActiveSegmentID: r.activeSegmentID,
		Ending: r.ending, UnresolvedEffects: slices.Clone(r.unresolvedEffects), Metrics: r.metrics,
		ContextTokens: r.contextTokens,
		Capabilities:  r.capabilities.Clone(),
		CreatedAt:     r.createdAt, FinishedAt: r.finishedAt,
		UpdatedAt: r.updatedAt, MessageMark: r.messageMark,
	}
}

func (r Run) Fork(sessionID identity.SessionID, id identity.RunID, lineage Lineage) (Run, error) {
	if !r.state.IsTerminal() {
		return Run{}, errors.New("run: only a terminal Run can be forked")
	}
	if r.lineage.IsRoot() != lineage.IsRoot() {
		return Run{}, errors.New("run: fork changes root/child lineage kind")
	}
	snapshot := r.Snapshot()
	snapshot.SessionID = sessionID
	snapshot.ID = id
	snapshot.Lineage = lineage
	snapshot.GoalIncarnationID = goalref.IncarnationID{}
	return Restore(snapshot)
}

func (r Run) Equal(other Run) bool {
	return r.sameIdentity(other) && r.sameProgress(other) &&
		r.createdAt.Equal(other.createdAt) && r.finishedAt.Equal(other.finishedAt) && r.updatedAt.Equal(other.updatedAt)
}

func (r Run) sameIdentity(other Run) bool {
	return r.sessionID == other.sessionID && r.id == other.id && r.lineage == other.lineage &&
		r.modelSelection.Equal(other.modelSelection) && r.goalIncarnationID == other.goalIncarnationID &&
		r.capabilities.Equal(other.capabilities)
}

func (r Run) sameProgress(other Run) bool {
	return r.state == other.state && r.activeSegmentID == other.activeSegmentID && r.ending == other.ending &&
		r.metrics.Equal(other.metrics) && r.contextTokens == other.contextTokens && r.messageMark == other.messageMark &&
		slices.Equal(r.unresolvedEffects, other.unresolvedEffects)
}

func (r Run) validate() error {
	if !r.state.Valid() {
		return fmt.Errorf("run: invalid state %q", r.state)
	}
	if err := r.validateIdentity(); err != nil {
		return err
	}
	if err := r.validateAccrual(); err != nil {
		return err
	}
	if err := r.validateRootOwnership(); err != nil {
		return err
	}
	if (r.state == Running) == r.activeSegmentID.IsZero() {
		return fmt.Errorf("run: %s Run has active Segment %q", r.state, r.activeSegmentID)
	}
	if err := validateUnresolvedEffects(r.unresolvedEffects); err != nil {
		return err
	}
	if len(r.unresolvedEffects) > 0 && !r.state.IsTerminal() {
		return errors.New("run: open run carries terminal effect evidence")
	}
	if r.state.IsTerminal() {
		return r.validateTerminal()
	}
	return r.validateOpen()
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
	if err := instant.Validate(r.createdAt, r.updatedAt, r.finishedAt); err != nil {
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
	case r.messageMark != UnknownMessageMark():
		return errors.New("run: child carries the conversation watermark its root owns")
	}
	return nil
}

func (r Run) validateOpen() error {
	switch {
	case r.ending != nil:
		return fmt.Errorf("run: %s Run carries an ending", r.state)
	case !r.finishedAt.IsZero():
		return fmt.Errorf("run: %s Run carries finish time", r.state)
	case r.messageMark != UnknownMessageMark():
		return fmt.Errorf("run: %s Run carries message watermark %s", r.state, r.messageMark)
	}
	return nil
}

func (r Run) validateTerminal() error {
	if err := ValidateEnding(r.ending); err != nil {
		return err
	}
	switch {
	case r.finishedAt.IsZero():
		return errors.New("run: terminal Run has no finish time")
	case r.finishedAt.Before(r.createdAt):
		return errors.New("run: finish time precedes creation")
	case r.updatedAt.Before(r.finishedAt):
		return errors.New("run: update time precedes finish")
	case r.messageMark.known && r.messageMark.count < 0:
		return fmt.Errorf("run: terminal Run has message watermark %s", r.messageMark)
	}
	return nil
}

func (r Run) AdvanceProgress(metrics Metrics, contextTokens int64, updatedAt time.Time) (Run, error) {
	if !r.state.AdvanceProgress() {
		return Run{}, fmt.Errorf("run: cannot advance progress from state %q", r.state)
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
	next, ok := r.state.Suspend()
	if !ok {
		return Run{}, fmt.Errorf("run: cannot suspend %s Run", r.state)
	}
	if err := r.validateTransitionTime(updatedAt); err != nil {
		return Run{}, err
	}
	r.state, r.activeSegmentID, r.updatedAt = next, identity.SegmentID{}, updatedAt.UTC()
	return r, nil
}

func (r Run) Resume(segmentID identity.SegmentID, resumedAt time.Time) (Run, error) {
	next, ok := r.state.Resume()
	if !ok {
		return Run{}, fmt.Errorf("run: cannot resume %s Run", r.state)
	}
	if segmentID.IsZero() {
		return Run{}, errors.New("run: continuation Segment is required")
	}
	if err := r.validateTransitionTime(resumedAt); err != nil {
		return Run{}, err
	}
	r.state, r.activeSegmentID, r.updatedAt = next, segmentID, resumedAt.UTC()
	return r, nil
}

type Termination struct {
	Ending            Ending
	UnresolvedEffects []UnresolvedEffect
	FinishedAt        time.Time
	MessageMark       MessageMark
}

func (r Run) Terminate(termination Termination) (Run, error) {
	if err := ValidateEnding(termination.Ending); err != nil {
		return Run{}, err
	}
	next, ok := r.state.Terminate(termination.Ending.Outcome())
	if !ok {
		return Run{}, fmt.Errorf("run: outcome %s cannot terminate %s Run", termination.Ending.Outcome(), r.state)
	}
	return r.finish(next, termination)
}

func (r Run) CancelWaiting(detail string, finishedAt time.Time, messageMark MessageMark) (Run, error) {
	if r.state != Waiting {
		return Run{}, fmt.Errorf("run: cannot cancel %s Run as waiting", r.state)
	}
	return r.finish(Finished, Termination{Ending: Canceled{Detail: detail}, FinishedAt: finishedAt, MessageMark: messageMark})
}

func (r Run) RecoverLost(loss Loss, finishedAt time.Time, messageMark MessageMark) (Run, error) {
	next, ok := r.state.RecoverLost()
	if !ok {
		return Run{}, fmt.Errorf("run: cannot recover terminal %s Run as lost", r.state)
	}
	return r.finish(next, Termination{Ending: Lost{Loss: loss}, FinishedAt: finishedAt, MessageMark: messageMark})
}

func (r Run) finish(state State, termination Termination) (Run, error) {
	if err := r.validateTransitionTime(termination.FinishedAt); err != nil {
		return Run{}, err
	}
	r.state, r.activeSegmentID = state, identity.SegmentID{}
	r.ending = termination.Ending
	r.finishedAt, r.updatedAt = termination.FinishedAt.UTC(), termination.FinishedAt.UTC()
	r.messageMark = termination.MessageMark
	r.unresolvedEffects = slices.Clone(termination.UnresolvedEffects)
	if err := r.validate(); err != nil {
		return Run{}, err
	}
	return r, nil
}

func (r Run) RebaseConversation(compaction conversation.Compaction) (Run, error) {
	if !r.state.IsTerminal() || r.lineage.IsChild() {
		return r, nil
	}
	count, known := r.messageMark.Count()
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
	if !r.state.IsTerminal() {
		return Run{}, errors.New("run: only terminal Run can resolve message watermark")
	}
	if messageMark < 0 {
		return Run{}, errors.New("run: message watermark must not be negative")
	}
	r.messageMark = MessageMarkAt(messageMark)
	if err := r.validate(); err != nil {
		return Run{}, err
	}
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
func (r Run) State() State                             { return r.state }
func (r Run) ActiveSegmentID() identity.SegmentID      { return r.activeSegmentID }

func (r Run) Executes(segmentID identity.SegmentID) bool {
	return !segmentID.IsZero() && r.activeSegmentID == segmentID
}

func (r Run) Outcome() (Outcome, bool) {
	if r.ending == nil {
		return "", false
	}
	return r.ending.Outcome(), true
}

func (r Run) Ending() (Ending, bool) { return r.ending, r.ending != nil }

func (r Run) Detail() string {
	canceled, _ := r.ending.(Canceled)
	return canceled.Detail
}

func (r Run) Canceled() bool {
	_, canceled := r.ending.(Canceled)
	return canceled
}

func (r Run) Failure() (Failure, bool) {
	failed, ok := r.ending.(Failed)
	return failed.Failure, ok
}

func (r Run) Loss() (Loss, bool) {
	lost, ok := r.ending.(Lost)
	return lost.Loss, ok
}

func (r Run) Metrics() Metrics     { return r.metrics }
func (r Run) ContextTokens() int64 { return r.contextTokens }

func (r Run) Capabilities() Capabilities { return r.capabilities.Clone() }
func (r Run) CreatedAt() time.Time       { return r.createdAt }
func (r Run) FinishedAt() time.Time      { return r.finishedAt }
func (r Run) UpdatedAt() time.Time       { return r.updatedAt }
func (r Run) MessageMark() MessageMark   { return r.messageMark }

func (r Run) UnresolvedEffects() []UnresolvedEffect { return slices.Clone(r.unresolvedEffects) }

func (r Run) CheckResumeCapabilities(caller Capabilities) error {
	if gap := r.capabilities.MissingFrom(caller); !gap.IsEmpty() {
		return &InsufficientCapabilitiesError{Missing: gap}
	}
	return nil
}

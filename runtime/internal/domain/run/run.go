package run

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
)

// ErrIdentityConflict reports an attempt to reuse a durable Run identity for a
// different Session.
var ErrIdentityConflict = errors.New("run: identity conflict")

// MessageMark is the conversation length where a whole Run tree ended. Only a
// terminal root records one; a child, and a Run whose final count is not
// known yet, carries none. The zero value is that absence, so it cannot be
// confused with a real count, including zero.
type MessageMark struct {
	count int
	known bool
}

// UnknownMessageMark() names the absent watermark.
func UnknownMessageMark() MessageMark { return MessageMark{} }

// MessageMarkAt records a known conversation length.
func MessageMarkAt(count int) MessageMark { return MessageMark{count: count, known: true} }

// Count returns the recorded conversation length, if one is known.
func (m MessageMark) Count() (int, bool) { return m.count, m.known }

func (m MessageMark) String() string {
	if !m.known {
		return "unknown"
	}
	return strconv.Itoa(m.count)
}

// Run is one logical unit of agent work from admission through any waiting and
// resume boundaries to exactly one terminal outcome. Its fields are private so
// every lifecycle and accounting change crosses a validated domain behavior.
type Run struct {
	sessionID         string
	id                string
	lineage           Lineage
	modelSelection    modelchoice.Selection
	goalIncarnationID goalref.IncarnationID
	state             State
	activeSegmentID   string
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

// Snapshot is the complete immutable value needed to restore or persist a Run.
// Restore validates every field; it is not a second mutation surface.
type Snapshot struct {
	SessionID         string
	ID                string
	Lineage           Lineage
	ModelSelection    modelchoice.Selection
	GoalIncarnationID string
	State             State
	ActiveSegmentID   string
	// Ending is present exactly when State is Finished.
	Ending            Ending
	UnresolvedEffects []UnresolvedEffect
	Metrics           Metrics
	// ContextTokens is the latest completed model request's prompt footprint.
	// Zero means no authoritative footprint has been observed yet.
	ContextTokens int64
	Capabilities  Capabilities
	CreatedAt     time.Time
	FinishedAt    time.Time
	UpdatedAt     time.Time
	MessageMark   MessageMark
}

// Admit creates the authoritative aggregate for a fresh root or child Run.
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

// Restore rebuilds a Run from durable values and rejects an invalid snapshot.
func Restore(snapshot Snapshot) (Run, error) {
	goalIncarnationID, _, err := goalref.ParseOptionalIncarnation(snapshot.GoalIncarnationID)
	if err != nil {
		return Run{}, fmt.Errorf("run: %w", err)
	}
	run := Run{
		sessionID: snapshot.SessionID, id: snapshot.ID, lineage: snapshot.Lineage,
		modelSelection: snapshot.ModelSelection, goalIncarnationID: goalIncarnationID,
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

// Snapshot returns a complete ownership-isolated value.
func (r Run) Snapshot() Snapshot {
	return Snapshot{
		SessionID: r.sessionID, ID: r.id, Lineage: r.lineage,
		ModelSelection: r.modelSelection, GoalIncarnationID: r.goalIncarnationID.String(),
		State: r.state, ActiveSegmentID: r.activeSegmentID,
		Ending: r.ending, UnresolvedEffects: slices.Clone(r.unresolvedEffects), Metrics: r.metrics,
		ContextTokens: r.contextTokens,
		Capabilities:  r.capabilities.Clone(),
		CreatedAt:     r.createdAt, FinishedAt: r.finishedAt,
		UpdatedAt: r.updatedAt, MessageMark: r.messageMark,
	}
}

// Fork derives a terminal historical Run for a child Session under fresh
// identities. It preserves the completed work facts, replaces root/child
// lineage with the caller-resolved equivalent, and clears Goal attribution
// because a Session branch does not inherit the parent's Goal incarnation.
func (r Run) Fork(sessionID, id string, lineage Lineage) (Run, error) {
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
	snapshot.GoalIncarnationID = ""
	return Restore(snapshot)
}

// Equal reports whether two values contain the same authoritative Run facts.
// It is useful at persistence and hand-off boundaries that must prove a
// proposed transition was derived from the currently committed aggregate.
func (r Run) Equal(other Run) bool {
	if r.sessionID != other.sessionID || r.id != other.id || r.lineage != other.lineage ||
		!r.modelSelection.Equal(other.modelSelection) || r.goalIncarnationID != other.goalIncarnationID ||
		r.state != other.state || r.activeSegmentID != other.activeSegmentID ||
		r.ending != other.ending || !r.metrics.Equal(other.metrics) ||
		r.contextTokens != other.contextTokens ||
		!r.capabilities.Equal(other.capabilities) || !r.createdAt.Equal(other.createdAt) ||
		!r.finishedAt.Equal(other.finishedAt) || !r.updatedAt.Equal(other.updatedAt) ||
		r.messageMark != other.messageMark || !slices.Equal(r.unresolvedEffects, other.unresolvedEffects) {
		return false
	}
	return true
}

func (r Run) validate() error {
	if err := validateUnresolvedEffects(r.unresolvedEffects); err != nil {
		return err
	}
	if len(r.unresolvedEffects) > 0 && !r.state.IsTerminal() {
		return errors.New("run: open run carries terminal effect evidence")
	}

	if !r.state.Valid() {
		return fmt.Errorf("run: invalid state %q", r.state)
	}
	if err := identity.ValidateRun(r.id); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	if err := identity.ValidateSession(r.sessionID); err != nil {
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
	if err := r.lineage.Validate(r.id); err != nil {
		return err
	}
	if err := instant.Validate(r.createdAt, r.updatedAt, r.finishedAt); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	if err := r.modelSelection.ValidateExact(); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	if r.lineage.IsChild() && r.goalIncarnationID.String() != "" {
		return errors.New("run: child carries a root Goal incarnation")
	}
	if r.activeSegmentID != "" {
		if err := identity.ValidateSegment(r.activeSegmentID); err != nil {
			return fmt.Errorf("run: active %w", err)
		}
	}
	if (r.state == Running) != (r.activeSegmentID != "") {
		return fmt.Errorf("run: %s Run has active Segment %q", r.state, r.activeSegmentID)
	}
	if err := r.metrics.Validate(); err != nil {
		return err
	}
	if r.contextTokens < 0 {
		return errors.New("run: context tokens must not be negative")
	}
	if err := r.capabilities.Validate(); err != nil {
		return err
	}
	if r.lineage.IsChild() && !r.capabilities.IsEmpty() {
		return errors.New("run: child carries capabilities its root owns")
	}
	if r.lineage.IsChild() && r.messageMark != UnknownMessageMark() {
		return errors.New("run: child carries the conversation watermark its root owns")
	}
	if r.state.IsTerminal() {
		return r.validateTerminal()
	}
	return r.validateOpen()
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
	if err := validateEnding(r.ending); err != nil {
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

// AdvanceProgress returns a Run with one model-response boundary committed.
// Metrics are cumulative and must remain monotonic. contextTokens is a latest
// point-in-time prompt footprint and may decrease after compaction; zero means
// the provider supplied no authoritative footprint, so the prior value remains.
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

// Suspend parks a Running Run and clears its active Segment.
func (r Run) Suspend(updatedAt time.Time) (Run, error) {
	next, ok := r.state.Suspend()
	if !ok {
		return Run{}, fmt.Errorf("run: cannot suspend %s Run", r.state)
	}
	if err := r.validateTransitionTime(updatedAt); err != nil {
		return Run{}, err
	}
	r.state, r.activeSegmentID, r.updatedAt = next, "", updatedAt.UTC()
	return r, nil
}

// Resume opens a fresh Segment for a Waiting Run.
func (r Run) Resume(segmentID string, resumedAt time.Time) (Run, error) {
	next, ok := r.state.Resume()
	if !ok {
		return Run{}, fmt.Errorf("run: cannot resume %s Run", r.state)
	}
	if err := identity.ValidateSegment(segmentID); err != nil {
		return Run{}, fmt.Errorf("run: continuation %w", err)
	}
	if err := r.validateTransitionTime(resumedAt); err != nil {
		return Run{}, err
	}
	r.state, r.activeSegmentID, r.updatedAt = next, segmentID, resumedAt.UTC()
	return r, nil
}

// Termination is the complete fact set required to finish a Run.
type Termination struct {
	Ending            Ending
	UnresolvedEffects []UnresolvedEffect
	FinishedAt        time.Time
	MessageMark       MessageMark
}

// Terminate finishes a Running Run with one coherent terminal fact set.
func (r Run) Terminate(termination Termination) (Run, error) {
	if err := validateEnding(termination.Ending); err != nil {
		return Run{}, err
	}
	next, ok := r.state.Terminate(termination.Ending.Outcome())
	if !ok {
		return Run{}, fmt.Errorf("run: outcome %s cannot terminate %s Run", termination.Ending.Outcome(), r.state)
	}
	return r.finish(next, termination)
}

// CancelWaiting finishes a Waiting Run as canceled.
func (r Run) CancelWaiting(detail string, finishedAt time.Time, messageMark MessageMark) (Run, error) {
	if r.state != Waiting {
		return Run{}, fmt.Errorf("run: cannot cancel %s Run as waiting", r.state)
	}
	return r.finish(Finished, Termination{Ending: Canceled{Detail: detail}, FinishedAt: finishedAt, MessageMark: messageMark})
}

// RecoverLost finishes a non-terminal Run whose executor state cannot be recovered.
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
	r.state, r.activeSegmentID = state, ""
	r.ending = termination.Ending
	r.finishedAt, r.updatedAt = termination.FinishedAt.UTC(), termination.FinishedAt.UTC()
	r.messageMark = termination.MessageMark
	r.unresolvedEffects = slices.Clone(termination.UnresolvedEffects)
	if err := r.validate(); err != nil {
		return Run{}, err
	}
	return r, nil
}

// WithMessageMark replaces a terminal Run's conversation coordinate without
// changing any other fact. Terminalization uses it after obtaining the final
// count; conversation compaction uses it to rebase an already-final boundary
// into the replacement history's coordinate space.
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

func (r Run) ID() string                            { return r.id }
func (r Run) SessionID() string                     { return r.sessionID }
func (r Run) Lineage() Lineage                      { return r.lineage }
func (r Run) ModelSelection() modelchoice.Selection { return r.modelSelection }
func (r Run) GoalIncarnationID() string             { return r.goalIncarnationID.String() }
func (r Run) State() State                          { return r.state }
func (r Run) ActiveSegmentID() string               { return r.activeSegmentID }
func (r Run) Outcome() (Outcome, bool) {
	if r.ending == nil {
		return "", false
	}
	return r.ending.Outcome(), true
}

// Ending returns how a finished Run ended.
func (r Run) Ending() (Ending, bool) { return r.ending, r.ending != nil }

// Detail returns a canceled Run's stated reason.
func (r Run) Detail() string {
	canceled, _ := r.ending.(Canceled)
	return canceled.Detail
}

// Canceled reports whether the Run finished because it was canceled.
func (r Run) Canceled() bool {
	_, canceled := r.ending.(Canceled)
	return canceled
}

// Failure returns a failed Run's classified failure.
func (r Run) Failure() (Failure, bool) {
	failed, ok := r.ending.(Failed)
	return failed.Failure, ok
}

// Loss returns why a lost Run's outcome cannot be established.
func (r Run) Loss() (Loss, bool) {
	lost, ok := r.ending.(Lost)
	return lost.Loss, ok
}

func (r Run) Metrics() Metrics     { return r.metrics }
func (r Run) ContextTokens() int64 { return r.contextTokens }

// Capabilities is a root Run's frozen optional behavior. A child carries none:
// it follows its root's.
func (r Run) Capabilities() Capabilities { return r.capabilities.Clone() }
func (r Run) CreatedAt() time.Time       { return r.createdAt }
func (r Run) FinishedAt() time.Time      { return r.finishedAt }
func (r Run) UpdatedAt() time.Time       { return r.updatedAt }
func (r Run) MessageMark() MessageMark   { return r.messageMark }

func (r Run) UnresolvedEffects() []UnresolvedEffect { return slices.Clone(r.unresolvedEffects) }

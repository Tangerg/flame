package runs

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	corechat "github.com/Tangerg/scope/core/chat"
)

type StateChange string

const (
	// StateUnchanged is the meaningful zero value: the commit advances other
	// durable Run facts without moving the lifecycle state.
	StateUnchanged   StateChange = ""
	StateSuspend     StateChange = "suspend"
	StateTerminalize StateChange = "terminalize"
)

// Valid reports whether s is one supported lifecycle mutation.
func (s StateChange) Valid() bool {
	return s == StateUnchanged || s == StateSuspend || s == StateTerminalize
}

// ModelInvocationState records the durable application observation of one
// provider call. It is deliberately smaller than a model response: semantic
// output belongs to Transcript Items and accounting belongs to ProgressCommit.
// This record exists to distinguish an invocation that never crossed the
// provider boundary from one whose final projection became indeterminate.
type ModelInvocationState string

const (
	ModelInvocationStarted   ModelInvocationState = "started"
	ModelInvocationCompleted ModelInvocationState = "completed"
	ModelInvocationFailed    ModelInvocationState = "failed"
	ModelInvocationUnknown   ModelInvocationState = "unknown"
)

// Valid reports whether m belongs to the durable model-invocation journal.
func (m ModelInvocationState) Valid() bool {
	return m == ModelInvocationStarted || m == ModelInvocationCompleted ||
		m == ModelInvocationFailed || m == ModelInvocationUnknown
}

// String returns the durable model-invocation state name.
func (m ModelInvocationState) String() string {
	if !m.Valid() {
		return "invalid"
	}
	return string(m)
}

// ModelInvocationCommit is one monotonic transition in the durable invocation
// journal. StartedAt is repeated on terminal transitions so persistence can
// compare the exact attempt instead of updating whichever row happens to share
// CallID.
type ModelInvocationCommit struct {
	CallID     string
	SegmentID  string
	State      ModelInvocationState
	StartedAt  time.Time
	FinishedAt time.Time
}

// ToolInvocationState records whether one model-requested Tool call has only
// started, reached a definite result, or was closed without one at a Run
// boundary. Final Tool content still has exactly one owner: the Transcript Item
// committed beside the terminal transition.
type ToolInvocationState string

const (
	ToolInvocationStarted    ToolInvocationState = "started"
	ToolInvocationCompleted  ToolInvocationState = "completed"
	ToolInvocationIncomplete ToolInvocationState = "incomplete"
)

// Valid reports whether t belongs to the durable Tool-invocation journal.
func (t ToolInvocationState) Valid() bool {
	return t == ToolInvocationStarted || t == ToolInvocationCompleted || t == ToolInvocationIncomplete
}

// String returns the durable Tool-invocation state name.
func (t ToolInvocationState) String() string {
	if !t.Valid() {
		return "invalid"
	}
	return string(t)
}

// ToolInvocationCommit is the durable pre-call/terminal attempt transition for
// one canonical Tool Item. ItemID connects the operational start boundary to
// the eventual Transcript projection without copying arguments or result data.
type ToolInvocationCommit struct {
	CallID     string
	ItemID     string
	SegmentID  string
	State      ToolInvocationState
	StartedAt  time.Time
	FinishedAt time.Time
}

func (t ToolInvocationCommit) validate() error {
	if _, err := runtimeidentity.ParseEffect(t.CallID); err != nil {
		return fmt.Errorf("runs: Tool invocation: %w", err)
	}
	if _, err := resourceid.ParseItem(t.ItemID); err != nil {
		return fmt.Errorf("runs: Tool invocation: %w", err)
	}
	if _, err := resourceid.ParseSegment(t.SegmentID); err != nil {
		return fmt.Errorf("runs: Tool invocation: %w", err)
	}
	if t.StartedAt.IsZero() {
		return errors.New("runs: Tool invocation start time is required")
	}
	switch t.State {
	case ToolInvocationStarted:
		if !t.FinishedAt.IsZero() {
			return errors.New("runs: started Tool invocation carries a finish time")
		}
	case ToolInvocationCompleted, ToolInvocationIncomplete:
		if t.FinishedAt.IsZero() {
			return errors.New("runs: terminal Tool invocation has no finish time")
		}
		if t.FinishedAt.Before(t.StartedAt) {
			return errors.New("runs: Tool invocation finish time precedes start time")
		}
	default:
		return fmt.Errorf("runs: Tool invocation has unknown state %q", t.State)
	}
	return nil
}

func (m ModelInvocationCommit) validate() error {
	if _, err := runtimeidentity.ParseEffect(m.CallID); err != nil {
		return fmt.Errorf("runs: model invocation: %w", err)
	}
	if _, err := resourceid.ParseSegment(m.SegmentID); err != nil {
		return fmt.Errorf("runs: model invocation: %w", err)
	}
	if m.StartedAt.IsZero() {
		return errors.New("runs: model invocation start time is required")
	}
	switch m.State {
	case ModelInvocationStarted:
		if !m.FinishedAt.IsZero() {
			return errors.New("runs: started model invocation carries a finish time")
		}
	case ModelInvocationCompleted, ModelInvocationFailed, ModelInvocationUnknown:
		if m.FinishedAt.IsZero() {
			return errors.New("runs: terminal model invocation has no finish time")
		}
		if m.FinishedAt.Before(m.StartedAt) {
			return errors.New("runs: model invocation finish time precedes start time")
		}
	default:
		return fmt.Errorf("runs: model invocation has unknown state %q", m.State)
	}
	return nil
}

// ProgressCommit is the durable progress snapshot produced at a model-response
// boundary. Metrics are cumulative; ContextTokens is the latest prompt footprint
// and may decrease after compaction. SegmentID fences both facts to the exact
// running segment so a stale continuation cannot overwrite a newer Run.
type ProgressCommit struct {
	SegmentID     string
	Metrics       run.Metrics
	ContextTokens int64
	UpdatedAt     time.Time
}

func (r ProgressCommit) validate() error {
	if _, err := resourceid.ParseSegment(r.SegmentID); err != nil {
		return fmt.Errorf("runs: progress: %w", err)
	}
	if r.UpdatedAt.IsZero() {
		return errors.New("runs: progress update time is required")
	}
	if err := r.Metrics.Validate(); err != nil {
		return fmt.Errorf("runs: progress metrics: %w", err)
	}
	if r.ContextTokens < 0 {
		return errors.New("runs: progress context tokens must not be negative")
	}
	return nil
}

type EventCommit struct {
	RunID     string
	SessionID string
	// SegmentID owns the complete event write-set, including projections that do
	// not otherwise carry segment identity. Persistence admits the transaction
	// only while this exact Segment is still active for the Run.
	SegmentID string
	// CommitID is the stable identity of one immutable top-level CommitEvent
	// write-set. Persistence records it inside that transaction, allowing a lost
	// COMMIT receipt to be reconciled without treating another Segment or write
	// attempt as success. Nested opening and tree-barrier projections must leave
	// it empty because their parent commit owns the complete transaction identity;
	// the top-level CommitEvent port boundary requires it.
	CommitID runtimeidentity.CommitID
	State    StateChange
	Outcome  run.Outcome
	Items    []transcript.Item
	// ConversationMessages are the provider-neutral messages this root
	// execution made durable for future model context. Conversation and
	// Transcript remain separate projections: the former feeds later model
	// calls, while the latter owns user-visible Run history.
	ConversationMessages []corechat.Message
	// ModelInvocations, ToolInvocations, and Progress are application
	// observations committed in the same transaction as the semantic Transcript
	// Items derived from one authoritative executor fact.
	ModelInvocations []ModelInvocationCommit
	ToolInvocations  []ToolInvocationCommit
	Progress         *ProgressCommit
	Run              *run.Run
	GoalRun          *goal.RunRecord
	// ObsoleteCheckpointRootID identifies the executor checkpoint aggregate the
	// root Run terminal makes obsolete. Child terminal commits leave it empty.
	ObsoleteCheckpointRootID string
}

// clone returns an ownership-isolated copy of one complete event write-set.
func (e EventCommit) clone() EventCommit {
	e.Items = slices.Clone(e.Items)
	e.ConversationMessages = cloneCommitMessages(e.ConversationMessages)
	e.ModelInvocations = slices.Clone(e.ModelInvocations)
	e.ToolInvocations = slices.Clone(e.ToolInvocations)
	if e.Progress != nil {
		progress := *e.Progress
		e.Progress = &progress
	}
	if e.Run != nil {
		run := *e.Run
		e.Run = &run
	}
	if e.GoalRun != nil {
		goalRun := *e.GoalRun
		e.GoalRun = &goalRun
	}
	return e
}

func cloneCommitMessages(messages []corechat.Message) []corechat.Message {
	owned := make([]corechat.Message, len(messages))
	for index, message := range messages {
		owned[index] = message.Clone()
	}
	return owned
}

// Validate proves that one event projection is owner-bound and that any Goal
// charge is exactly the accounting fact implied by its terminal Run.
func (e EventCommit) Validate() error {
	if err := e.validateEnvelope(); err != nil {
		return err
	}
	if err := e.validateItems(); err != nil {
		return err
	}
	if err := e.validateConversationMessages(); err != nil {
		return err
	}
	if err := e.validateInvocations(); err != nil {
		return err
	}
	if e.Progress != nil {
		if err := e.Progress.validate(); err != nil {
			return err
		}
		if e.Progress.SegmentID != e.SegmentID {
			return fmt.Errorf("runs: event commit progress belongs to Segment %q, want %q", e.Progress.SegmentID, e.SegmentID)
		}
	}
	return e.validateLifecycle()
}

func (e EventCommit) validateConversationMessages() error {
	for index, message := range e.ConversationMessages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("runs: event commit conversation message[%d]: %w", index, err)
		}
		if err := conversation.ValidateMessageIdentities(message); err != nil {
			return fmt.Errorf("runs: event commit conversation message[%d]: %w", index, err)
		}
	}
	return nil
}

func (e EventCommit) validateEnvelope() error {
	if _, err := resourceid.ParseRun(e.RunID); err != nil {
		return fmt.Errorf("runs: event commit: %w", err)
	}
	if _, err := resourceid.ParseSession(e.SessionID); err != nil {
		return fmt.Errorf("runs: event commit: %w", err)
	}
	if _, err := resourceid.ParseSegment(e.SegmentID); err != nil {
		return fmt.Errorf("runs: event commit: %w", err)
	}
	if _, _, err := runtimeidentity.ParseOptionalMember(e.ObsoleteCheckpointRootID); err != nil {
		return fmt.Errorf("runs: event commit checkpoint root: %w", err)
	}
	if !e.CommitID.IsZero() {
		if err := e.CommitID.Validate(); err != nil {
			return fmt.Errorf("runs: event commit: %w", err)
		}
	}
	return nil
}

func (e EventCommit) validateItems() error {
	seenItems := make(map[string]struct{}, len(e.Items))
	for index, item := range e.Items {
		if item.ID() == "" || item.RunID() != e.RunID || item.SessionID() != e.SessionID {
			return fmt.Errorf("runs: event commit Item[%d] is not owned by Run %q", index, e.RunID)
		}
		if _, duplicate := seenItems[item.ID()]; duplicate {
			return fmt.Errorf("runs: event commit repeats Item %q", item.ID())
		}
		seenItems[item.ID()] = struct{}{}
		if err := item.Validate(); err != nil {
			return fmt.Errorf("runs: event commit Item %q: %w", item.ID(), err)
		}
	}
	return nil
}

func (e EventCommit) validateInvocations() error {
	items := make(map[string]transcript.Item, len(e.Items))
	for _, item := range e.Items {
		items[item.ID()] = item
	}
	if err := e.validateModelInvocations(); err != nil {
		return err
	}
	return e.validateToolInvocations(items)
}

func (e EventCommit) validateModelInvocations() error {
	seenInvocations := make(map[string]struct{}, len(e.ModelInvocations))
	for index, invocation := range e.ModelInvocations {
		if err := invocation.validate(); err != nil {
			return fmt.Errorf("runs: event commit model invocation[%d]: %w", index, err)
		}
		if _, duplicate := seenInvocations[invocation.CallID]; duplicate {
			return fmt.Errorf("runs: event commit repeats model invocation %q", invocation.CallID)
		}
		if invocation.SegmentID != e.SegmentID {
			return fmt.Errorf("runs: event commit model invocation[%d] belongs to Segment %q, want %q", index, invocation.SegmentID, e.SegmentID)
		}
		seenInvocations[invocation.CallID] = struct{}{}
	}
	return nil
}

func (e EventCommit) validateToolInvocations(items map[string]transcript.Item) error {
	seenTools := make(map[string]struct{}, len(e.ToolInvocations))
	seenToolItems := make(map[string]struct{}, len(e.ToolInvocations))
	for index, invocation := range e.ToolInvocations {
		if err := invocation.validate(); err != nil {
			return fmt.Errorf("runs: event commit Tool invocation[%d]: %w", index, err)
		}
		if _, duplicate := seenTools[invocation.CallID]; duplicate {
			return fmt.Errorf("runs: event commit repeats Tool invocation %q", invocation.CallID)
		}
		if invocation.SegmentID != e.SegmentID {
			return fmt.Errorf("runs: event commit Tool invocation[%d] belongs to Segment %q, want %q", index, invocation.SegmentID, e.SegmentID)
		}
		if _, duplicate := seenToolItems[invocation.ItemID]; duplicate {
			return fmt.Errorf("runs: event commit repeats Tool invocation Item %q", invocation.ItemID)
		}
		seenTools[invocation.CallID] = struct{}{}
		seenToolItems[invocation.ItemID] = struct{}{}
		item, present := items[invocation.ItemID]
		if !present || item.Kind() != transcript.ToolCall {
			return fmt.Errorf(
				"runs: event commit Tool invocation %q has no matching Tool Item",
				invocation.CallID,
			)
		}
		if err := validateToolInvocationItem(invocation, item); err != nil {
			return err
		}
	}
	return nil
}

func validateToolInvocationItem(invocation ToolInvocationCommit, item transcript.Item) error {
	switch invocation.State {
	case ToolInvocationStarted:
		if item.Status() != transcript.ItemRunning {
			return fmt.Errorf("runs: started Tool invocation %q Item is not running", invocation.CallID)
		}
	case ToolInvocationCompleted:
		switch item.Status() {
		case transcript.ItemCompleted:
		case transcript.ItemIncomplete:
			if _, failed := item.Failure(); !failed {
				return fmt.Errorf("runs: completed Tool invocation %q has an unclassified incomplete Item", invocation.CallID)
			}
		default:
			return fmt.Errorf("runs: completed Tool invocation %q Item is not terminal", invocation.CallID)
		}
	case ToolInvocationIncomplete:
		if item.Status() != transcript.ItemIncomplete && item.Status() != transcript.ItemRunning {
			return fmt.Errorf("runs: incomplete Tool invocation %q Item is neither incomplete nor parked", invocation.CallID)
		}
	}
	return nil
}

func (e EventCommit) validateLifecycle() error {
	switch e.State {
	case StateUnchanged:
		if e.Outcome != "" || e.Run != nil || e.GoalRun != nil || e.ObsoleteCheckpointRootID != "" {
			return errors.New("runs: unchanged event commit carries lifecycle facts")
		}
		return nil
	case StateSuspend:
		if e.Run == nil || e.Run.State() != run.Waiting {
			return errors.New("runs: suspend event commit has no waiting Run")
		}
		if e.Outcome != "" || e.GoalRun != nil || e.ObsoleteCheckpointRootID != "" {
			return errors.New("runs: suspend event commit carries terminal facts")
		}
	case StateTerminalize:
		if e.CommitID.IsZero() {
			return errors.New("runs: terminal event commit has no commit identity")
		}
		if e.Run == nil || !e.Run.State().IsTerminal() {
			return errors.New("runs: terminal event commit has no matching terminal Run")
		}
		outcome, ok := e.Run.Outcome()
		if !ok || outcome != e.Outcome {
			return errors.New("runs: terminal event commit has no matching terminal outcome")
		}
	default:
		return fmt.Errorf("runs: event commit has unknown state change %q", e.State)
	}

	if e.Run.ID() != e.RunID || e.Run.SessionID() != e.SessionID {
		return errors.New("runs: event commit Run ownership differs from its envelope")
	}
	validatedRun := *e.Run
	if e.State == StateTerminalize && validatedRun.MessageMark() == run.UnknownMessageMark {
		// The reducer cannot know the final conversation watermark. The terminal
		// transaction resolves it while committing this Run; every other terminal
		// fact must already satisfy the domain invariant.
		var err error
		validatedRun, err = validatedRun.WithMessageMark(0)
		if err != nil {
			return fmt.Errorf("runs: resolve provisional message watermark: %w", err)
		}
	}
	if err := validatedRun.Validate(); err != nil {
		return fmt.Errorf("runs: event commit Run: %w", err)
	}
	if e.State == StateSuspend {
		return nil
	}
	return validateTerminalGoalRun(*e.Run, e.GoalRun)
}

func validateTerminalGoalRun(value run.Run, record *goal.RunRecord) error {
	if value.GoalIncarnationID() == "" {
		if record != nil {
			return fmt.Errorf("runs: non-Goal Run %q carries a Goal Run", value.ID())
		}
		return nil
	}
	if !value.Lineage().IsRoot() {
		return fmt.Errorf("runs: child Run %q carries a root Goal incarnation", value.ID())
	}
	if record == nil {
		return fmt.Errorf("runs: Goal-owned terminal Run %q has no Goal Run", value.ID())
	}
	if err := record.Validate(); err != nil {
		return fmt.Errorf("runs: terminal Goal Run: %w", err)
	}
	cost, err := costFromRunMetrics(value.Metrics())
	if err != nil {
		return fmt.Errorf("runs: terminal Goal Run cost: %w", err)
	}
	outcome, ok := value.Outcome()
	if !ok || record.SessionID != value.SessionID() || record.IncarnationID != value.GoalIncarnationID() ||
		record.RunID != value.ID() || record.Outcome != outcome || !record.Cost.Equal(cost) ||
		record.Steps != value.Metrics().Steps() || !record.CompletedAt.Equal(value.FinishedAt()) {
		return fmt.Errorf("runs: Goal Run differs from terminal Run %q", value.ID())
	}
	return nil
}

func (e EventCommit) isEmpty() bool {
	return len(e.Items) == 0 &&
		len(e.ConversationMessages) == 0 &&
		len(e.ModelInvocations) == 0 &&
		len(e.ToolInvocations) == 0 &&
		e.Progress == nil &&
		e.Outcome == "" &&
		e.Run == nil &&
		e.GoalRun == nil &&
		e.ObsoleteCheckpointRootID == "" &&
		e.State == StateUnchanged
}

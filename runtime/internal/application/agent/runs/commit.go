package runs

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
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

// ModelInvocationCommit is one monotonic transition in the durable invocation
// journal. StartedAt is repeated on terminal transitions so persistence can
// compare the exact attempt instead of updating whichever row happens to share
// CallID.
type ModelInvocationCommit struct {
	Purpose                  run.ModelCallPurpose
	KnownResponse            *corechat.Response
	FirstOutputLatencyMillis *int64
	// Usage is the provider's own report for this call, so an unsupported
	// breakdown stays absent rather than being stored as a reported zero.
	Usage      *corechat.Usage
	CallID     string
	SegmentID  identity.SegmentID
	State      modelref.InvocationState
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

// Settled reports a Tool attempt that reached its boundary, with or without a
// definite result.
func (t ToolInvocationState) Settled() bool {
	return t == ToolInvocationCompleted || t == ToolInvocationIncomplete
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
	Caller     *plugin.ToolCaller
	CallID     string
	ItemID     identity.ItemID
	SegmentID  identity.SegmentID
	State      ToolInvocationState
	StartedAt  time.Time
	FinishedAt time.Time
}

func (t ToolInvocationCommit) Validate() error {
	if t.Caller != nil {
		if t.State != ToolInvocationStarted {
			return errors.New("runs: only Tool dispatch admission may capture a caller")
		}
		if err := t.Caller.Validate(); err != nil {
			return err
		}
		if t.Caller.Snapshot().Invocation.ID != t.CallID {
			return plugin.ErrCommandConflict
		}
	}
	if err := ValidateInvocation(t.CallID); err != nil {
		return fmt.Errorf("runs: Tool invocation: %w", err)
	}
	if err := t.ItemID.Validate(); err != nil {
		return fmt.Errorf("runs: Tool invocation: %w", err)
	}
	if err := t.SegmentID.Validate(); err != nil {
		return fmt.Errorf("runs: Tool invocation: %w", err)
	}
	if !t.State.Valid() {
		return fmt.Errorf("runs: Tool invocation has unknown state %q", t.State)
	}
	return validateAttemptTiming("Tool invocation", t.State.Settled(), t.StartedAt, t.FinishedAt)
}

// validateAttemptTiming requires every attempt to have started, an unsettled
// one to carry no finish time, and a settled one to finish no earlier than it
// started.
func validateAttemptTiming(subject string, settled bool, startedAt, finishedAt time.Time) error {
	switch {
	case startedAt.IsZero():
		return fmt.Errorf("runs: %s start time is required", subject)
	case !settled && !finishedAt.IsZero():
		return fmt.Errorf("runs: started %s carries a finish time", subject)
	case settled && finishedAt.IsZero():
		return fmt.Errorf("runs: terminal %s has no finish time", subject)
	case settled && finishedAt.Before(startedAt):
		return fmt.Errorf("runs: %s finish time precedes start time", subject)
	}
	return nil
}

func (m ModelInvocationCommit) Validate() error {
	if err := m.Purpose.Validate(); err != nil {
		return err
	}
	if err := m.State.Validate(); err != nil {
		return fmt.Errorf("runs: model invocation: %w", err)
	}
	if err := m.validateEvidence(); err != nil {
		return err
	}
	if err := ValidateInvocation(m.CallID); err != nil {
		return fmt.Errorf("runs: model invocation: %w", err)
	}
	if err := m.SegmentID.Validate(); err != nil {
		return fmt.Errorf("runs: model invocation: %w", err)
	}
	return validateAttemptTiming("model invocation", m.State.Settled(), m.StartedAt, m.FinishedAt)
}

// validateEvidence admits only what the outcome can carry: latency needs a
// definite outcome, usage needs a known response, and a known response can
// only explain a failed call.
func (m ModelInvocationCommit) validateEvidence() error {
	if m.FirstOutputLatencyMillis != nil && (*m.FirstOutputLatencyMillis < 0 || !m.State.HasResponse()) {
		return errors.New("runs: first output latency requires a nonnegative measurement and a definite model outcome")
	}
	if m.Usage != nil {
		if m.State != modelref.InvocationCompleted && (m.State != modelref.InvocationFailed || m.KnownResponse == nil) {
			return errors.New("runs: model invocation usage requires a known response")
		}
		if err := m.Usage.Validate(); err != nil {
			return fmt.Errorf("runs: model invocation usage: %w", err)
		}
	}
	if m.KnownResponse == nil {
		return nil
	}
	if m.State != modelref.InvocationFailed {
		return errors.New("runs: known response evidence requires a failed outer model invocation")
	}
	if err := m.KnownResponse.Validate(); err != nil {
		return err
	}
	if !modelref.UsageMatchesResponse(m.Usage, m.KnownResponse) {
		return errors.New("runs: usage differs from known response evidence")
	}
	return nil
}

type EventCommit struct {
	Compaction        *ConversationCompactionPlan
	ExecutionContext  *ExecutorCheckpoint
	ResultPublication *ResultPublication
	ToolResults       []corechat.ToolResult
	RunID             identity.RunID
	SessionID         identity.SessionID
	// SegmentID owns the complete event write-set, including projections that do
	// not otherwise carry segment identity. Persistence admits the transaction
	// only while this exact Segment is still active for the Run.
	SegmentID identity.SegmentID
	// CommitID is the stable identity of one immutable top-level CommitEvent
	// write-set. Persistence records it inside that transaction, allowing a lost
	// COMMIT receipt to be reconciled without treating another Segment or write
	// attempt as success. Nested opening and tree-barrier projections must leave
	// it empty because their parent commit owns the complete transaction identity;
	// the top-level CommitEvent port boundary requires it.
	CommitID CommitID
	State    StateChange
	Items    []ItemWrite
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
	// Progress is the running Run as the domain advanced it at a model-response
	// boundary. Persistence writes it fenced to its active Segment and does not
	// re-derive the advance.
	Progress               *run.Run
	Run                    *run.Run
	rootCancellation       *rootCancellationCommit
	childCancellation      *childCancellationCommit
	cancellation           CancellationID
	winningCancellationKey string
	// ObsoleteCheckpointRootID identifies the executor checkpoint aggregate the
	// root Run terminal makes obsolete. Child terminal commits leave it empty.
	ObsoleteCheckpointRootID string
}

func (e EventCommit) WinningCancellationKey() string { return e.winningCancellationKey }

// clone returns an ownership-isolated copy of one complete event write-set.
func (e EventCommit) clone() EventCommit {
	if e.ExecutionContext != nil {
		e.ExecutionContext = new(e.ExecutionContext.Clone())
	}
	if e.ResultPublication != nil {
		e.ResultPublication = new(*e.ResultPublication)
	}
	e.ToolResults = cloneToolResults(e.ToolResults)
	e.Items = slices.Clone(e.Items)
	e.ConversationMessages = cloneCommitMessages(e.ConversationMessages)
	e.ModelInvocations = slices.Clone(e.ModelInvocations)
	for index := range e.ModelInvocations {
		e.ModelInvocations[index].KnownResponse = e.ModelInvocations[index].KnownResponse.Clone()
		if latency := e.ModelInvocations[index].FirstOutputLatencyMillis; latency != nil {
			e.ModelInvocations[index].FirstOutputLatencyMillis = new(*latency)
		}
		if usage := e.ModelInvocations[index].Usage; usage != nil {
			copy := usage.Clone()
			e.ModelInvocations[index].Usage = &copy
		}
	}
	e.ToolInvocations = slices.Clone(e.ToolInvocations)
	for index := range e.ToolInvocations {
		if e.ToolInvocations[index].Caller != nil {
			e.ToolInvocations[index].Caller = new(*e.ToolInvocations[index].Caller)
		}
	}
	if e.Progress != nil {
		e.Progress = new(*e.Progress)
	}
	if e.Run != nil {
		e.Run = new(*e.Run)
	}
	return e
}

func cloneToolResults(results []corechat.ToolResult) []corechat.ToolResult {
	owned := make([]corechat.ToolResult, len(results))
	for index, result := range results {
		owned[index] = result.Clone()
	}
	return owned
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
	for _, validate := range []func() error{
		e.validateToolResults,
		e.validateCompaction,
		e.validateResultPublication,
		e.validateEnvelope,
		e.validateCancellationEvidence,
		e.validateItems,
		e.validateChildCancellation,
		e.validateConversationMessages,
		e.validateInvocations,
		e.validateProgress,
		e.validateLifecycle,
	} {
		if err := validate(); err != nil {
			return err
		}
	}
	return nil
}

func (e EventCommit) validateToolResults() error {
	if len(e.ToolResults) > 0 && e.ResultPublication == nil {
		return errors.New("runs: exact Tool results require a publication receipt")
	}
	for index, result := range e.ToolResults {
		if err := result.Validate(); err != nil {
			return fmt.Errorf("runs: exact Tool result[%d]: %w", index, err)
		}
	}
	return nil
}

func (e EventCommit) validateCompaction() error {
	if e.Compaction == nil {
		return nil
	}
	if e.Compaction.SessionID() != e.SessionID {
		return errors.New("runs: compaction belongs to another Session")
	}
	if err := e.Compaction.validateOwner(e.RunID, e.SegmentID); err != nil {
		return err
	}
	if e.State != StateUnchanged || len(e.Items) != 1 || e.Items[0].Item().Kind() != transcript.Compaction || len(e.ConversationMessages) != 0 {
		return errors.New("runs: compaction requires one Compaction Item in its history transaction")
	}
	return nil
}

func (e EventCommit) validateResultPublication() error {
	if e.ResultPublication == nil {
		return nil
	}
	if err := e.ResultPublication.Validate(); err != nil {
		return err
	}
	if e.State != StateUnchanged || len(e.ToolInvocations) == 0 || len(e.ToolResults) != len(e.ToolInvocations) {
		return errors.New("runs: result publication requires a Tool result transaction")
	}
	return nil
}

func (e EventCommit) validateCancellationEvidence() error {
	canceledTerminal := e.State == StateTerminalize && e.Run != nil && e.Run.Canceled()
	switch {
	case !e.cancellation.IsZero() && !canceledTerminal:
		return errors.New("runs: cancellation evidence requires a canceled terminal")
	case e.winningCancellationKey != "" && e.cancellation.IsZero():
		return errors.New("runs: a winning cancellation key requires its confirmed terminal")
	case e.rootCancellation != nil && (!canceledTerminal || !e.Run.Lineage().IsRoot()):
		return errors.New("runs: a root cancellation receipt requires its canceled terminal Run")
	}
	return nil
}

func (e EventCommit) validateChildCancellation() error {
	if e.childCancellation == nil {
		return nil
	}
	return e.childCancellation.validate(e)
}

func (e EventCommit) validateProgress() error {
	if e.Progress == nil {
		return nil
	}
	if e.Progress.ID() != e.RunID || e.Progress.SessionID() != e.SessionID {
		return fmt.Errorf("runs: event commit progress belongs to Run %q in Session %q", e.Progress.ID(), e.Progress.SessionID())
	}
	if !e.Progress.Executes(e.SegmentID) {
		return fmt.Errorf(
			"runs: event commit progress is %s in Segment %q, want running Segment %q",
			e.Progress.State(), e.Progress.ActiveSegmentID(), e.SegmentID,
		)
	}
	return nil
}

func (e EventCommit) validateConversationMessages() error {
	for index, message := range e.ConversationMessages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("runs: event commit conversation message[%d]: %w", index, err)
		}
	}
	return nil
}

func (e EventCommit) validateEnvelope() error {
	if err := e.RunID.Validate(); err != nil {
		return fmt.Errorf("runs: event commit: %w", err)
	}
	if err := e.SessionID.Validate(); err != nil {
		return fmt.Errorf("runs: event commit: %w", err)
	}
	if e.ExecutionContext != nil {
		if e.State != StateUnchanged {
			return errors.New("runs: execution context cannot accompany a lifecycle transition")
		}
		if err := e.ExecutionContext.ValidateOwnership(e.ExecutionContext.RootMemberID, e.SessionID); err != nil {
			return err
		}
	}
	if err := e.SegmentID.Validate(); err != nil {
		return fmt.Errorf("runs: event commit: %w", err)
	}
	if err := validateOptionalMember(e.ObsoleteCheckpointRootID); err != nil {
		return fmt.Errorf("runs: event commit checkpoint root: %w", err)
	}
	return nil
}

func (e EventCommit) validateItems() error {
	seenItems := make(map[identity.ItemID]struct{}, len(e.Items))
	for index, write := range e.Items {
		item := write.Item()
		if item.ID().IsZero() || item.RunID() != e.RunID || item.SessionID() != e.SessionID {
			return fmt.Errorf("runs: event commit Item[%d] is not owned by Run %q", index, e.RunID)
		}
		if _, duplicate := seenItems[item.ID()]; duplicate {
			return fmt.Errorf("runs: event commit repeats Item %q", item.ID())
		}
		seenItems[item.ID()] = struct{}{}
	}
	return nil
}

func (e EventCommit) validateInvocations() error {
	items := make(map[identity.ItemID]transcript.Item, len(e.Items))
	for _, write := range e.Items {
		items[write.Item().ID()] = write.Item()
	}
	if err := e.validateModelInvocations(); err != nil {
		return err
	}
	return e.validateToolInvocations(items)
}

func (e EventCommit) validateModelInvocations() error {
	seenInvocations := make(map[string]struct{}, len(e.ModelInvocations))
	for index, invocation := range e.ModelInvocations {
		if err := invocation.Validate(); err != nil {
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

func (e EventCommit) validateToolInvocations(items map[identity.ItemID]transcript.Item) error {
	seenTools := make(map[string]struct{}, len(e.ToolInvocations))
	seenToolItems := make(map[identity.ItemID]struct{}, len(e.ToolInvocations))
	for index, invocation := range e.ToolInvocations {
		if err := invocation.Validate(); err != nil {
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
			if call, _ := item.ToolCall(); call.Failure == nil {
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

func (e EventCommit) validateSuspend() error {
	if e.Run == nil || e.Run.State() != run.Waiting {
		return errors.New("runs: suspend event commit has no waiting Run")
	}
	if e.ObsoleteCheckpointRootID != "" {
		return errors.New("runs: suspend event commit carries terminal facts")
	}
	return nil
}

func (e EventCommit) validateTerminalize() error {
	if e.CommitID.IsZero() {
		return errors.New("runs: terminal event commit has no commit identity")
	}
	if e.Run == nil || !e.Run.State().IsTerminal() {
		return errors.New("runs: terminal event commit has no matching terminal Run")
	}
	if e.ObsoleteCheckpointRootID != "" && !e.Run.Lineage().IsRoot() {
		return errors.New("runs: only a root terminal retires execution checkpoints")
	}
	return nil
}

func (e EventCommit) validateLifecycle() error {
	switch e.State {
	case StateUnchanged:
		if e.Run != nil || e.ObsoleteCheckpointRootID != "" {
			return errors.New("runs: unchanged event commit carries lifecycle facts")
		}
		return nil
	case StateSuspend:
		if err := e.validateSuspend(); err != nil {
			return err
		}
	case StateTerminalize:
		if err := e.validateTerminalize(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("runs: event commit has unknown state change %q", e.State)
	}

	if e.Run.ID() != e.RunID || e.Run.SessionID() != e.SessionID {
		return errors.New("runs: event commit Run ownership differs from its envelope")
	}
	// A terminalize commit may still carry an unresolved conversation watermark:
	// the reducer cannot know the final count, and the terminal transaction
	// resolves it while committing. Every other terminal fact is the Run's own.
	return nil
}

// ChargesGoal reports whether this commit terminalizes a Goal-owned root Run,
// whose transaction also applies that Run to the Session's Goal.
func (e EventCommit) ChargesGoal() bool {
	return e.State == StateTerminalize && e.Run != nil && e.Run.GoalIncarnationID() != ""
}

func (e EventCommit) isEmpty() bool {
	return e.ExecutionContext == nil && len(e.ToolResults) == 0 && len(e.Items) == 0 &&
		len(e.ConversationMessages) == 0 &&
		len(e.ModelInvocations) == 0 &&
		len(e.ToolInvocations) == 0 && e.ResultPublication == nil &&
		e.Progress == nil &&
		e.Run == nil &&
		e.rootCancellation == nil &&
		e.childCancellation == nil &&
		e.cancellation.IsZero() &&
		e.ObsoleteCheckpointRootID == "" &&
		e.State == StateUnchanged
}

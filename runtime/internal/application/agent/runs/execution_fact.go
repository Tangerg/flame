package runs

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	corechat "github.com/Tangerg/scope/core/chat"
)

// ExecutorMember is the executor-owned identity of the member that produced an
// event. It deliberately carries no Run or Segment identity: mapping executor
// members onto application Runs belongs to the Coordinator.
type ExecutorMember struct {
	MemberID string
	ParentID string
	// SpawnCallID is the opaque provider ToolCall ID, not an executor Effect ID.
	SpawnCallID string
}

// Child reports whether this member was delegated by another member.
func (e ExecutorMember) Child() bool { return e.ParentID != "" }

// Validate rejects malformed or self-referential member identity. An entirely
// empty member is reserved for a root execution that failed before the executor
// created its member.
func (e ExecutorMember) Validate() error {
	if err := runtimeidentity.ValidateOptionalMember(e.MemberID); err != nil {
		return fmt.Errorf("runs: %w", err)
	}
	if err := runtimeidentity.ValidateOptionalMember(e.ParentID); err != nil {
		return fmt.Errorf("runs: executor parent: %w", err)
	}
	if e.MemberID == "" {
		if e.ParentID != "" || e.SpawnCallID != "" {
			return errors.New("runs: empty executor member id cannot carry parent or spawn-call identity")
		}
		return nil
	}
	if e.ParentID == e.MemberID {
		return errors.New("runs: executor member cannot parent itself")
	}
	if e.ParentID == "" && e.SpawnCallID != "" {
		return errors.New("runs: root executor member cannot carry spawn-call identity")
	}
	return nil
}

// ExecutorEvent is the driven-executor port value. Member identifies the
// concrete root/child member; Payload is the closed application-owned signal
// family. A root stream may therefore carry child signals without relabeling
// their producer as the root.
type ExecutorEvent struct {
	Member  ExecutorMember
	Payload ExecutorPayload
}

// Validate checks the envelope before the Coordinator routes it.
func (e ExecutorEvent) Validate() error {
	if e.Payload == nil {
		return errors.New("runs: executor event payload is required")
	}
	return e.Member.Validate()
}

// ExecutorPayload is the closed family carried by the ordered executor stream.
// Most values are reducible [ExecutionFact] facts. A control handshake such as
// control requests share the stream only when their ordering relative to those
// facts is itself part of correctness.
type ExecutorPayload interface {
	executorPayload()
}

type executorPayloadBase struct{}

func (executorPayloadBase) executorPayload() {}

// ExecutionFact is the closed application-owned execution fact family emitted at
// the ExecutionObserver port. Control handshakes deliberately implement only
// [ExecutorPayload], so they cannot accidentally enter the reducer.
type ExecutionFact interface {
	ExecutorPayload
	executionFact()
}

type executionFactBase struct{ executorPayloadBase }

func (executionFactBase) executionFact() {}

type MessageDelta struct {
	executionFactBase
	Text string
}

type ReasoningDelta struct {
	executionFactBase
	Text string
}

// ModelCallStarted is the authoritative pre-provider boundary for one model
// invocation. CallID is an opaque, stable executor identity; the Application
// never parses framework Effect identity from it.
type ModelCallStarted struct {
	executionFactBase
	CallID string
}

// ModelCallCompleted is the sole authoritative semantic and accounting projection
// of one completed model invocation. Message is absent when the provider
// completed without portable content; the strategy owns whether that response
// can complete execution. Message may contain ToolCall parts; the
// reducer projects only assistant content/reasoning here because each actual
// Tool invocation has its own pre-call commit boundary.
type ModelCallCompleted struct {
	executionFactBase
	CallID                   string
	FirstOutputLatencyMillis *int64
	// ReportedUsage is the provider's own per-call report, absent when the
	// provider reported nothing. It keeps the reported shape so an unsupported
	// breakdown stays absent instead of becoming a zero; Tokens and ByModel
	// below are Runtime's cumulative counters for this executing process.
	ReportedUsage *corechat.Usage
	Message       *corechat.Message
	Tokens        accounting.Tokens
	ByModel       []accounting.ModelUsage
	Cost          accounting.Cost
	Steps         int
	ContextTokens int64
}

// ModelObservation retains validated visible output without asserting a complete
// response. It never contains executable calls or canonical continuation state.
type ModelObservation struct {
	Text      string
	Reasoning string
}

// ModelCallFailed closes a provider attempt whose failure is definite. Its
// observation is incomplete transcript content, not assistant output or usage.
// If this fact cannot be durably committed, the dispatcher must instead leave
// the invocation open and reconcile the Effect as unknown.
type ModelCallFailed struct {
	Observation ModelObservation
	executionFactBase
	FirstOutputLatencyMillis *int64
	CallID                   string
}

type ToolCallStarted struct {
	// ArgumentsText preserves rejected input that was never admitted as an argument object.
	ArgumentsText string
	executionFactBase
	CallID            string
	ModelCallSequence uint64
	ToolCallIndex     uint32
	// SourceCallID is the executor's parent-call identity. It exists solely to map
	// a child member causal edge to this
	// canonical Item without parsing CallID or relying on event timing.
	SourceCallID string
	ToolName     string
	Arguments    string
	Activity     string
	SafetyClass  tool.SafetyClass
}

type ToolCallFinished struct {
	CallID    string
	Arguments string
	// ModelResult is the exact provider-neutral value published by Scope.
	// Result remains the independently presented client transcript value.
	ModelResult  *corechat.ToolResult
	Result       *tool.Result
	Offload      *toolresult.Ref
	OutputText   string
	MutatedPaths []string
	// Failure is the one structured failure channel for a completed Tool call.
	// nil means success; its Tool taxonomy prevents Run failures from entering
	// this slot without parallel error strings or boolean classifications.
	Failure *tool.Failure
}

type CompactionBoundary struct {
	executionFactBase
	Summary        string
	MessagesBefore int
	MessagesAfter  int
}

// MemberInterruption is one direct external-input boundary discovered in the
// executor tree. MemberID identifies the member already admitted to an
// application Run; RequestID is private continuation identity; Interrupt is
// the application prompt projected from that request.
type MemberInterruption struct {
	MemberID  string
	RequestID string
	Interrupt Interrupt
}

// TreeInterrupted is the executor's complete, stable view of one human-input
// barrier. It is a control payload rather than an ExecutionFact because no single
// Run reducer may commit it: the Coordinator must suspend the entire active tree
// in one transaction.
type TreeInterrupted struct {
	executorPayloadBase
	checkpoint    ExecutorCheckpoint
	interruptions []MemberInterruption
}

// NewTreeInterrupted captures one validated, ownership-independent executor
// waiting boundary. The Coordinator never interprets the checkpoint; it only
// places its write into the tree-barrier transaction.
func NewTreeInterrupted(
	checkpoint ExecutorCheckpoint,
	interruptions []MemberInterruption,
) (TreeInterrupted, error) {
	barrier := TreeInterrupted{
		checkpoint:    checkpoint.Clone(),
		interruptions: cloneMemberInterruptions(interruptions),
	}
	if err := barrier.validate(); err != nil {
		return TreeInterrupted{}, err
	}
	return barrier, nil
}

// Checkpoint returns an ownership-independent executor snapshot.
func (t TreeInterrupted) Checkpoint() ExecutorCheckpoint {
	return t.checkpoint.Clone()
}

// Interruptions returns ownership-independent external-input bindings.
func (t TreeInterrupted) Interruptions() []MemberInterruption {
	return cloneMemberInterruptions(t.interruptions)
}

func cloneMemberInterruptions(values []MemberInterruption) []MemberInterruption {
	interruptions := make([]MemberInterruption, len(values))
	for index, value := range values {
		value.Interrupt = cloneInterrupt(value.Interrupt)
		interruptions[index] = value
	}
	return interruptions
}

func (t TreeInterrupted) validate() error {
	if err := t.checkpoint.Validate(); err != nil {
		return fmt.Errorf("runs: executor tree interrupt has an invalid checkpoint: %w", err)
	}
	if len(t.interruptions) == 0 {
		return errors.New("runs: executor emitted an empty tree interrupt")
	}
	seen := make(map[inputRequestKey]struct{}, len(t.interruptions))
	for index, request := range t.interruptions {
		if err := runtimeidentity.ValidateMember(request.MemberID); err != nil {
			return fmt.Errorf("runs: tree interrupt request[%d] member: %w", index, err)
		}
		if err := runtimeidentity.ValidateRequest(request.RequestID); err != nil {
			return fmt.Errorf("runs: tree interrupt request[%d]: %w", index, err)
		}
		if err := request.Interrupt.Validate(); err != nil {
			return fmt.Errorf("runs: tree interrupt request[%d]: %w", index, err)
		}
		key := inputRequestIdentity(request.MemberID, request.RequestID)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf(
				"runs: member %q repeated request %q",
				request.MemberID,
				request.RequestID,
			)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (t TreeInterrupted) validateFor(
	rootMemberID string,
	sessionID string,
	goalIncarnationID string,
	selection modelref.Selection,
) error {
	if err := t.validate(); err != nil {
		return err
	}
	if err := t.checkpoint.ValidateOwnership(rootMemberID, sessionID); err != nil {
		return fmt.Errorf("runs: executor tree interrupt checkpoint ownership: %w", err)
	}
	if t.checkpoint.Scope.GoalIncarnationID != goalIncarnationID {
		return fmt.Errorf(
			"runs: executor tree interrupt checkpoint goal incarnation %q does not match Run %q: %w",
			t.checkpoint.Scope.GoalIncarnationID,
			goalIncarnationID,
			ErrInvalidExecutorCheckpoint,
		)
	}
	if !t.checkpoint.ModelSelection.Equal(selection) {
		return fmt.Errorf(
			"runs: executor tree interrupt checkpoint model %q does not match Run %q: %w",
			t.checkpoint.ModelSelection,
			selection,
			ErrInvalidExecutorCheckpoint,
		)
	}
	return nil
}

// SegmentInterrupted is the member-Run reducer input derived from a
// [TreeInterrupted] barrier. Executor sources never emit it directly.
type SegmentInterrupted struct {
	executionFactBase
	Interrupts []Interrupt
	// Duration is how long this segment executed before parking. A parked Run
	// still reports what it consumed, so the executor stamps it here for the same
	// reason it stamps it on SegmentEnded.
	Duration time.Duration
}

func (s SegmentInterrupted) validate() error {
	if len(s.Interrupts) == 0 {
		return errors.New("runs: executor emitted an empty interrupt")
	}
	for _, interrupt := range s.Interrupts {
		if err := interrupt.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type SegmentEnded struct {
	executionFactBase
	Reason            run.Outcome
	unresolvedEffects []run.UnresolvedEffect
	// Failure is present exactly when Reason is Failed, TimedOut, or Lost. It is
	// already a stable, client-safe classification; executor diagnostics
	// never enter the event stream.
	failure *run.Failure
	// Usage is the segment's final accounting, and is absent only when the
	// executor cannot produce an authoritative report. Child executors retain
	// their subtree ledger through cancellation and failure, so those terminals
	// can still carry usage. Absent is NOT zero: reading a missing report as
	// "spent nothing" made a canceled Run's committed metering fall back below
	// what its own progress events had already published.
	usage    *SegmentUsage
	Duration time.Duration
}

// SegmentUsage is one authoritative accounting report for a segment. The three
// numbers are produced together by the executor's observer, so they travel
// together: a report that had tokens but no per-model split would be a different
// report, not this one with a field missing.
type SegmentUsage struct {
	Tokens  accounting.Tokens
	ByModel []accounting.ModelUsage
	Cost    accounting.Cost
	Steps   int
}

// NewSegmentEnded captures an ownership-independent terminal observation.
// Contextual outcome and monotonic-accounting checks remain with the reducer,
// which owns the active Segment state needed to decide them.
func NewSegmentEnded(
	reason run.Outcome,
	failure *run.Failure,
	usage *SegmentUsage,
	duration time.Duration,
) SegmentEnded {
	end := SegmentEnded{Reason: reason, Duration: duration}
	if failure != nil {
		owned := *failure
		end.failure = &owned
	}
	if usage != nil {
		owned := *usage
		owned.ByModel = append([]accounting.ModelUsage(nil), usage.ByModel...)
		end.usage = &owned
	}
	return end
}

// Failure returns an ownership-independent terminal failure, when present.
func (s SegmentEnded) Failure() *run.Failure {
	if s.failure == nil {
		return nil
	}
	failure := *s.failure
	return &failure
}

// Usage returns an ownership-independent final accounting report, when present.
func (s SegmentEnded) Usage() *SegmentUsage {
	if s.usage == nil {
		return nil
	}
	usage := *s.usage
	usage.ByModel = append([]accounting.ModelUsage(nil), s.usage.ByModel...)
	return &usage
}

type UsageReported struct {
	executionFactBase
	Tokens        accounting.Tokens
	ByModel       []accounting.ModelUsage
	Cost          accounting.Cost
	Steps         int
	ContextTokens int64
}

// PlanUpdated reports the committed Plan after a replacement — read back
// from the store, so what is published is what was written rather than what the
// tool was asked to write.
type PlanUpdated struct {
	executionFactBase
	State plan.State
}

// AppliedSteerMessage is one ordered user message first made visible to a model
// call. ItemID was reserved at admission. AlreadyProjected identifies input
// whose transcript Item was committed by a continuation opening.
// Both paths still append the message to Conversation at this model boundary.
type AppliedSteerMessage struct {
	Content          []transcript.ContentBlock
	ItemID           string
	AlreadyProjected bool
}

// SteerMessagesApplied reports one ordered model-boundary batch. The reducer
// commits its remaining transcript and Conversation projections atomically.
type SteerMessagesApplied struct {
	executionFactBase
	Messages []AppliedSteerMessage
}

func (t ToolCallFinished) clone() ToolCallFinished {
	if t.ModelResult != nil {
		t.ModelResult = new(t.ModelResult.Clone())
	}
	if t.Result != nil {
		t.Result = new(*t.Result)
	}
	if t.Offload != nil {
		t.Offload = new(*t.Offload)
	}
	if t.Failure != nil {
		t.Failure = new(*t.Failure)
	}
	t.MutatedPaths = slices.Clone(t.MutatedPaths)
	return t
}

func (s SegmentEnded) WithUnresolvedEffects(effects []run.UnresolvedEffect) SegmentEnded {
	s.unresolvedEffects = slices.Clone(effects)
	return s
}
func (s SegmentEnded) UnresolvedEffects() []run.UnresolvedEffect {
	return slices.Clone(s.unresolvedEffects)
}

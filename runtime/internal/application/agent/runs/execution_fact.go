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
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	corechat "github.com/Tangerg/scope/core/chat"
)

type ExecutorMember struct {
	MemberID    string
	ParentID    string
	SpawnCallID string
}

func (e ExecutorMember) Child() bool { return e.ParentID != "" }

func (e ExecutorMember) Validate() error {
	if err := validateOptionalMember(e.MemberID); err != nil {
		return fmt.Errorf("runs: %w", err)
	}
	if err := validateOptionalMember(e.ParentID); err != nil {
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

type ExecutorEvent struct {
	Member  ExecutorMember
	Payload executorPayload
}

func (e ExecutorEvent) Validate() error {
	if e.Payload == nil {
		return errors.New("runs: executor event payload is required")
	}
	return e.Member.Validate()
}

type executorPayload interface {
	executorPayload()
}

type executorPayloadBase struct{}

func (executorPayloadBase) executorPayload() {}

type ExecutionFact interface {
	executorPayload
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

type ModelCallStarted struct {
	executionFactBase
	Purpose run.ModelCallPurpose
	CallID  string
}

type ModelCallCompleted struct {
	executionFactBase
	CallID                   string
	FirstOutputLatencyMillis *int64
	ReportedUsage            *corechat.Usage
	Message                  *corechat.Message
	Model                    string
	Cost                     accounting.Cost
	ContextTokens            int64
}

type ModelObservation struct {
	Text      string
	Reasoning string
}

// If this fact cannot be durably committed, the dispatcher must leave the
// invocation open and reconcile the Effect as unknown.
type ModelCallFailed struct {
	KnownResponse *corechat.Response
	Accounting    *ModelCallCompleted
	Observation   ModelObservation
	executionFactBase
	FirstOutputLatencyMillis *int64
	CallID                   string
}

func (f ModelCallFailed) validateFor(purpose run.ModelCallPurpose) error {
	if purpose == run.ModelIndependent && f.carriesConversationState() {
		return fmt.Errorf("%w: independent model failure carries conversation state", errExecutorContract)
	}
	if f.KnownResponse != nil {
		if err := f.KnownResponse.Validate(); err != nil {
			return err
		}
		if f.Accounting == nil || !modelref.UsageMatchesResponse(f.Accounting.ReportedUsage, f.KnownResponse) {
			return fmt.Errorf("%w: known model failure response lacks its usage", errExecutorContract)
		}
	}
	if f.Accounting == nil {
		return nil
	}
	if f.Accounting.CallID != f.CallID {
		return fmt.Errorf("%w: model failure accounting names another call", errExecutorContract)
	}
	if f.KnownResponse == nil && f.Accounting.describesResponse() {
		return fmt.Errorf("%w: model failure accounting describes an unknown response", errExecutorContract)
	}
	return nil
}

func (f ModelCallFailed) carriesConversationState() bool {
	if f.Observation != (ModelObservation{}) || f.FirstOutputLatencyMillis != nil {
		return true
	}
	a := f.Accounting
	return a != nil && (a.Message != nil || a.ContextTokens != 0 || a.FirstOutputLatencyMillis != nil)
}

func (c ModelCallCompleted) describesResponse() bool {
	return c.ReportedUsage != nil || c.Cost.OptionalUSD() != nil || c.ContextTokens != 0 || c.Message != nil
}

type ToolCallStarted struct {
	executionFactBase
	CallID            string
	ModelCallSequence uint64
	ToolCallIndex     uint32
	SourceCallID      string
	Source            tool.Ref
	ToolName          string
	Input             transcript.ToolInput
	SafetyClass       tool.SafetyClass
}

func (t ToolCallStarted) MatchesCapturedRef(ref tool.Ref) bool {
	return ref.MatchesPublishedName(t.ToolName) && (t.Source == (tool.Ref{}) || t.Source == ref)
}

type ToolCallFinished struct {
	CallID       string
	ModelResult  *corechat.ToolResult
	Result       *tool.Result
	Offload      *transcript.ToolResultOffload
	OutputText   string
	MutatedPaths []string
	Failure      *tool.Failure
}

type ToolCallPrepared struct {
	executionFactBase
	Start   ToolCallStarted
	Context ExecutorCheckpoint
	Caller  plugin.ToolCaller
}

type CompactionBoundary struct {
	Rewrite *conversation.Compaction
	executionFactBase
	Summary        string
	MessagesBefore int
	MessagesAfter  int
}

type MemberInterruption struct {
	MemberID  string
	RequestID string
	Interrupt Interrupt
}

// A control payload rather than an ExecutionFact: no single Run reducer may commit
// it, because the whole active tree suspends in one transaction.
type TreeInterrupted struct {
	executorPayloadBase
	checkpoint    ExecutorCheckpoint
	interruptions []MemberInterruption
}

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

func (t TreeInterrupted) Checkpoint() ExecutorCheckpoint {
	return t.checkpoint.Clone()
}

func (t TreeInterrupted) Interruptions() []MemberInterruption {
	return cloneMemberInterruptions(t.interruptions)
}

func cloneMemberInterruptions(values []MemberInterruption) []MemberInterruption {
	interruptions := make([]MemberInterruption, len(values))
	for index, value := range values {
		if value.Interrupt != nil {
			value.Interrupt = value.Interrupt.clone()
		}
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
		if err := ValidateMember(request.MemberID); err != nil {
			return fmt.Errorf("runs: tree interrupt request[%d] member: %w", index, err)
		}
		if err := validateRequest(request.RequestID); err != nil {
			return fmt.Errorf("runs: tree interrupt request[%d]: %w", index, err)
		}
		if err := validateInterrupt(request.Interrupt); err != nil {
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

func (t TreeInterrupted) validateFor(rootMemberID string, sessionID identity.SessionID) error {
	if err := t.validate(); err != nil {
		return err
	}
	if err := t.checkpoint.ValidateOwnership(rootMemberID, sessionID); err != nil {
		return fmt.Errorf("runs: executor tree interrupt checkpoint ownership: %w", err)
	}
	return nil
}

type SegmentEnded struct {
	executionFactBase
	Ending            run.Ending
	unresolvedEffects []run.UnresolvedEffect
	cancellation      CancellationID
	Duration          time.Duration
}

func NewSegmentEnded(ending run.Ending, duration time.Duration) SegmentEnded {
	return SegmentEnded{Ending: ending, Duration: duration}
}

func NewCancellationEnded(cancellation CancellationID, duration time.Duration) SegmentEnded {
	return SegmentEnded{Ending: run.Canceled{}, Duration: duration, cancellation: cancellation}
}

type PlanUpdated struct {
	executionFactBase
	State plan.State
}

type AppliedSteerMessage struct {
	Content          []transcript.ContentBlock
	ItemID           identity.ItemID
	AlreadyProjected bool
}

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

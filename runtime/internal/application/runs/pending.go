package runs

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/transcript"
	"github.com/Tangerg/flame/runtime/internal/executoridentity"
)

// Pending is one complete Run-tree barrier awaiting human decisions. The set is
// keyed by RootRunID and consumed all-or-nothing: individual member Runs do not
// own separate resume claims. Interrupts is the published typed set;
// Bindings connects each item to the executor request it answers;
// Continuations is the durable state required to reopen every surviving Run
// with a fresh Segment, including after host restart.
type Pending struct {
	RootRunID  string
	SessionID  string
	ExecutorID string
	// GoalIncarnationID is the root Run's autonomous-goal incarnation. It is an
	// Run continuation fact, not executor payload: a resumed Segment
	// needs it to keep terminal budget accounting attached to the same Goal.
	GoalIncarnationID string
	Interrupts        []transcript.Interrupt
	Bindings          []InterruptBinding
	Continuations     []Continuation
	// Capabilities is the Run's frozen optional behavior. A continuation refuses
	// callers that lack it and reuses its admitted interrupt kinds.
	Capabilities run.Capabilities
	// CreatedAt orders open sets. It is the barrier commit time, not any one
	// input request's creation time.
	CreatedAt time.Time
}

// Continuation is the durable hand-off for one suspended Run. MemberID is the
// opaque binding between that Run and its executor member; the
// executor's parent/spawn topology remains inside its opaque checkpoint. Run
// lineage is the product's independent tree fact.
type Continuation struct {
	RunID          string
	MemberID       string
	Lineage        run.Lineage
	ModelSelection modelref.Selection
	DrainedTools   []DrainedTool
	// CommittedTools are tool results committed to the transcript while the
	// executor tree stayed parked. The
	// executor still publishes those results when it re-enters the checkpoint
	// because the model needs them in its continuation message; the resumed Run
	// reducer consumes this identity set without appending the Item a second time.
	CommittedTools []CommittedTool
	RunCreatedAt   time.Time
	Metrics        run.Metrics
	ContextTokens  int64
	Limits         run.Limits
}

// InterruptBinding is the private correspondence between one published
// interrupt Item and the executor input request that must receive its answer.
type InterruptBinding struct {
	InterruptItemID string
	MemberID        string
	RequestID       string
	// ToolCallID is the provider call identity of an approval boundary. It is
	// intentionally private continuation data: edited arguments may change the
	// invocation replayed after approval, so neither name nor arguments can own
	// the resumed ToolCall identity. Questions leave it empty because their
	// underlying tool is already carried as a drained Tool.
	ToolCallID string
}

type memberRequestIdentity struct {
	memberID  string
	requestID string
}

type memberToolCallIdentity struct {
	memberID   string
	toolCallID string
}

type pendingBindingValidator struct {
	pending          Pending
	interruptsByItem map[string]transcript.Interrupt
	boundItems       map[string]struct{}
	boundRequests    map[memberRequestIdentity]struct{}
	boundToolCalls   map[memberToolCallIdentity]struct{}
}

// InterruptAnswer is one validated decision bound to the exact executor
// boundary that must consume it. InterruptItemID keeps the transcript item
// identity attached until the execution-control boundary; MemberID and
// RequestID prevents execution from guessing which parked branch it answers.
type InterruptAnswer struct {
	InterruptItemID string
	MemberID        string
	RequestID       string
	Resolution      interrupt.Resolution
}

// DrainedTool records one tool item that was still open when its Run suspended.
// The continuation re-binds the re-fired tool to this original item identity
// instead of minting a duplicate.
type DrainedTool struct {
	ItemID         string
	ItemOccurredAt time.Time
	CallID         string
	// SourceCallID is the provider ToolCall identity used by model-context results.
	SourceCallID string
	Name         string
	// Arguments is the canonical JSON used for resume correlation.
	Arguments string
}

// CommittedTool is the durable hand-off for one tool result already written to
// the transcript while its executor checkpoint was parked. Failure records the
// classification that was committed; it is not reconstructed from
// the executor's lower-level error when the checkpoint later publishes its
// model-facing result.
type CommittedTool struct {
	ItemID       string
	CallID       string
	SourceCallID string
	Name         string
	// Arguments is the canonical JSON used to reject a mismatched replay.
	Arguments string
	Failure   tool.Failure
}

// RootContinuation returns the root Run's hand-off. A valid Pending always has
// exactly one.
func (p Pending) RootContinuation() (Continuation, bool) {
	for _, continuation := range p.Continuations {
		if continuation.RunID == p.RootRunID {
			return continuation, true
		}
	}
	return Continuation{}, false
}

// Validate checks the complete tree hand-off. It deliberately validates both
// directions of the item/input-request relation so accepting a response never
// requires guessing which executor boundary it belongs to.
func (p Pending) Validate() error {
	if err := p.validateEnvelope(); err != nil {
		return err
	}
	if err := p.Capabilities.Validate(); err != nil {
		return fmt.Errorf("interrupts: pending capabilities: %w", err)
	}
	runIDs, err := p.validateContinuations()
	if err != nil {
		return err
	}
	interruptsByItem, err := p.validateInterrupts(runIDs)
	if err != nil {
		return err
	}
	return p.validateBindings(interruptsByItem)
}

func (p Pending) validateEnvelope() error {
	if _, err := resourceid.ParseRun(p.RootRunID); err != nil {
		return fmt.Errorf("interrupts: pending root: %w", err)
	}
	if _, err := resourceid.ParseSession(p.SessionID); err != nil {
		return fmt.Errorf("interrupts: pending: %w", err)
	}
	if _, _, err := goalref.ParseOptionalIncarnation(p.GoalIncarnationID); err != nil {
		return fmt.Errorf("interrupts: pending: %w", err)
	}
	if _, err := executoridentity.ParseExecutor(p.ExecutorID); err != nil {
		return fmt.Errorf("interrupts: pending: %w", err)
	}
	switch {
	case p.CreatedAt.IsZero():
		return errors.New("interrupts: pending creation time is required")
	case len(p.Interrupts) == 0:
		return errors.New("interrupts: pending set has no interrupts")
	case len(p.Continuations) == 0:
		return errors.New("interrupts: pending set has no continuations")
	case len(p.Bindings) != len(p.Interrupts):
		return fmt.Errorf(
			"interrupts: %d input-request bindings do not match %d interrupts",
			len(p.Bindings),
			len(p.Interrupts),
		)
	}
	return nil
}

func (p Pending) validateContinuations() (map[string]struct{}, error) {
	runIDs := make(map[string]struct{}, len(p.Continuations))
	memberIDs := make(map[string]struct{}, len(p.Continuations))
	treeMembers := make([]run.TreeMember, 0, len(p.Continuations))
	rootCount := 0
	for index, continuation := range p.Continuations {
		if err := continuation.Validate(); err != nil {
			return nil, fmt.Errorf("interrupts: continuation[%d]: %w", index, err)
		}
		if _, duplicate := runIDs[continuation.RunID]; duplicate {
			return nil, fmt.Errorf("interrupts: duplicate continuation run %q", continuation.RunID)
		}
		runIDs[continuation.RunID] = struct{}{}
		treeMembers = append(treeMembers, run.TreeMember{
			RunID:   continuation.RunID,
			Lineage: continuation.Lineage,
		})
		if _, duplicate := memberIDs[continuation.MemberID]; duplicate {
			return nil, fmt.Errorf("interrupts: duplicate continuation member %q", continuation.MemberID)
		}
		memberIDs[continuation.MemberID] = struct{}{}
		if continuation.RunID == p.RootRunID {
			rootCount++
			if !continuation.Lineage.IsRoot() {
				return nil, errors.New("interrupts: root continuation carries child lineage")
			}
		} else if continuation.Lineage.RootRunID != p.RootRunID {
			return nil, fmt.Errorf(
				"interrupts: child continuation %q names root run %q, want %q",
				continuation.RunID,
				continuation.Lineage.RootRunID,
				p.RootRunID,
			)
		}
	}
	if rootCount != 1 {
		return nil, fmt.Errorf("interrupts: pending set has %d root continuations", rootCount)
	}
	tree, err := run.NewTree(p.RootRunID, treeMembers)
	if err != nil {
		return nil, fmt.Errorf("interrupts: continuation tree: %w", err)
	}
	if len(p.Continuations) > 1 && !p.Capabilities.ChildRuns {
		return nil, errors.New("interrupts: pending tree has child Runs but its capabilities forbid them")
	}
	canonicalRunIDs := tree.Postorder()
	for index, continuation := range p.Continuations {
		if continuation.RunID != canonicalRunIDs[index] {
			return nil, fmt.Errorf(
				"interrupts: continuation[%d] is run %q, canonical postorder requires %q",
				index,
				continuation.RunID,
				canonicalRunIDs[index],
			)
		}
	}
	return runIDs, nil
}

func (p Pending) validateInterrupts(runIDs map[string]struct{}) (map[string]transcript.Interrupt, error) {
	interruptsByItem := make(map[string]transcript.Interrupt, len(p.Interrupts))
	for index, interrupt := range p.Interrupts {
		if err := validateInterrupt(interrupt); err != nil {
			return nil, fmt.Errorf("interrupts: interrupt[%d]: %w", index, err)
		}
		if _, exists := runIDs[interrupt.RunID]; !exists {
			return nil, fmt.Errorf("interrupts: interrupt item %q names unknown run %q", interrupt.ItemID, interrupt.RunID)
		}
		if !slices.Contains(p.Capabilities.InterruptKinds, interrupt.Kind) {
			return nil, fmt.Errorf(
				"interrupts: interrupt item %q has kind %s outside the frozen capabilities",
				interrupt.ItemID,
				interrupt.Kind,
			)
		}
		if _, duplicate := interruptsByItem[interrupt.ItemID]; duplicate {
			return nil, fmt.Errorf("interrupts: duplicate interrupt item %q", interrupt.ItemID)
		}
		interruptsByItem[interrupt.ItemID] = interrupt
	}
	return interruptsByItem, nil
}

func (p Pending) validateBindings(interruptsByItem map[string]transcript.Interrupt) error {
	validator := pendingBindingValidator{
		pending:          p,
		interruptsByItem: interruptsByItem,
		boundItems:       make(map[string]struct{}, len(p.Bindings)),
		boundRequests:    make(map[memberRequestIdentity]struct{}, len(p.Bindings)),
		boundToolCalls:   make(map[memberToolCallIdentity]struct{}, len(p.Bindings)),
	}
	for index, binding := range p.Bindings {
		if err := validator.validate(index, binding); err != nil {
			return err
		}
	}
	return nil
}

func (v *pendingBindingValidator) validate(index int, binding InterruptBinding) error {
	if err := binding.validateIdentities(); err != nil {
		return fmt.Errorf("interrupts: input-request binding[%d]: %w", index, err)
	}
	request, err := v.interrupt(index, binding)
	if err != nil {
		return err
	}
	if err := v.validateInterruptTool(index, binding, request); err != nil {
		return err
	}
	continuation, err := v.continuation(index, binding, request)
	if err != nil {
		return err
	}
	if request.Kind == interrupt.Approval {
		if err := validateApprovalToolState(binding, continuation); err != nil {
			return err
		}
	}
	return v.record(binding)
}

func (b InterruptBinding) validateIdentities() error {
	if _, err := resourceid.ParseItem(b.InterruptItemID); err != nil {
		return fmt.Errorf("interrupt item: %w", err)
	}
	if _, err := executoridentity.ParseMember(b.MemberID); err != nil {
		return err
	}
	if _, err := executoridentity.ParseRequest(b.RequestID); err != nil {
		return err
	}
	return nil
}

func (v *pendingBindingValidator) interrupt(
	index int,
	binding InterruptBinding,
) (transcript.Interrupt, error) {
	request, exists := v.interruptsByItem[binding.InterruptItemID]
	if !exists {
		return transcript.Interrupt{}, fmt.Errorf(
			"interrupts: input-request binding[%d] names unknown item %q",
			index,
			binding.InterruptItemID,
		)
	}
	if v.pending.Interrupts[index].ItemID != binding.InterruptItemID {
		return transcript.Interrupt{}, fmt.Errorf(
			"interrupts: input-request binding[%d] names item %q, canonical interrupt order requires %q",
			index,
			binding.InterruptItemID,
			v.pending.Interrupts[index].ItemID,
		)
	}
	return request, nil
}

func (v *pendingBindingValidator) validateInterruptTool(
	index int,
	binding InterruptBinding,
	request transcript.Interrupt,
) error {
	switch request.Kind {
	case interrupt.Approval:
		if _, err := executoridentity.ParseEffect(binding.ToolCallID); err != nil {
			return fmt.Errorf("interrupts: input-request binding[%d]: %w", index, err)
		}
		key := memberToolCallIdentity{memberID: binding.MemberID, toolCallID: binding.ToolCallID}
		if _, duplicate := v.boundToolCalls[key]; duplicate {
			return fmt.Errorf(
				"interrupts: member %q Tool call %q is bound more than once",
				binding.MemberID,
				binding.ToolCallID,
			)
		}
		v.boundToolCalls[key] = struct{}{}
	case interrupt.Question:
		if binding.ToolCallID != "" {
			return fmt.Errorf(
				"interrupts: question item %q carries approval Tool call %q",
				binding.InterruptItemID,
				binding.ToolCallID,
			)
		}
	}
	return nil
}

func (v *pendingBindingValidator) continuation(
	index int,
	binding InterruptBinding,
	request transcript.Interrupt,
) (Continuation, error) {
	continuation, exists := continuationForMember(v.pending.Continuations, binding.MemberID)
	if !exists {
		return Continuation{}, fmt.Errorf(
			"interrupts: input-request binding[%d] names unknown member %q",
			index,
			binding.MemberID,
		)
	}
	if continuation.RunID != request.RunID {
		return Continuation{}, fmt.Errorf(
			"interrupts: item %q belongs to run %q but its input request belongs to run %q",
			request.ItemID,
			request.RunID,
			continuation.RunID,
		)
	}
	return continuation, nil
}

func validateApprovalToolState(binding InterruptBinding, continuation Continuation) error {
	for _, tool := range continuation.DrainedTools {
		if tool.CallID == binding.ToolCallID {
			return fmt.Errorf(
				"interrupts: approval Tool call %q is also drained for member %q",
				binding.ToolCallID,
				binding.MemberID,
			)
		}
	}
	for _, tool := range continuation.CommittedTools {
		if tool.CallID == binding.ToolCallID {
			return fmt.Errorf(
				"interrupts: approval Tool call %q is already committed for member %q",
				binding.ToolCallID,
				binding.MemberID,
			)
		}
	}
	return nil
}

func (v *pendingBindingValidator) record(binding InterruptBinding) error {
	if _, duplicate := v.boundItems[binding.InterruptItemID]; duplicate {
		return fmt.Errorf("interrupts: item %q is bound more than once", binding.InterruptItemID)
	}
	v.boundItems[binding.InterruptItemID] = struct{}{}
	key := memberRequestIdentity{memberID: binding.MemberID, requestID: binding.RequestID}
	if _, duplicate := v.boundRequests[key]; duplicate {
		return fmt.Errorf(
			"interrupts: member %q input request %q is bound more than once",
			binding.MemberID,
			binding.RequestID,
		)
	}
	v.boundRequests[key] = struct{}{}
	return nil
}

// Validate checks one Run-to-member continuation and all of its transcript
// hand-off identities independently of the root-owned Pending aggregate.
func (c Continuation) Validate() error {
	if err := c.validateRun(); err != nil {
		return err
	}
	openItems, openCalls, err := validateDrainedTools(c.DrainedTools)
	if err != nil {
		return err
	}
	return validateCommittedTools(c.CommittedTools, openItems, openCalls)
}

func (c Continuation) validateRun() error {
	if _, err := resourceid.ParseRun(c.RunID); err != nil {
		return err
	}
	if _, err := executoridentity.ParseMember(c.MemberID); err != nil {
		return err
	}
	if c.RunCreatedAt.IsZero() {
		return errors.New("run creation time is required")
	}
	if err := c.Lineage.Validate(c.RunID); err != nil {
		return fmt.Errorf("lineage: %w", err)
	}
	if err := c.ModelSelection.Validate(); err != nil {
		return fmt.Errorf("model selection: %w", err)
	}
	if err := c.Metrics.Validate(); err != nil {
		return fmt.Errorf("metrics: %w", err)
	}
	if c.ContextTokens < 0 {
		return errors.New("context tokens must not be negative")
	}
	if err := c.Limits.Validate(); err != nil {
		return fmt.Errorf("limits: %w", err)
	}
	return nil
}

func validateDrainedTools(tools []DrainedTool) (map[string]struct{}, map[string]struct{}, error) {
	items := make(map[string]struct{}, len(tools))
	calls := make(map[string]struct{}, len(tools))
	for index, tool := range tools {
		if err := tool.validate(); err != nil {
			return nil, nil, fmt.Errorf("drained tool[%d]: %w", index, err)
		}
		if _, duplicate := items[tool.ItemID]; duplicate {
			return nil, nil, fmt.Errorf("drained tool item %q is duplicated", tool.ItemID)
		}
		if _, duplicate := calls[tool.CallID]; duplicate {
			return nil, nil, fmt.Errorf("drained tool call %q is duplicated", tool.CallID)
		}
		items[tool.ItemID] = struct{}{}
		calls[tool.CallID] = struct{}{}
	}
	return items, calls, nil
}

func (d DrainedTool) validate() error {
	if err := validateToolIdentity(d.ItemID, d.CallID, d.Name, d.Arguments); err != nil {
		return err
	}
	if _, _, err := conversation.ParseOptionalToolCallIdentity(d.SourceCallID); err != nil {
		return err
	}
	if d.ItemOccurredAt.IsZero() {
		return errors.New("item occurrence time is required")
	}
	return nil
}

func validateCommittedTools(
	tools []CommittedTool,
	openItems map[string]struct{},
	openCalls map[string]struct{},
) error {
	items := make(map[string]struct{}, len(tools))
	calls := make(map[string]struct{}, len(tools))
	for index, tool := range tools {
		if err := tool.validate(); err != nil {
			return fmt.Errorf("committed tool[%d]: %w", index, err)
		}
		if err := tool.Failure.Validate(); err != nil {
			return fmt.Errorf("committed tool[%d] failure: %w", index, err)
		}
		if _, duplicate := items[tool.ItemID]; duplicate {
			return fmt.Errorf("committed tool item %q is duplicated", tool.ItemID)
		}
		if _, duplicate := calls[tool.CallID]; duplicate {
			return fmt.Errorf("committed tool call %q is duplicated", tool.CallID)
		}
		if _, open := openItems[tool.ItemID]; open {
			return fmt.Errorf("tool item %q is both drained and committed", tool.ItemID)
		}
		if _, open := openCalls[tool.CallID]; open {
			return fmt.Errorf("tool call %q is both drained and committed", tool.CallID)
		}
		items[tool.ItemID] = struct{}{}
		calls[tool.CallID] = struct{}{}
	}
	return nil
}

func (c CommittedTool) validate() error {
	if err := validateToolIdentity(c.ItemID, c.CallID, c.Name, c.Arguments); err != nil {
		return err
	}
	if _, _, err := conversation.ParseOptionalToolCallIdentity(c.SourceCallID); err != nil {
		return err
	}
	return nil
}

func validateToolIdentity(itemID, callID, name, arguments string) error {
	if _, err := resourceid.ParseItem(itemID); err != nil {
		return err
	}
	if _, err := executoridentity.ParseEffect(callID); err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" || name != strings.TrimSpace(name) {
		return errors.New("name is required without surrounding whitespace")
	}
	if strings.TrimSpace(arguments) == "" {
		return errors.New("arguments are required")
	}
	return nil
}

func validateInterrupt(request transcript.Interrupt) error {
	if _, err := resourceid.ParseItem(request.ItemID); err != nil {
		return fmt.Errorf("pending interrupt: %w", err)
	}
	if request.ItemOccurredAt.IsZero() {
		return errors.New("item occurrence time is required")
	}
	if _, err := resourceid.ParseRun(request.RunID); err != nil {
		return fmt.Errorf("pending interrupt: %w", err)
	}
	switch request.Kind {
	case interrupt.Approval:
		if request.Approval == nil || request.Question != nil {
			return errors.New("approval interrupt requires only an approval payload")
		}
		if err := request.Approval.Validate(); err != nil {
			return err
		}
	case interrupt.Question:
		if request.Question == nil || request.Approval != nil {
			return errors.New("question interrupt requires only a question payload")
		}
		if err := request.Question.Validate(); err != nil {
			return err
		}
		if request.Question.Answered() {
			return errors.New("open question interrupt already carries an accepted answer")
		}
	default:
		return fmt.Errorf("unknown interrupt kind %q", request.Kind)
	}
	return nil
}

func continuationForMember(continuations []Continuation, memberID string) (Continuation, bool) {
	for _, continuation := range continuations {
		if continuation.MemberID == memberID {
			return continuation, true
		}
	}
	return Continuation{}, false
}

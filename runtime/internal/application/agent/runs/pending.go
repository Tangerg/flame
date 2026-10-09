package runs

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/instant"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/optional"
)

// Pending is one complete Run-tree barrier awaiting human decisions. The set is
// keyed by RootRunID and consumed all-or-nothing: individual member Runs do not
// own separate resume claims. Interrupts names the Items awaiting answers;
// Bindings connects each Item to the executor request it answers;
// Continuations is the durable state required to reopen every surviving Run
// with a fresh Segment, including after host restart. The parked Runs own
// their Session, lineage, capabilities, and Goal incarnation, and the Items own
// what was asked; a Pending names them and never restates those facts.
type Pending struct {
	RootRunID     string
	ExecutorID    string
	Interrupts    []OpenInterrupt
	Bindings      []InterruptBinding
	Continuations []Continuation
	// CreatedAt orders open sets. It is the barrier commit time, not any one
	// input request's creation time.
	CreatedAt time.Time
}

// Continuation is the durable hand-off for one suspended Run. MemberID is the
// opaque binding between that Run and its executor member; the
// executor's parent/spawn topology remains inside its opaque checkpoint. The
// Run's own lineage is the product's tree fact.
type Continuation struct {
	RunID        string
	MemberID     string
	DrainedTools []DrainedTool
}

// InterruptBinding is the private correspondence between one published
// interrupt Item and the executor input request that must receive its answer.
type InterruptBinding struct {
	InterruptItemID string
	MemberID        string
	RequestID       string
	// ToolCallID is the stable executor call identity of an approval boundary. It is
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
	interruptsByItem map[string]OpenInterrupt
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

// OpenInterrupt is one Item a waiting tree awaits an answer for. The Item owns
// the Run that raised it, its occurrence, and the Tool invocation or Question
// asked; the hand-off owns only that the Item is open and, for an approval,
// the policy's review.
type OpenInterrupt struct {
	ItemID string
	// Approval is the policy's review when the Item is a ToolCall awaiting a
	// verdict; nil when it is a Question Item awaiting answers.
	Approval *ApprovalReview
}

// ApprovalReview is the approval policy's explanation of one held Tool call.
type ApprovalReview struct {
	Risk         tool.RiskLevel
	Reason       string
	Rememberable bool
}

// Kind is the answer the open Item awaits.
func (o OpenInterrupt) Kind() interrupt.Kind {
	if o.Approval != nil {
		return interrupt.Approval
	}
	return interrupt.Question
}

// DrainedTool binds one Tool Item that was still open when its Run suspended
// to the executor call that will re-fire it, so the continuation reuses the
// original Item instead of minting a duplicate. The Item owns its occurrence
// and invocation.
type DrainedTool struct {
	ItemID string
	CallID string
	// SourceCallID is the provider ToolCall identity used by model-context results.
	SourceCallID string
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

// Clone returns an ownership-isolated copy of the complete waiting hand-off.
func (p Pending) Clone() Pending {
	p.Interrupts = slices.Clone(p.Interrupts)
	for index := range p.Interrupts {
		p.Interrupts[index].Approval = optional.Clone(p.Interrupts[index].Approval)
	}
	p.Bindings = slices.Clone(p.Bindings)
	p.Continuations = slices.Clone(p.Continuations)
	for index := range p.Continuations {
		continuation := &p.Continuations[index]
		continuation.DrainedTools = slices.Clone(continuation.DrainedTools)
	}
	return p
}

// Validate checks the complete tree hand-off. It deliberately validates both
// directions of the item/input-request relation so accepting a response never
// requires guessing which executor boundary it belongs to.
func (p Pending) Validate() error {
	if err := p.validateEnvelope(); err != nil {
		return err
	}
	if err := p.validateContinuations(); err != nil {
		return err
	}
	interruptsByItem, err := p.validateInterrupts()
	if err != nil {
		return err
	}
	return p.validateBindings(interruptsByItem)
}

// ValidateForRoot verifies the complete hand-off and its exact requested root
// Run identity. Point reads use it before stored continuation state can
// influence a use case.
func (p Pending) ValidateForRoot(expectedRootRunID string) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return p.requireRoot(expectedRootRunID)
}

func (p Pending) requireRoot(expectedRootRunID string) error {
	if p.RootRunID != expectedRootRunID {
		return fmt.Errorf(
			"interrupts: pending root %q does not match requested identity %q",
			p.RootRunID,
			expectedRootRunID,
		)
	}
	return nil
}

func (p Pending) validateEnvelope() error {
	if err := instant.Validate(p.CreatedAt); err != nil {
		return fmt.Errorf("interrupts: pending creation time: %w", err)
	}
	if err := resourceid.ValidateRun(p.RootRunID); err != nil {
		return fmt.Errorf("interrupts: pending root: %w", err)
	}
	if err := runtimeidentity.ValidateExecutor(p.ExecutorID); err != nil {
		return fmt.Errorf("interrupts: pending: %w", err)
	}
	switch {
	case p.CreatedAt.IsZero() || p.CreatedAt.Location() != time.UTC:
		return errors.New("interrupts: pending creation time is required in UTC")
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

func (p Pending) validateContinuations() error {
	runIDs := make(map[string]struct{}, len(p.Continuations))
	memberIDs := make(map[string]struct{}, len(p.Continuations))
	rootCount := 0
	for index, continuation := range p.Continuations {
		if err := continuation.Validate(); err != nil {
			return fmt.Errorf("interrupts: continuation[%d]: %w", index, err)
		}
		if _, duplicate := runIDs[continuation.RunID]; duplicate {
			return fmt.Errorf("interrupts: duplicate continuation run %q", continuation.RunID)
		}
		runIDs[continuation.RunID] = struct{}{}
		if _, duplicate := memberIDs[continuation.MemberID]; duplicate {
			return fmt.Errorf("interrupts: duplicate continuation member %q", continuation.MemberID)
		}
		memberIDs[continuation.MemberID] = struct{}{}
		if continuation.RunID == p.RootRunID {
			rootCount++
		}
	}
	if rootCount != 1 {
		return fmt.Errorf("interrupts: pending set has %d root continuations", rootCount)
	}
	return nil
}

func (p Pending) validateInterrupts() (map[string]OpenInterrupt, error) {
	interruptsByItem := make(map[string]OpenInterrupt, len(p.Interrupts))
	for index, open := range p.Interrupts {
		if err := open.validate(); err != nil {
			return nil, fmt.Errorf("interrupts: interrupt[%d]: %w", index, err)
		}
		if _, duplicate := interruptsByItem[open.ItemID]; duplicate {
			return nil, fmt.Errorf("interrupts: duplicate interrupt item %q", open.ItemID)
		}
		interruptsByItem[open.ItemID] = open
	}
	return interruptsByItem, nil
}

func (o OpenInterrupt) validate() error {
	if err := resourceid.ValidateItem(o.ItemID); err != nil {
		return fmt.Errorf("pending interrupt: %w", err)
	}
	if o.Approval != nil && !o.Approval.Risk.Valid() {
		return fmt.Errorf("approval has unknown risk %q", o.Approval.Risk)
	}
	return nil
}

func (p Pending) validateBindings(interruptsByItem map[string]OpenInterrupt) error {
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
	continuation, err := v.continuation(index, binding)
	if err != nil {
		return err
	}
	if request.Kind() == interrupt.Approval {
		if err := validateApprovalToolState(binding, continuation); err != nil {
			return err
		}
	}
	return v.record(binding)
}

func (b InterruptBinding) validateIdentities() error {
	if err := resourceid.ValidateItem(b.InterruptItemID); err != nil {
		return fmt.Errorf("interrupt item: %w", err)
	}
	if err := runtimeidentity.ValidateMember(b.MemberID); err != nil {
		return err
	}
	if err := runtimeidentity.ValidateRequest(b.RequestID); err != nil {
		return err
	}
	return nil
}

func (v *pendingBindingValidator) interrupt(
	index int,
	binding InterruptBinding,
) (OpenInterrupt, error) {
	request, exists := v.interruptsByItem[binding.InterruptItemID]
	if !exists {
		return OpenInterrupt{}, fmt.Errorf(
			"interrupts: input-request binding[%d] names unknown item %q",
			index,
			binding.InterruptItemID,
		)
	}
	if v.pending.Interrupts[index].ItemID != binding.InterruptItemID {
		return OpenInterrupt{}, fmt.Errorf(
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
	request OpenInterrupt,
) error {
	switch request.Kind() {
	case interrupt.Approval:
		if err := runtimeidentity.ValidateEffect(binding.ToolCallID); err != nil {
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
) (Continuation, error) {
	continuation, exists := continuationForMember(v.pending.Continuations, binding.MemberID)
	if !exists {
		return Continuation{}, fmt.Errorf(
			"interrupts: input-request binding[%d] names unknown member %q",
			index,
			binding.MemberID,
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
	return validateDrainedTools(c.DrainedTools)
}

func (c Continuation) validateRun() error {
	if err := resourceid.ValidateRun(c.RunID); err != nil {
		return err
	}
	return runtimeidentity.ValidateMember(c.MemberID)
}

func validateDrainedTools(tools []DrainedTool) error {
	items := make(map[string]struct{}, len(tools))
	calls := make(map[string]struct{}, len(tools))
	for index, tool := range tools {
		if err := tool.validate(); err != nil {
			return fmt.Errorf("drained tool[%d]: %w", index, err)
		}
		if _, duplicate := items[tool.ItemID]; duplicate {
			return fmt.Errorf("drained tool item %q is duplicated", tool.ItemID)
		}
		if _, duplicate := calls[tool.CallID]; duplicate {
			return fmt.Errorf("drained tool call %q is duplicated", tool.CallID)
		}
		items[tool.ItemID] = struct{}{}
		calls[tool.CallID] = struct{}{}
	}
	return nil
}

func (d DrainedTool) validate() error {
	if err := resourceid.ValidateItem(d.ItemID); err != nil {
		return err
	}
	return runtimeidentity.ValidateEffect(d.CallID)
}

func continuationForMember(continuations []Continuation, memberID string) (Continuation, bool) {
	for _, continuation := range continuations {
		if continuation.MemberID == memberID {
			return continuation, true
		}
	}
	return Continuation{}, false
}

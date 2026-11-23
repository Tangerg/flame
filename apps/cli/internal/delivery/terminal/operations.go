package terminal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/Tangerg/oolong/ui/core/program"
)

type operationSlot struct {
	name       string
	subject    string
	contention slotContention
}

type slotContention uint8

const (
	keepRunning slotContention = iota
	replaceRunning
)

func exclusiveSlot(name string) operationSlot {
	return operationSlot{name: name, contention: keepRunning}
}

// reported names an exclusive slot the person can start from several entry
// points, so every refusal tells them the same thing.
func (s operationSlot) reported(subject string) operationSlot {
	s.subject = subject
	return s
}

func (s operationSlot) busy() error {
	return fmt.Errorf("wait for the current %s to finish", s.subject)
}

func latestSlot(name string) operationSlot {
	return operationSlot{name: name, contention: replaceRunning}
}

func (s operationSlot) superseding() operationSlot {
	s.contention = replaceRunning
	return s
}

var (
	streamOperation               = exclusiveSlot("stream")
	inputPreparationOperation     = exclusiveSlot("input-preparation").reported("input preparation")
	pendingRunRecoveryOperation   = exclusiveSlot("pending-run-recovery")
	pendingRunSettlementOperation = exclusiveSlot("pending-run-settlement")
	resumeSettlementOperation     = exclusiveSlot("resume-settlement")
	resumeRecoveryOperation       = exclusiveSlot("resume-recovery")
	ownershipSettlementOperation  = exclusiveSlot("ownership-settlement")
	completionOperation           = latestSlot("completion")
	searchOperation               = latestSlot("search")
	readerSearchOperation         = latestSlot("reader-search")
	readerDocumentOperation       = latestSlot("reader-document")
	pickerCatalogOperation        = latestSlot("picker-catalog")
	approvalModeOperation         = latestSlot("approval-mode")
	sessionCenterOperation        = exclusiveSlot("session-center").reported("session action")
	sessionChangeOperation        = exclusiveSlot("session-change").reported("session change")
	sessionOutputOperation        = exclusiveSlot("session-output").reported("session output operation")
	approvalRuleOperation         = exclusiveSlot("approval-rule").reported("approval operation")
	cancelRunOperation            = latestSlot("cancel-run")
	steerRunOperation             = exclusiveSlot("steer-run").reported("steer operation")
	steerReceiptReadOperation     = exclusiveSlot("steer-receipt-read")
	workspaceQueryOperation       = latestSlot("workspace-query")
	runtimeChangesOperation       = latestSlot("runtime-changes")
	sessionInvalidationOperation  = exclusiveSlot("session-invalidation")
	modelConfigOperation          = exclusiveSlot("model-config").reported("model configuration operation")
	goalOperation                 = exclusiveSlot("goal").reported("goal operation")
	skillOperation                = exclusiveSlot("skill").reported("skill operation")
	mcpOperation                  = exclusiveSlot("mcp").reported("MCP operation")
	mcpAuthorizationOperation     = exclusiveSlot("mcp-authorization")
	pluginViewsOperation          = latestSlot("plugin-views")
	pluginViewOperation           = exclusiveSlot("plugin-view")
	diagnosticToolOperation       = exclusiveSlot("diagnostic-tool").reported("diagnostic tool operation")
	hookOperation                 = exclusiveSlot("hook").reported("hook operation")
	feedbackOperation             = exclusiveSlot("feedback").reported("feedback operation")
)

type operationLease struct {
	slot operationSlot
	id   operationLeaseID
}

type operationLeaseID uint64

func (id operationLeaseID) successor() (operationLeaseID, bool) {
	if id == operationLeaseID(math.MaxUint64) {
		return 0, false
	}
	return id + 1, true
}

type operationScope uint8

const (
	applicationOperationScope operationScope = iota
	sessionOperationScope
)

type runAdmissionPolicy uint8

const (
	runAdmissionUnrestricted runAdmissionPolicy = iota
	runAdmissionFence
	runAdmissionAfterSettlement
)

type operationPolicy struct {
	scope        operationScope
	runAdmission runAdmissionPolicy
}

type ownedOperation struct {
	id     operationLeaseID
	policy operationPolicy
	cancel context.CancelFunc
}

type operationOwner struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	next      operationLeaseID
	active    map[string]ownedOperation
	closed    bool
	failure   error
	onFailure func()
	wg        sync.WaitGroup
}

func post(ctx context.Context, dispatcher program.Dispatcher, fn func()) error {
	return dispatcher.Invoke(ctx, func() error {
		fn()
		return nil
	})
}

func newOperationOwner(parent context.Context) *operationOwner {
	ctx, cancel := context.WithCancel(parent)
	return &operationOwner{ctx: ctx, cancel: cancel, active: make(map[string]ownedOperation)}
}

func (o *operationOwner) Go(slot operationSlot, work func(context.Context, operationLease)) bool {
	return o.goWithPolicy(operationPolicy{scope: applicationOperationScope}, slot, work)
}

// A projection replacement cancels all work in this scope before the new session
// becomes visible, so a late result cannot bleed into it.
func (o *operationOwner) GoSession(slot operationSlot, work func(context.Context, operationLease)) bool {
	return o.goWithPolicy(operationPolicy{scope: sessionOperationScope}, slot, work)
}

func (o *operationOwner) GoSessionSettlement(
	slot operationSlot,
	work func(context.Context, operationLease),
) bool {
	return o.goWithPolicy(operationPolicy{
		scope: sessionOperationScope, runAdmission: runAdmissionAfterSettlement,
	}, slot, work)
}

func (o *operationOwner) goWithPolicy(
	policy operationPolicy,
	slot operationSlot,
	work func(context.Context, operationLease),
) bool {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return false
	}
	next, available := o.next.successor()
	if !available {
		o.mu.Unlock()
		return false
	}
	if previous, exists := o.active[slot.name]; exists {
		if slot.contention != replaceRunning {
			o.mu.Unlock()
			return false
		}
		previous.cancel()
	}
	ctx, cancel := context.WithCancel(o.ctx)
	o.next = next
	lease := operationLease{slot: slot, id: next}
	o.active[slot.name] = ownedOperation{id: lease.id, policy: policy, cancel: cancel}
	o.wg.Go(func() {
		defer o.release(lease, cancel)
		work(ctx, lease)
	})
	o.mu.Unlock()
	return true
}

func (o *operationOwner) Current(lease operationLease) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	active, ok := o.active[lease.slot.name]
	return !o.closed && ok && active.id == lease.id
}

func (o *operationOwner) Active(slot operationSlot) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.active[slot.name]
	return !o.closed && ok
}

func (o *operationOwner) BlocksRunAdmission() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return false
	}
	for _, operation := range o.active {
		if operation.policy.runAdmission != runAdmissionUnrestricted {
			return true
		}
	}
	return false
}

func (o *operationOwner) Cancel(slot operationSlot) {
	o.mu.Lock()
	active, ok := o.active[slot.name]
	if ok {
		delete(o.active, slot.name)
	}
	o.mu.Unlock()
	if ok {
		active.cancel()
	}
}

// Removing all leases under one lock prevents a completed old worker from winning
// a race with a newly installed session projection.
func (o *operationOwner) CancelScope(scope operationScope) {
	o.mu.Lock()
	operations := make([]ownedOperation, 0, len(o.active))
	for slot, operation := range o.active {
		if operation.policy.scope != scope {
			continue
		}
		delete(o.active, slot)
		operations = append(operations, operation)
	}
	o.mu.Unlock()
	for _, operation := range operations {
		operation.cancel()
	}
}

func (o *operationOwner) Release(lease operationLease) bool {
	o.mu.Lock()
	active, ok := o.active[lease.slot.name]
	if ok && active.id == lease.id {
		delete(o.active, lease.slot.name)
	}
	o.mu.Unlock()
	if ok && active.id == lease.id {
		active.cancel()
		return true
	}
	return false
}

func (o *operationOwner) Close() {
	if o == nil {
		return
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	o.closed = true
	active := make([]ownedOperation, 0, len(o.active))
	for _, operation := range o.active {
		active = append(active, operation)
	}
	clear(o.active)
	o.mu.Unlock()

	o.cancel()
	for _, operation := range active {
		operation.cancel()
	}
	o.wg.Wait()
}

func (o *operationOwner) failDelivery(err error) {
	if err == nil || errors.Is(err, program.ErrStopped) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	o.mu.Lock()
	if o.failure == nil {
		o.failure = fmt.Errorf("terminal result delivery: %w", err)
	}
	notify := o.onFailure
	o.mu.Unlock()
	if notify != nil {
		notify()
	}
}

func (o *operationOwner) Failure() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.failure
}

func (o *operationOwner) release(lease operationLease, cancel context.CancelFunc) {
	cancel()
	o.mu.Lock()
	if active, ok := o.active[lease.slot.name]; ok && active.id == lease.id {
		delete(o.active, lease.slot.name)
	}
	o.mu.Unlock()
}

type taskRunner struct {
	operations *operationOwner
	dispatch   program.Dispatcher
	settled    func()
}

func (r taskRunner) deliver(ctx context.Context, apply func()) error {
	err := post(ctx, r.dispatch, apply)
	r.operations.failDelivery(err)
	return err
}

func (r taskRunner) session[T any](slot operationSlot, work func(context.Context) (T, error), apply func(T, error)) bool {
	return runTask(r, operationPolicy{scope: sessionOperationScope}, slot, work, apply)
}

func (r taskRunner) sessionSettlement[T any](slot operationSlot, work func(context.Context) (T, error), apply func(T, error)) bool {
	return runTask(r, operationPolicy{scope: sessionOperationScope, runAdmission: runAdmissionAfterSettlement}, slot, work, apply)
}

func (r taskRunner) sessionFence[T any](slot operationSlot, work func(context.Context) (T, error), apply func(T, error)) bool {
	return runTask(r, operationPolicy{scope: sessionOperationScope, runAdmission: runAdmissionFence}, slot, work, apply)
}

func (r taskRunner) application[T any](slot operationSlot, work func(context.Context) (T, error), apply func(T, error)) bool {
	return runTask(r, operationPolicy{scope: applicationOperationScope}, slot, work, apply)
}

func (r taskRunner) admissionMutation[T any](slot operationSlot, work func(context.Context) (T, error), apply func(T, error)) bool {
	return runTask(r, operationPolicy{scope: applicationOperationScope, runAdmission: runAdmissionAfterSettlement}, slot, work, apply)
}

func runTask[T any](r taskRunner, policy operationPolicy, slot operationSlot, work func(context.Context) (T, error), apply func(T, error)) bool {
	return r.operations.goWithPolicy(policy, slot, func(ctx context.Context, lease operationLease) {
		result, err := work(ctx)
		if context.Cause(ctx) != nil {
			return
		}
		_ = r.deliver(ctx, func() {
			if !r.operations.Current(lease) || !r.operations.Release(lease) {
				return
			}
			apply(result, err)
			if policy.runAdmission == runAdmissionAfterSettlement {
				r.settled()
			}
		})
	})
}

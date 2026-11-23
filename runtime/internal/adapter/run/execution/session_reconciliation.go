package execution

import (
	"errors"
	"time"

	scoperun "github.com/Tangerg/scope/agent/run"
)

func (i *interactionSession) reconcileUnknownEffectsGuarded() {
	defer i.reportExecutionDefect()
	i.reconcileUnknownEffects()
}

func (i *interactionSession) reconcileExecutionStateGuarded() {
	defer i.reportExecutionDefect()
	i.reconcileExecutionState()
}

func (i *interactionSession) stopReconciliation() {
	i.lifetime.stopReconciling()
	i.lifetime.reconcilers.Wait()
}

func (i *interactionSession) reconcileUnknownEffects() {
	ticker := time.NewTicker(i.unknownPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-i.lifetime.unknownWake:
		case <-ticker.C:
		case <-i.lifetime.reconciling.Done():
			return
		}
		if i.reportUnknownEffects() {
			return
		}
	}
}

func (i *interactionSession) reconcileExecutionState() {
	ticker := time.NewTicker(i.statePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-i.lifetime.stateWake:
		case <-ticker.C:
		case <-i.lifetime.reconciling.Done():
			return
		}
		progressed, err := i.reconcileCompletedDelegateChildren(i.lifetime.reconciling)
		if err != nil {
			i.reportProjectionFailure(err)
			return
		}
		if progressed {
			continue
		}
		if i.publishWaitingBoundary() {
			continue
		}
	}
}

func (i *interactionSession) reportUnknownEffects() bool {
	ctx := i.lifetime.reconciling
	if ctx.Err() != nil {
		return true
	}
	ids, err := i.unknownEffectIDs(ctx)
	if err != nil || len(ids) == 0 {
		return false
	}
	resumeDispatch := i.state.holdReceiptInvestigation()
	if resumeDispatch == nil {
		return true
	}
	defer resumeDispatch()
	if _, err := i.investigateToolReceipts(ctx); err != nil {
		if ctx.Err() == nil {
			i.reportProjectionFailure(err)
		}
		return true
	}
	ids, err = i.unknownEffectIDs(ctx)
	if err != nil {
		if ctx.Err() == nil {
			i.reportProjectionFailure(err)
		}
		return true
	}
	if len(ids) == 0 {
		return false
	}
	i.state.mu.Lock()
	if i.state.unknownReported {
		i.state.mu.Unlock()
		return true
	}
	i.state.unknownReported = true
	process := i.state.process
	i.state.mu.Unlock()
	if err := process.Cancel(ctx, unresolvedEffectsStopReason); err != nil && !errors.Is(err, scoperun.ErrProcessFinished) {
		i.reportProjectionFailure(err)
	}
	return true
}

func (i *interactionState) holdReceiptInvestigation() func() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.unknownReported {
		return nil
	}
	if i.investigationGate != nil {
		return func() {}
	}
	gate := make(chan struct{})
	i.investigationGate = gate
	return func() {
		i.mu.Lock()
		defer i.mu.Unlock()
		close(gate)
		i.investigationGate = nil
	}
}

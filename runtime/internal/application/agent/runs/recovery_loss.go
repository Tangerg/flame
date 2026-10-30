package runs

import (
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

// A pending external dependency defers validation without granting resumption.
// The zero value never preserves a waiting tree.
type WaitingResumption struct {
	state waitingResumptionState
	loss  rundomain.LossReason
}

type waitingResumptionState uint8

const (
	waitingUnresumable waitingResumptionState = iota
	waitingResumable
	waitingDeferred
)

func ResumableWaiting() WaitingResumption { return WaitingResumption{state: waitingResumable} }

func DeferredWaiting() WaitingResumption { return WaitingResumption{state: waitingDeferred} }

func UnresumableWaiting(loss rundomain.LossReason) WaitingResumption {
	return WaitingResumption{loss: loss}
}

func (w WaitingResumption) Resumable() bool { return w.state == waitingResumable }
func (w WaitingResumption) Deferred() bool  { return w.state == waitingDeferred }

func (w WaitingResumption) Loss() rundomain.LossReason { return w.loss }

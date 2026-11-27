package runs

import (
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

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

func recoverLostTree(
	tree recoveryRunTree,
	items []transcript.Item,
	closure recoveryConversationClosure,
	finishedAt time.Time,
	ending rundomain.Lost,
	models []ModelInvocationRecovery,
	independent []modelref.EndpointObservation,
) (LostTreeRecovery, error) {
	lost := LostTreeRecovery{
		runs:    make([]rundomain.Replacement, 0, len(tree.postorder)),
		closure: closure,
	}
	messageMark := closure.previousCount + len(closure.messages)
	for _, runID := range tree.postorder {
		active := tree.runsByID[runID]
		for _, item := range items {
			if item.RunID() != active.ID() || item.Status() != transcript.ItemRunning {
				continue
			}
			failure := tool.Failure{
				Kind:   tool.FailureExecution,
				Detail: "tool call interrupted because the run was lost on restart",
			}
			itemReplacement, err := item.AbandonToolCall(&failure, finishedAt)
			if err != nil {
				return LostTreeRecovery{}, fmt.Errorf("runs: recover lost Item %q: %w", item.ID(), err)
			}
			lost.items = append(lost.items, itemReplacement)
		}

		replacement, err := recoverLostRun(active, models, independent, ending, finishedAt, messageMark)
		if err != nil {
			return LostTreeRecovery{}, fmt.Errorf("runs: recover lost Run %q: %w", active.ID(), err)
		}
		lost.runs = append(lost.runs, replacement)
	}
	return lost, nil
}

func recoverLostRun(
	active rundomain.Run,
	models []ModelInvocationRecovery,
	independent []modelref.EndpointObservation,
	ending rundomain.Lost,
	finishedAt time.Time,
	messageMark int,
) (rundomain.Replacement, error) {
	replacement := active.Replace()
	for _, invocation := range models {
		if invocation.RunID != active.ID() || invocation.Endpoint == nil {
			continue
		}
		current := replacement.State()
		record := invocation.Endpoint.Snapshot()
		reported := record.Usage
		metrics, err := current.Metrics().AccrueModelCall(invocation.Endpoint.ServedModel(), reported, record.Cost)
		if err != nil {
			return rundomain.Replacement{}, err
		}
		contextTokens := current.ContextTokens()
		if invocation.Purpose == rundomain.ModelConversation && reported != nil && reported.InputTokens > 0 {
			contextTokens = reported.InputTokens
		}
		if replacement, err = replacement.AdvanceProgress(metrics, contextTokens, finishedAt); err != nil {
			return rundomain.Replacement{}, err
		}
	}
	for _, source := range independent {
		record := source.Snapshot()
		if record.RunID != active.ID() {
			continue
		}
		current := replacement.State()
		metrics, err := current.Metrics().AccrueModelCall(source.ServedModel(), record.Usage, record.Cost)
		if err != nil {
			return rundomain.Replacement{}, err
		}
		if replacement, err = replacement.AdvanceProgress(metrics, current.ContextTokens(), finishedAt); err != nil {
			return rundomain.Replacement{}, err
		}
	}
	return replacement.Terminate(rundomain.Termination{
		Ending: ending, FinishedAt: finishedAt, MessageMark: active.Lineage().MessageMark(messageMark),
	})
}

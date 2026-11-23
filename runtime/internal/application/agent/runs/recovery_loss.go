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
	messageMark int,
	finishedAt time.Time,
	loss rundomain.LossReason,
	models []ModelInvocationRecovery,
	independent []modelref.EndpointObservation,
) ([]rundomain.Replacement, []transcript.Replacement, error) {
	lostRuns := make([]rundomain.Replacement, 0, len(tree.postorder))
	var replacements []transcript.Replacement
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
				return nil, nil, fmt.Errorf("runs: recover lost Item %q: %w", item.ID(), err)
			}
			replacements = append(replacements, itemReplacement)
		}

		replacement, err := recoverLostRun(active, models, independent, rundomain.Loss{Reason: loss}, finishedAt, messageMark)
		if err != nil {
			return nil, nil, fmt.Errorf("runs: recover lost Run %q: %w", active.ID(), err)
		}
		lostRuns = append(lostRuns, replacement)
	}
	return lostRuns, replacements, nil
}

func recoverLostRun(
	active rundomain.Run,
	models []ModelInvocationRecovery,
	independent []modelref.EndpointObservation,
	loss rundomain.Loss,
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
	return replacement.RecoverLost(loss, finishedAt, active.Lineage().MessageMark(messageMark))
}

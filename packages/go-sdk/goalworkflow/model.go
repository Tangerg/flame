package goalworkflow

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/runoutcome"
)

const Reference = "flame.goal-workflow.v1"

type Signal string

const (
	NextRun         Signal = "nextRun"
	Start           Signal = "start"
	Resume          Signal = "resume"
	Stop            Signal = "stop"
	Revise          Signal = "revise"
	ReviseAndResume Signal = "reviseAndResume"
	Pause           Signal = "pause"
	Complete        Signal = "complete"
	Block           Signal = "block"
	TerminalRun     Signal = "terminalRun"
)

type Value struct {
	Objective   string          `json:"objective"`
	Status      GoalStatus      `json:"status"`
	Reason      GoalReasonCode  `json:"reason,omitzero"`
	Outcome     runoutcome.Kind `json:"outcome,omitzero"`
	Explanation string          `json:"explanation,omitzero"`
}

type Request struct {
	Ordinal     uint64          `json:"ordinal,omitzero"`
	Current     *Value          `json:"current,omitzero"`
	Signal      Signal          `json:"signal"`
	Objective   string          `json:"objective,omitzero"`
	Reason      GoalReasonCode  `json:"reason,omitzero"`
	Outcome     runoutcome.Kind `json:"outcome,omitzero"`
	Explanation string          `json:"explanation,omitzero"`
}

type Refusal string

const (
	Invalid               Refusal = "invalid"
	NotResumable          Refusal = "notResumable"
	BlockedReasonRequired Refusal = "blockedReasonRequired"
)

type AuthoredDecision struct {
	Source extension.ContributionSource
	Decision
}
type Decision struct {
	Instructions string  `json:"instructions,omitzero"`
	Next         *Value  `json:"next,omitzero"`
	Refusal      Refusal `json:"refusal,omitzero"`
}

type GoalStatus string

const (
	GoalActive     GoalStatus = "active"
	GoalPaused     GoalStatus = "paused"
	GoalBlocked    GoalStatus = "blocked"
	GoalCompleting GoalStatus = "completing"
)

type GoalReasonCode string

const (
	GoalReasonStoppedByUser          GoalReasonCode = "stoppedByUser"
	GoalReasonRuntimeRestarted       GoalReasonCode = "runtimeRestarted"
	GoalReasonRunStartFailed         GoalReasonCode = "runStartFailed"
	GoalReasonAwaitingInput          GoalReasonCode = "awaitingInput"
	GoalReasonTerminalOutcomeMissing GoalReasonCode = "terminalOutcomeMissing"
	GoalReasonRunNotCompleted        GoalReasonCode = "runNotCompleted"

	GoalReasonBlockedByModel GoalReasonCode = "blockedByModel"
)

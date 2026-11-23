package protocol

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
)

type Goal struct {
	Author          *extension.ContributionSource `json:"author,omitzero"`
	SessionID       string                        `json:"sessionId"`
	Objective       string                        `json:"objective"`
	Status          GoalStatus                    `json:"status"`
	Reason          *GoalReason                   `json:"reason,omitzero"`
	Provider        string                        `json:"provider,omitempty"`
	Model           string                        `json:"model,omitempty"`
	ReasoningEffort string                        `json:"reasoningEffort,omitempty"`
	Used            GoalUsage                     `json:"used"`
	CreatedAt       time.Time                     `json:"createdAt,omitzero"`
	UpdatedAt       time.Time                     `json:"updatedAt,omitzero"`
}

//flame:enum
type GoalStatus string

const (
	GoalActive     GoalStatus = GoalStatus(goalworkflow.GoalActive)
	GoalPaused     GoalStatus = GoalStatus(goalworkflow.GoalPaused)
	GoalBlocked    GoalStatus = GoalStatus(goalworkflow.GoalBlocked)
	GoalCompleting GoalStatus = GoalStatus(goalworkflow.GoalCompleting)
)

type GoalReason struct {
	Code        GoalReasonCode `json:"code"`
	RunOutcome  RunOutcomeType `json:"runOutcome,omitempty"`
	Explanation string         `json:"explanation,omitempty"`
}

//flame:enum
type GoalReasonCode string

const (
	GoalReasonStoppedByUser          GoalReasonCode = GoalReasonCode(goalworkflow.GoalReasonStoppedByUser)
	GoalReasonRuntimeRestarted       GoalReasonCode = GoalReasonCode(goalworkflow.GoalReasonRuntimeRestarted)
	GoalReasonRunStartFailed         GoalReasonCode = GoalReasonCode(goalworkflow.GoalReasonRunStartFailed)
	GoalReasonAwaitingInput          GoalReasonCode = GoalReasonCode(goalworkflow.GoalReasonAwaitingInput)
	GoalReasonTerminalOutcomeMissing GoalReasonCode = GoalReasonCode(goalworkflow.GoalReasonTerminalOutcomeMissing)
	GoalReasonRunNotCompleted        GoalReasonCode = GoalReasonCode(goalworkflow.GoalReasonRunNotCompleted)

	GoalReasonBlockedByModel GoalReasonCode = GoalReasonCode(goalworkflow.GoalReasonBlockedByModel)
)

type GoalUsage struct {
	Runs    int      `json:"runs"`
	CostUSD *float64 `json:"costUsd,omitzero"`
	Steps   int      `json:"steps"`
}

type StartGoalRequest struct {
	SessionID       string `json:"sessionId"`
	Objective       string `json:"objective"`
	Provider        string `json:"provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
}

type UpdateGoalRequest struct {
	SessionID string `json:"sessionId"`
	Objective string `json:"objective"`
}

type GoalRequest struct {
	SessionID string `json:"sessionId"`
}

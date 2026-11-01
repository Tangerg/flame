package protocol

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"time"
)

// Goal is one Session's autonomous objective and loop state.
// status is active | paused | blocked | completing. Completing is the observable
// settlement window after the model has declared success and before the owning
// drive has charged the final Run and cleared the objective.
type Goal struct {
	Author    *extension.ContributionSource `json:"author,omitzero"`
	SessionID string                        `json:"sessionId"`
	Objective string                        `json:"objective"`
	Status    GoalStatus                    `json:"status"`
	Reason    *GoalReason                   `json:"reason,omitzero"`
	// Provider and Model are the Goal's own model override. Absent, each Goal
	// Run uses the Session's selection when it starts.
	Provider        string    `json:"provider,omitempty"`
	Model           string    `json:"model,omitempty"`
	ReasoningEffort string    `json:"reasoningEffort,omitempty"`
	Used            GoalUsage `json:"used"`
	CreatedAt       time.Time `json:"createdAt,omitzero"`
	UpdatedAt       time.Time `json:"updatedAt,omitzero"`
}

// GoalStatus is the lifecycle vocabulary exposed by the autonomous-goal API.
// Completing is intentionally a read-model word rather than the domain's
// complete: the objective is complete, but its owning drive is still settling
// final accounting and remains the lifecycle owner until the row is cleared.
//
//flame:enum
type GoalStatus string

const (
	GoalActive     GoalStatus = GoalStatus(goalworkflow.GoalActive)
	GoalPaused     GoalStatus = GoalStatus(goalworkflow.GoalPaused)
	GoalBlocked    GoalStatus = GoalStatus(goalworkflow.GoalBlocked)
	GoalCompleting GoalStatus = GoalStatus(goalworkflow.GoalCompleting)
)

// GoalReason is machine-readable stopping context. Code determines client
// presentation and behavior.
type GoalReason struct {
	Code GoalReasonCode `json:"code"`
	// RunOutcome is how the Run that paused a runNotCompleted Goal ended.
	RunOutcome RunOutcomeType `json:"runOutcome,omitempty"`
	// Explanation is the model's own account of a blockedByModel Goal.
	Explanation string `json:"explanation,omitempty"`
}

// GoalReasonCode is the closed vocabulary for paused and blocked goals.
//
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

// GoalUsage reports consumption across all Runs for an objective.
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

// UpdateGoalRequest — goals.update body. Updating the objective preserves the
// Goal's lifecycle, accounting and model selection while opening a new
// objective incarnation so work admitted for the prior text cannot mutate it.
type UpdateGoalRequest struct {
	SessionID string `json:"sessionId"`
	Objective string `json:"objective"`
}

// GoalRequest — goals.get / goals.stop / goals.resume body (keyed by session).
type GoalRequest struct {
	SessionID string `json:"sessionId"`
}

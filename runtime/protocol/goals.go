package protocol

import (
	"time"
)

// Goal is one Session's autonomous objective and loop state.
// status is active | paused | blocked | completing. Completing is the observable
// settlement window after the model has declared success and before the owning
// drive has charged the final Run and cleared the objective.
type Goal struct {
	SessionID       string      `json:"sessionId"`
	Objective       string      `json:"objective"`
	Status          GoalStatus  `json:"status"`
	Reason          *GoalReason `json:"reason,omitempty"`
	Provider        string      `json:"provider"`
	Model           string      `json:"model"`
	ReasoningEffort string      `json:"reasoningEffort,omitempty"`
	Used            GoalUsage   `json:"used"`
	CreatedAt       time.Time   `json:"createdAt,omitzero"`
	UpdatedAt       time.Time   `json:"updatedAt,omitzero"`
}

// GoalStatus is the lifecycle vocabulary exposed by the autonomous-goal API.
// Completing is intentionally a read-model word rather than the domain's
// complete: the objective is complete, but its owning drive is still settling
// final accounting and remains the lifecycle owner until the row is cleared.
type GoalStatus string

const (
	GoalActive     GoalStatus = "active"
	GoalPaused     GoalStatus = "paused"
	GoalBlocked    GoalStatus = "blocked"
	GoalCompleting GoalStatus = "completing"
)

// GoalReason is machine-readable stopping context. Code determines client
// presentation and behavior; detail carries only safe domain/model context.
type GoalReason struct {
	Code   GoalReasonCode `json:"code"`
	Detail string         `json:"detail,omitempty"`
}

// GoalReasonCode is the closed vocabulary for paused and blocked goals.
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

// GoalUsage reports consumption across all Runs for an objective.
type GoalUsage struct {
	Runs    int      `json:"runs"`
	CostUSD *float64 `json:"costUsd,omitempty"`
	Steps   int      `json:"steps"`
}

// StartGoalRequest — goals.start body.
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

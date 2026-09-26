package protocol

import (
	"time"
)

// Schedule is one scheduled run. Instructions is the final text
// sent as the run's input. cron is a 5-field standard expression
// ("min hour dom month dow"). lastRunAt is omitted until first fired; nextRunAt
// is omitted when the schedule is disabled.
type Schedule struct {
	ID              string        `json:"id"`
	Title           string        `json:"title"`
	Instructions    string        `json:"instructions"`
	Workspace       *WorkspaceRef `json:"workspace,omitzero"`
	Provider        string        `json:"provider,omitempty"`
	Model           string        `json:"model,omitempty"`
	ReasoningEffort string        `json:"reasoningEffort,omitempty"`
	Cron            string        `json:"cron"`
	Enabled         bool          `json:"enabled"`
	LastRunAt       *time.Time    `json:"lastRunAt,omitzero"`
	NextRunAt       *time.Time    `json:"nextRunAt,omitzero"`
	CreatedAt       time.Time     `json:"createdAt,omitzero"`
	Revision        uint64        `json:"revision"`
}

// CreateScheduleRequest — schedules.create body. A new schedule is enabled.
type CreateScheduleRequest struct {
	Title           string        `json:"title,omitempty"`
	Instructions    string        `json:"instructions"`
	Workspace       *WorkspaceRef `json:"workspace,omitzero"`
	Provider        string        `json:"provider,omitempty"`
	Model           string        `json:"model,omitempty"`
	ReasoningEffort string        `json:"reasoningEffort,omitempty"`
	Cron            string        `json:"cron"`
}

// ScheduleWorkspaceMode selects the Runtime-owned workspace binding for a
// schedule update. Omitting it preserves the current binding.
type ScheduleWorkspaceMode string

const (
	// ScheduleWorkspaceDefault removes an explicit binding so future firings use
	// ServerInfo.defaultWorkspace.
	ScheduleWorkspaceDefault ScheduleWorkspaceMode = "default"
)

// UpdateScheduleRequest — schedules.update body. Editable fields form a
// revision-checked partial patch. Workspace sets an explicit binding;
// WorkspaceMode="default" clears one. Omitting both preserves the binding, and
// they are mutually exclusive.
// Disabling stops future claims. An already claimed occurrence retains its
// accepted input and may still start; cancel its Run separately when required.
type UpdateScheduleRequest struct {
	ID               string                `json:"id"`
	ExpectedRevision uint64                `json:"expectedRevision"`
	Title            *string               `json:"title,omitzero"`
	Instructions     *string               `json:"instructions,omitzero"`
	Workspace        *WorkspaceRef         `json:"workspace,omitzero"`
	WorkspaceMode    ScheduleWorkspaceMode `json:"workspaceMode,omitempty"`
	Provider         *string               `json:"provider,omitzero"`
	Model            *string               `json:"model,omitzero"`
	ReasoningEffort  *string               `json:"reasoningEffort,omitzero"`
	Cron             *string               `json:"cron,omitzero"`
	Enabled          *bool                 `json:"enabled,omitzero"`
}

// DeleteScheduleRequest — schedules.delete body. Deletion stops future claims;
// it does not revoke an already claimed occurrence or cancel its Run.
type DeleteScheduleRequest struct {
	ID string `json:"id"`
}

// RunScheduleNowRequest — schedules.runNow body.
type RunScheduleNowRequest struct {
	ID string `json:"id"`
}

type RunScheduleNowResponse struct {
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId"`
}

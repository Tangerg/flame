package protocol

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/planworkflow"
	"github.com/Tangerg/flame/packages/go-sdk/runevent"
)

type RunEvent struct {
	RunID     string      `json:"runId"`
	SegmentID string      `json:"segmentId"`
	EventID   string      `json:"eventId"`
	Timestamp time.Time   `json:"timestamp,omitzero"`
	Event     StreamEvent `json:"event"`
}

type StreamEvent struct {
	Type runevent.Kind `json:"type"`

	Run        *RunRef      `json:"run,omitzero"`
	Progress   *RunProgress `json:"progress,omitzero"`
	Interrupts []Interrupt  `json:"interrupts,omitempty"`
	Item       *Item        `json:"item,omitzero"`
	ItemID     string       `json:"itemId,omitempty"`
	Delta      *ItemDelta   `json:"delta,omitzero"`
	Plan       *Plan        `json:"plan,omitzero"`
}

type RunProgress struct {
	Step          *int      `json:"step,omitzero"`
	Usage         *Usage    `json:"usage,omitzero"`
	ContextTokens *int64    `json:"contextTokens,omitzero"`
	Activity      *Activity `json:"activity,omitzero"`
}

//flame:enum
type ActivityType string

const (
	ActivityCallingModel      ActivityType = "callingModel"
	ActivityModelCallFailed   ActivityType = "modelCallFailed"
	ActivityApplyingUserInput ActivityType = "applyingUserInput"
	ActivityToolCall          ActivityType = "toolCall"
)

type Activity struct {
	Type   ActivityType `json:"type"`
	ItemID string       `json:"itemId,omitempty"`
	Tool   string       `json:"tool,omitempty"`
}

type Plan struct {
	SessionID string     `json:"sessionId"`
	State     *PlanState `json:"state,omitzero"`
}

type PlanState struct {
	Revision  uint64     `json:"revision"`
	Steps     []PlanStep `json:"steps"`
	UpdatedAt time.Time  `json:"updatedAt,omitzero"`
}

type GetPlanRequest struct {
	SessionID string `json:"sessionId"`
}

type PlanStep struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Status      PlanStatus `json:"status"`
}

//flame:enum
type PlanStatus string

const (
	PlanStatusPending    PlanStatus = PlanStatus(planworkflow.PlanStatusPending)
	PlanStatusInProgress PlanStatus = PlanStatus(planworkflow.PlanStatusInProgress)
	PlanStatusCompleted  PlanStatus = PlanStatus(planworkflow.PlanStatusCompleted)
)

//flame:enum
type ItemDeltaType string

const (
	DeltaContent       ItemDeltaType = "content"
	DeltaReasoning     ItemDeltaType = "reasoning"
	DeltaToolArguments ItemDeltaType = "toolArguments"
	DeltaToolOutput    ItemDeltaType = "toolOutput"
)

type ItemDelta struct {
	Type ItemDeltaType `json:"type"`

	Text               string `json:"text,omitempty"`
	ArgumentsTextDelta string `json:"argumentsTextDelta,omitempty"`
}

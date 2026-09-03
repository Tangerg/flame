package protocol

import "time"

// RunEvent is the params of the notifications.run.event notification —
// the single downstream stream carrying segment / item / Plan events. RunID is
// the stable logical run; SegmentID is the streamed
// segment the event belongs to (§0.3) — a client scopes its stream tree +
// reconnect-replay dedup to it. eventId is monotonic within one segment stream.
//
// There is no per-frame reliability flag. Authoritativeness and replayability
// are protocol facts owned by the event type.
type RunEvent struct {
	RunID     string      `json:"runId"`
	SegmentID string      `json:"segmentId"`          // seg_…
	EventID   string      `json:"eventId"`            // evt_…
	Timestamp time.Time   `json:"timestamp,omitzero"` // ISO-8601 (time.Time marshals to RFC3339)
	Event     StreamEvent `json:"event"`
}

// StreamEventType discriminates the StreamEvent union.
type StreamEventType string

const (
	StreamSegmentStarted  StreamEventType = "segment.started"
	StreamSegmentProgress StreamEventType = "segment.progress"
	StreamSegmentFinished StreamEventType = "segment.finished"
	StreamItemStarted     StreamEventType = "item.started"
	StreamItemDelta       StreamEventType = "item.delta"
	StreamItemCompleted   StreamEventType = "item.completed"
	StreamPlanUpdated     StreamEventType = "plan.updated"
)

// StreamEvent is a tag-discriminated union over downstream events. Type selects
// which optional fields apply.
//
//	segment.started     → Run
//	segment.progress    → Progress
//	segment.finished    → Outcome, Metrics, ContextTokens
//	item.started    → Item
//	item.delta      → ItemID, Delta
//	item.completed  → Item
//	plan.updated    → Plan
type StreamEvent struct {
	Type StreamEventType `json:"type"`

	Run      *RunRef         `json:"run,omitempty"`
	Progress *RunProgress    `json:"progress,omitempty"`
	Outcome  *SegmentOutcome `json:"outcome,omitempty"`
	// Metrics rides every segment.finished, terminal or not: a client reads what
	// the run consumed from one field instead of looking for it in whichever
	// branch of the outcome happens to carry it.
	Metrics *RunMetrics `json:"metrics,omitempty"`
	// ContextTokens is the final durable prompt footprint at this segment
	// boundary. It repeats the latest progress preview because progress is not
	// replayable: a reconnecting client must recover the same RunRef value from
	// the authoritative completion frame alone.
	ContextTokens *int64     `json:"contextTokens,omitempty"`
	Item          *Item      `json:"item,omitempty"`
	ItemID        string     `json:"itemId,omitempty"`
	Delta         *ItemDelta `json:"delta,omitempty"`
	Plan          *Plan      `json:"plan,omitempty"`
}

// Authoritative reports whether the event itself is a fact a client may fold.
// It is deliberately separate from [StreamEvent.Replayable]: the current core
// event set happens to give every authoritative frame a replay window, but
// neither concept defines the other.
func (s StreamEvent) Authoritative() bool {
	switch s.Type {
	case StreamSegmentStarted, StreamSegmentFinished,
		StreamItemStarted, StreamItemCompleted, StreamPlanUpdated:
		return true
	default:
		return false
	}
}

// Replayable reports whether the Runtime-instance-local segment journal retains this
// event and whether its HTTP frame receives an SSE id. Unknown events fail
// closed and never enter the replay window.
func (s StreamEvent) Replayable() bool {
	switch s.Type {
	case StreamSegmentStarted, StreamSegmentFinished,
		StreamItemStarted, StreamItemCompleted, StreamPlanUpdated:
		return true
	default:
		return false
	}
}

// RunProgress is the mid-run progress preview carried by a segment.progress
// event. It previews the same run-cumulative figures
// that land authoritatively on segment.finished.metrics, so it may run briefly
// ahead of them but never contradicts them.
type RunProgress struct {
	Step  *int   `json:"step,omitempty"`
	Usage *Usage `json:"usage,omitempty"`
	// ContextTokens is the latest round's prompt-token count — the live
	// context-window occupancy (how full the window is right now), distinct from
	// the cumulative-over-rounds Usage.inputTokens (which only grows). Pair it
	// with the served model's contextWindow (models.list) for an occupancy gauge;
	// it drops after a compaction. Ephemeral, like the rest of RunProgress.
	ContextTokens *int64 `json:"contextTokens,omitempty"`
	Activity      string `json:"activity,omitempty"` // human-readable current action
}

// Plan is the Session's persisted latest Plan. A root Run publishes it through
// plan.updated, and plan.get returns the same shape, so live and cold recovery cannot
// describe the checklist differently (§5.2 / §5.3). A segment that changed the Plan
// republishes its final revision immediately before segment.finished; consumers fold
// identical revision + content idempotently even though the fence has a fresh eventId.
// State is absent when no Plan replacement has ever been written; a committed empty
// State means the Plan was explicitly cleared. This keeps absence distinct without a
// magic revision zero.
type Plan struct {
	SessionID string     `json:"sessionId"`
	State     *PlanState `json:"state,omitempty"`
}

// PlanState is one committed whole-list replacement. Revision and UpdatedAt are
// coupled to Steps as one value, so a frame cannot carry version metadata without
// content or content without the version that orders it.
type PlanState struct {
	Revision  uint64     `json:"revision"`
	Steps     []PlanStep `json:"steps"`
	UpdatedAt time.Time  `json:"updatedAt,omitzero"`
}

// GetPlanRequest is the plan.get body — the cold read for the Plan projection.
type GetPlanRequest struct {
	SessionID string `json:"sessionId"`
}

// PlanStep is one Step of the session [Plan].
// The Plan is replaced whole each set_plan, so ID is positional — a stable key
// within a snapshot, not a durable identity. Status is
// "pending" | "in_progress" | "completed".
type PlanStep struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Status      PlanStatus `json:"status"`
}

// PlanStatus is one Step's execution state.
type PlanStatus string

const (
	PlanStatusPending    PlanStatus = "pending"
	PlanStatusInProgress PlanStatus = "in_progress"
	PlanStatusCompleted  PlanStatus = "completed"
)

// ItemDeltaType discriminates the ItemDelta union.
type ItemDeltaType string

const (
	DeltaContent       ItemDeltaType = "content"
	DeltaReasoning     ItemDeltaType = "reasoning"
	DeltaToolArguments ItemDeltaType = "toolArguments"
	DeltaToolOutput    ItemDeltaType = "toolOutput"
)

// ItemDelta is a tag-discriminated union over incremental updates. All delta
// events are non-authoritative and non-replayable.
//
//	content       → Text
//	reasoning     → Text
//	toolArguments → ArgumentsTextDelta (partial JSON text; client repairs)
//	toolOutput    → Text
type ItemDelta struct {
	Type ItemDeltaType `json:"type"`

	Text               string `json:"text,omitempty"`
	ArgumentsTextDelta string `json:"argumentsTextDelta,omitempty"`
}

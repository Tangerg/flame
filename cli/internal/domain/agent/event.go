package agent

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"time"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

// RunEvent is one projected runtime event. EventID is an opaque replay token
// scoped to the root StreamSegmentID; SegmentID identifies the producing run.
type RunEvent struct {
	EventID         string
	RunID           string
	SegmentID       string
	StreamSegmentID string
	At              time.Time
	Event           Event
}

// StreamSegment returns the root segment whose journal owns this event. A
// root-only producer may omit StreamSegmentID because its producer segment is
// also the stream segment; tree adapters set it explicitly for every event.
func (r RunEvent) StreamSegment() string {
	if r.StreamSegmentID != "" {
		return r.StreamSegmentID
	}
	return r.SegmentID
}

// Validate enforces the CLI-owned event envelope and payload identity without
// depending on the conversation aggregate that later folds the event.
func (r RunEvent) Validate() error {
	if err := runtimeprotocol.ValidateRunEventID(r.EventID); err != nil {
		return fmt.Errorf("run event: %w", err)
	}
	if err := runtimeprotocol.ValidateRunID(r.RunID); err != nil {
		return fmt.Errorf("run event: %w", err)
	}
	if err := runtimeprotocol.ValidateSegmentID(r.SegmentID); err != nil {
		return fmt.Errorf("run event: %w", err)
	}
	if err := runtimeprotocol.ValidateSegmentID(r.StreamSegment()); err != nil {
		return fmt.Errorf("run event stream: %w", err)
	}
	if r.Event == nil {
		return errors.New("run event payload is nil")
	}
	if err := ValidateEvent(r.Event); err != nil {
		return fmt.Errorf("run event payload: %w", err)
	}
	switch event := r.Event.(type) {
	case SegmentStarted:
		if event.Run.ID != r.RunID || event.Run.ActiveSegmentID != r.SegmentID {
			return errors.New("run event segment-start identity does not match its envelope")
		}
	case BlockStarted:
		if event.Block.RunID != r.RunID {
			return fmt.Errorf("run event block %s belongs to run %s, not %s", event.Block.ID, event.Block.RunID, r.RunID)
		}
	case BlockCompleted:
		if event.Block.RunID != r.RunID {
			return fmt.Errorf("run event block %s belongs to run %s, not %s", event.Block.ID, event.Block.RunID, r.RunID)
		}
	}
	return nil
}

func (r RunEvent) Clone() RunEvent {
	r.Event = CloneEvent(r.Event)
	return r
}

// Equal reports whether two envelopes contain the same durable event fact.
// Timestamp equality is based on the instant, not time.Location pointer or a
// process-local monotonic reading that cannot survive persistence.
func (r RunEvent) Equal(other RunEvent) bool {
	return r.EventID == other.EventID && r.RunID == other.RunID && r.SegmentID == other.SegmentID &&
		r.StreamSegment() == other.StreamSegment() && r.At.Equal(other.At) && equalEvent(r.Event, other.Event)
}

type Event interface {
	isEvent()
	equal(Event) bool
}

// SegmentStarted is the authoritative opening fact of every initial or resumed
// run segment.
type SegmentStarted struct{ Run Run }

type BlockStarted struct{ Block Block }

type BlockDelta struct {
	BlockID string
	Text    string
}

// ToolArgumentsDelta carries the provisional JSON text used to assemble a
// tool invocation. The completed tool block remains authoritative; preserving
// this preview lets streaming clients expose it without teaching the
// conversation aggregate how to repair partial JSON.
type ToolArgumentsDelta struct {
	BlockID string
	Text    string
}

// RunProgress is a non-replayable preview of a running segment. Usage is
// run-cumulative when present; ContextTokens is the current context-window
// occupancy and may decrease after compaction.
type RunProgress struct {
	Step          *int
	Usage         *Usage
	ContextTokens *int64
	Activity      string
}

// CustomEvent preserves an extension event without coupling the CLI domain to
// a vendor-specific payload type. PayloadJSON always contains one valid JSON
// value, including "null" when the runtime supplied no payload.
type CustomEvent struct {
	Name        string
	PayloadJSON []byte
}

type BlockCompleted struct{ Block Block }

type PlanChanged struct {
	Plan runtimeprotocol.Plan
}

// RunInterrupted closes the current segment and parks the stable logical run.
// Interactions is the complete pending set that must be answered atomically;
// Usage and ContextTokens are the complete durable Run facts committed at that
// segment boundary.
type RunInterrupted struct {
	Interactions  []Interaction
	Usage         Usage
	ContextTokens int64
}

// RunSuspended closes a member segment because another run in the same tree
// interrupted. It carries no duplicate interactions; the tree-level pending
// set is assembled from the member that raised them.
type RunSuspended struct {
	Usage         Usage
	ContextTokens int64
}

type RunFinished struct {
	Outcome       Outcome
	Usage         Usage
	ContextTokens int64
}

func (SegmentStarted) isEvent()     {}
func (BlockStarted) isEvent()       {}
func (BlockDelta) isEvent()         {}
func (ToolArgumentsDelta) isEvent() {}
func (RunProgress) isEvent()        {}
func (CustomEvent) isEvent()        {}
func (BlockCompleted) isEvent()     {}
func (PlanChanged) isEvent()        {}
func (RunInterrupted) isEvent()     {}
func (RunSuspended) isEvent()       {}
func (RunFinished) isEvent()        {}

func (item SegmentStarted) equal(event Event) bool {
	other, ok := event.(SegmentStarted)
	return ok && item.Run.Equal(other.Run)
}

func (item BlockStarted) equal(event Event) bool {
	other, ok := event.(BlockStarted)
	return ok && item.Block.Equal(other.Block)
}

func (item BlockDelta) equal(event Event) bool {
	other, ok := event.(BlockDelta)
	return ok && item == other
}

func (item ToolArgumentsDelta) equal(event Event) bool {
	other, ok := event.(ToolArgumentsDelta)
	return ok && item == other
}

func (item RunProgress) equal(event Event) bool {
	other, ok := event.(RunProgress)
	return ok && equalOptional(item.Step, other.Step) && equalOptionalUsage(item.Usage, other.Usage) &&
		equalOptional(item.ContextTokens, other.ContextTokens) && item.Activity == other.Activity
}

func (item CustomEvent) equal(event Event) bool {
	other, ok := event.(CustomEvent)
	return ok && item.Name == other.Name && bytes.Equal(item.PayloadJSON, other.PayloadJSON)
}

func (item BlockCompleted) equal(event Event) bool {
	other, ok := event.(BlockCompleted)
	return ok && item.Block.Equal(other.Block)
}

func (item PlanChanged) equal(event Event) bool {
	other, ok := event.(PlanChanged)
	return ok && equalPlans(&item.Plan, &other.Plan)
}

func (item RunInterrupted) equal(event Event) bool {
	other, ok := event.(RunInterrupted)
	return ok && item.ContextTokens == other.ContextTokens && item.Usage.Equal(other.Usage) &&
		equalInteractions(item.Interactions, other.Interactions)
}

func (item RunSuspended) equal(event Event) bool {
	other, ok := event.(RunSuspended)
	return ok && item.ContextTokens == other.ContextTokens && item.Usage.Equal(other.Usage)
}

func (item RunFinished) equal(event Event) bool {
	other, ok := event.(RunFinished)
	return ok && item.ContextTokens == other.ContextTokens &&
		item.Outcome.Equal(other.Outcome) && item.Usage.Equal(other.Usage)
}

// ReplayableEvent reports whether the underlying runtime retains this event in
// its segment journal. Deltas are deliberately ephemeral.
func ReplayableEvent(event Event) bool {
	switch event.(type) {
	case SegmentStarted, BlockStarted, BlockCompleted, PlanChanged, RunInterrupted, RunSuspended, RunFinished:
		return true
	default:
		return false
	}
}

func CloneEvent(event Event) Event {
	switch item := event.(type) {
	case SegmentStarted:
		item.Run = item.Run.Clone()
		return item
	case BlockStarted:
		item.Block = item.Block.Clone()
		return item
	case BlockDelta:
		return item
	case ToolArgumentsDelta:
		return item
	case RunProgress:
		if item.Step != nil {
			item.Step = new(*item.Step)
		}
		if item.Usage != nil {
			usage := item.Usage.Clone()
			item.Usage = &usage
		}
		if item.ContextTokens != nil {
			item.ContextTokens = new(*item.ContextTokens)
		}
		return item
	case CustomEvent:
		item.PayloadJSON = bytes.Clone(item.PayloadJSON)
		return item
	case BlockCompleted:
		item.Block = item.Block.Clone()
		return item
	case PlanChanged:
		item.Plan = *clonePlan(&item.Plan)
		return item
	case RunInterrupted:
		item.Interactions = CloneInteractions(item.Interactions)
		item.Usage = item.Usage.Clone()
		return item
	case RunSuspended:
		item.Usage = item.Usage.Clone()
		return item
	case RunFinished:
		item.Outcome = item.Outcome.Clone()
		item.Usage = item.Usage.Clone()
		return item
	default:
		return nil
	}
}

func equalEvent(left, right Event) bool {
	if left == nil {
		return right == nil
	}
	return left.equal(right)
}

func equalOptional[T comparable](left, right *T) bool {
	return (left == nil) == (right == nil) && (left == nil || *left == *right)
}

func equalOptionalUsage(left, right *Usage) bool {
	return (left == nil) == (right == nil) && (left == nil || left.Equal(*right))
}

func equalInteractions(left, right []Interaction) bool {
	return slices.EqualFunc(left, right, func(left, right Interaction) bool {
		switch item := left.(type) {
		case Approval:
			other, ok := right.(Approval)
			return ok && item.Equal(other)
		case Question:
			other, ok := right.(Question)
			return ok && item.Equal(other)
		case nil:
			return right == nil
		default:
			return false
		}
	})
}

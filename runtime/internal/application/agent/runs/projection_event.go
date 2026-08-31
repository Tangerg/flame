package runs

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

type ProjectionEvent interface {
	runEvent()
	// validate makes publication safety a compile-time obligation of the closed
	// event family. A new variant cannot enter projection without defining what a
	// well-formed value means.
	validate() error
	Replayable() bool
	Terminal() bool
	// retainedBytes reports the approximate heap retained by this value. Keeping
	// it on the closed event family makes retention accounting a compile-time
	// obligation whenever a new event variant is introduced.
	retainedBytes() int
}

type SegmentStarted struct{ Run run.Run }
type SegmentProgressed struct{ Progress Progress }
type SegmentFinished struct {
	Run        run.Run
	Interrupts []transcript.Interrupt
}

// ItemStart is the provisional stream projection emitted before a complete
// transcript fact exists. It is deliberately an Application value: message and
// reasoning deltas need a rendering anchor, but they are not running Domain
// Items. Only a running ToolCall may carry a durable Item into a waiting
// boundary.
type ItemStart struct {
	SessionID      string
	RunID          string
	ItemID         string
	Kind           transcript.ItemKind
	OccurredAt     time.Time
	ToolInvocation *transcript.ToolInvocation
	SafetyClass    tool.SafetyClass

	durable *transcript.Item
}

type ItemStarted struct{ Item ItemStart }
type ItemChanged struct {
	ItemID string
	Delta  ItemDelta
}
type ItemCompleted struct {
	Item         transcript.Item
	mutatedPaths []string
}

// PlanSnapshot publishes a persisted latest-value projection the run changed. It
// carries the projection's own revision, not just its contents: the list is
// replaced wholesale, so a fold that only saw contents could not tell an older
// snapshot from a newer one.
type PlanSnapshot struct {
	SessionID string
	Steps     []plan.Step
	Revision  uint64
	UpdatedAt time.Time
}

// validate proves this is a Plan replacement that may be published. The zero
// Plan state is useful to a cold query, but it is not a change and therefore is
// never a plan.updated event.
func (p PlanSnapshot) validate() error {
	if _, err := resourceid.ParseSession(p.SessionID); err != nil {
		return fmt.Errorf("runs: Plan snapshot: %w", err)
	}
	if p.Revision == 0 {
		return errors.New("runs: Plan snapshot revision must be positive")
	}
	if p.UpdatedAt.IsZero() {
		return errors.New("runs: Plan snapshot update time is required")
	}
	if p.UpdatedAt.Location() != time.UTC {
		return errors.New("runs: Plan snapshot update time must be canonical UTC")
	}
	if err := plan.ValidateSteps(p.Steps); err != nil {
		return fmt.Errorf("runs: Plan snapshot: %w", err)
	}
	return nil
}

func (p PlanSnapshot) clone() PlanSnapshot {
	p.Steps = slices.Clone(p.Steps)
	return p
}

func (SegmentStarted) runEvent()    {}
func (SegmentProgressed) runEvent() {}
func (SegmentFinished) runEvent()   {}
func (ItemStarted) runEvent()       {}
func (ItemChanged) runEvent()       {}
func (ItemCompleted) runEvent()     {}
func (PlanSnapshot) runEvent()      {}

func (s SegmentStarted) validate() error {
	if err := s.Run.Validate(); err != nil {
		return fmt.Errorf("runs: started Segment Run: %w", err)
	}
	if s.Run.State() != run.Running {
		return fmt.Errorf("runs: started Segment carries %s Run", s.Run.State())
	}
	return nil
}

func (s SegmentProgressed) validate() error { return s.Progress.validate() }

func (s SegmentFinished) validate() error {
	if err := s.Run.Validate(); err != nil {
		return fmt.Errorf("runs: finished Segment Run: %w", err)
	}
	if s.Run.State() == run.Running {
		return errors.New("runs: finished Segment carries a running Run")
	}
	if s.Run.State().IsTerminal() && len(s.Interrupts) != 0 {
		return errors.New("runs: terminal Segment carries pending interrupts")
	}
	seen := make(map[string]struct{}, len(s.Interrupts))
	for index, pending := range s.Interrupts {
		if err := validateInterrupt(pending); err != nil {
			return fmt.Errorf("runs: finished Segment interrupt %d: %w", index, err)
		}
		if _, duplicate := seen[pending.ItemID]; duplicate {
			return fmt.Errorf("runs: finished Segment repeats interrupt Item %q", pending.ItemID)
		}
		seen[pending.ItemID] = struct{}{}
	}
	return nil
}

func (i ItemStarted) validate() error { return i.Item.validate() }

func (i ItemCompleted) validate() error {
	if err := i.Item.Validate(); err != nil {
		return fmt.Errorf("runs: completed Item: %w", err)
	}
	return nil
}

func (SegmentStarted) Replayable() bool    { return true }
func (SegmentProgressed) Replayable() bool { return false }
func (SegmentFinished) Replayable() bool   { return true }
func (ItemStarted) Replayable() bool       { return true }
func (ItemChanged) Replayable() bool       { return false }
func (ItemCompleted) Replayable() bool     { return true }
func (PlanSnapshot) Replayable() bool      { return true }

func (SegmentStarted) Terminal() bool    { return false }
func (SegmentProgressed) Terminal() bool { return false }
func (SegmentFinished) Terminal() bool   { return true }
func (ItemStarted) Terminal() bool       { return false }
func (ItemChanged) Terminal() bool       { return false }
func (ItemCompleted) Terminal() bool     { return false }
func (PlanSnapshot) Terminal() bool      { return false }

func (s SegmentStarted) retainedBytes() int  { return retainedRunBytes(s.Run) }
func (SegmentProgressed) retainedBytes() int { return 0 }
func (s SegmentFinished) retainedBytes() int {
	bytes := retainedRunBytes(s.Run) + cap(s.Interrupts)*retainedInterruptBytes
	for _, pending := range s.Interrupts {
		bytes += retainedInterruptPayloadBytes(pending)
	}
	return bytes
}
func (i ItemStarted) retainedBytes() int   { return retainedItemStartBytes(i.Item) }
func (ItemChanged) retainedBytes() int     { return 0 }
func (i ItemCompleted) retainedBytes() int { return retainedItemBytes(i.Item) }
func (p PlanSnapshot) retainedBytes() int  { return retainedPlanSnapshotBytes(p) }

type Progress struct {
	Step          *int
	Usage         *accounting.Usage
	ContextTokens *int64
	Activity      string
}

// validate proves a segment.progress event carries at least one real preview
// fact. Its fields are independent because one executor report may advance
// several at once; the value is not a bag of arbitrary metadata.
func (p Progress) validate() error {
	if p.Step == nil && p.Usage == nil && p.ContextTokens == nil && p.Activity == "" {
		return errors.New("runs: progress carries no facts")
	}
	if p.Step != nil && *p.Step < 0 {
		return errors.New("runs: progress step must not be negative")
	}
	if p.Usage != nil {
		if err := p.Usage.Validate(); err != nil {
			return fmt.Errorf("runs: progress usage: %w", err)
		}
	}
	if p.ContextTokens != nil && *p.ContextTokens < 0 {
		return errors.New("runs: progress context tokens must not be negative")
	}
	if p.Activity != strings.TrimSpace(p.Activity) {
		return errors.New("runs: progress activity must not have surrounding whitespace")
	}
	return nil
}

// ItemDelta is the closed family of provisional Item changes. Each concrete
// value owns exactly the payload its kind can carry, so Application code cannot
// construct the impossible field combinations the flat wire union must use.
type ItemDelta interface {
	itemDelta()
	validate() error
}

// ContentItemDelta appends text to one provisional AgentMessage.
type ContentItemDelta struct{ text string }

// ReasoningItemDelta appends text to one provisional Reasoning Item.
type ReasoningItemDelta struct{ text string }

// ToolArgumentsItemDelta appends partial JSON text to a ToolCall preview.
type ToolArgumentsItemDelta struct{ text string }

// ToolOutputItemDelta appends human-readable Tool output to a ToolCall preview.
type ToolOutputItemDelta struct{ text string }

func newContentItemDelta(text string) (ContentItemDelta, error) {
	delta := ContentItemDelta{text: text}
	return delta, delta.validate()
}

func newReasoningItemDelta(text string) (ReasoningItemDelta, error) {
	delta := ReasoningItemDelta{text: text}
	return delta, delta.validate()
}

func newToolArgumentsItemDelta(text string) (ToolArgumentsItemDelta, error) {
	delta := ToolArgumentsItemDelta{text: text}
	return delta, delta.validate()
}

func newToolOutputItemDelta(text string) (ToolOutputItemDelta, error) {
	delta := ToolOutputItemDelta{text: text}
	return delta, delta.validate()
}

func (ContentItemDelta) itemDelta()       {}
func (ReasoningItemDelta) itemDelta()     {}
func (ToolArgumentsItemDelta) itemDelta() {}
func (ToolOutputItemDelta) itemDelta()    {}

func (d ContentItemDelta) validate() error {
	return validateItemDeltaText("content", d.text)
}

func (d ReasoningItemDelta) validate() error {
	return validateItemDeltaText("reasoning", d.text)
}

func (d ToolArgumentsItemDelta) validate() error {
	return validateItemDeltaText("Tool arguments", d.text)
}

func (d ToolOutputItemDelta) validate() error {
	return validateItemDeltaText("Tool output", d.text)
}

func validateItemDeltaText(kind, text string) error {
	if text == "" {
		return fmt.Errorf("runs: %s Item delta is empty", kind)
	}
	return nil
}

func (d ContentItemDelta) Text() string       { return d.text }
func (d ReasoningItemDelta) Text() string     { return d.text }
func (d ToolArgumentsItemDelta) Text() string { return d.text }
func (d ToolOutputItemDelta) Text() string    { return d.text }

func (i ItemChanged) validate() error {
	if _, err := resourceid.ParseItem(i.ItemID); err != nil {
		return fmt.Errorf("runs: changed Item: %w", err)
	}
	if i.Delta == nil {
		return errors.New("runs: changed Item delta is required")
	}
	return i.Delta.validate()
}

func newTransientItemStart(identity transcript.ItemIdentity, kind transcript.ItemKind) (ItemStart, error) {
	if err := identity.Validate(); err != nil {
		return ItemStart{}, err
	}
	if kind != transcript.AgentMessage && kind != transcript.Reasoning {
		return ItemStart{}, fmt.Errorf("runs: Item start kind %q is not a transient stream", kind)
	}
	return ItemStart{
		SessionID: identity.SessionID, RunID: identity.RunID, ItemID: identity.ItemID,
		Kind: kind, OccurredAt: identity.OccurredAt,
	}, nil
}

func newToolItemStart(item transcript.Item) (ItemStart, error) {
	if item.Kind() != transcript.ToolCall || item.Status() != transcript.ItemRunning {
		return ItemStart{}, errors.New("runs: durable Item start is not a running ToolCall")
	}
	invocation, ok := item.ToolInvocation()
	if !ok {
		return ItemStart{}, errors.New("runs: running ToolCall has no invocation")
	}
	owned := item
	return ItemStart{
		SessionID: item.SessionID(), RunID: item.RunID(), ItemID: item.ID(),
		Kind: item.Kind(), OccurredAt: item.OccurredAt(), ToolInvocation: &invocation,
		SafetyClass: item.SafetyClass(), durable: &owned,
	}, nil
}

func (i ItemStart) validate() error {
	if err := (transcript.ItemIdentity{
		SessionID: i.SessionID, RunID: i.RunID, ItemID: i.ItemID,
		OccurredAt: i.OccurredAt,
	}).Validate(); err != nil {
		return err
	}
	if i.Kind != transcript.AgentMessage && i.Kind != transcript.Reasoning && i.Kind != transcript.ToolCall {
		return fmt.Errorf("runs: unsupported Item start kind %q", i.Kind)
	}
	if i.Kind != transcript.ToolCall {
		if i.ToolInvocation != nil || i.SafetyClass != "" || i.durable != nil {
			return errors.New("runs: transient Item start carries ToolCall facts")
		}
		return nil
	}
	if i.ToolInvocation == nil || strings.TrimSpace(i.ToolInvocation.Name) == "" || i.durable == nil {
		return errors.New("runs: ToolCall start has no durable invocation")
	}
	item := *i.durable
	if item.SessionID() != i.SessionID || item.RunID() != i.RunID ||
		item.ID() != i.ItemID || item.Kind() != i.Kind ||
		!item.OccurredAt().Equal(i.OccurredAt) {
		return errors.New("runs: ToolCall start differs from its durable Item")
	}
	invocation, present := item.ToolInvocation()
	if !present || invocation.Name != i.ToolInvocation.Name ||
		invocation.Arguments != i.ToolInvocation.Arguments ||
		item.SafetyClass() != i.SafetyClass {
		return errors.New("runs: ToolCall start differs from its durable invocation")
	}
	return item.Validate()
}

// Package conversation folds Runtime facts into CLI-owned transcript and interrupt projections.
package conversation

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/protocol"
)

var (
	ErrUnknownBlock      = errors.New("unknown transcript block")
	ErrInvalidTransition = errors.New("invalid conversation transition")
)

type EventAcceptance struct{ Applied bool }

type Phase string

const (
	Idle    Phase = "idle"
	Running Phase = "running"
	Waiting Phase = "waiting"
)

// Valid reports whether c belongs to the live conversation lifecycle.
func (c Phase) Valid() bool {
	return c == Idle || c == Running || c == Waiting
}

// Conversation is the terminal-facing aggregate. Durable history is restored
// from Items; live state is folded from one exact segment stream at a time.
type Conversation struct {
	blocks     []Block
	plan       *protocol.Plan
	interrupts []Interrupt

	// opening is the only lifecycle fact held outside a Run: a root start the
	// terminal requested before Runtime named its Run. Once runID is set the
	// root Run alone answers phase, usage and outcome.
	opening     opening
	runID       string
	segmentID   string
	checkpoint  string
	seen        map[string]RunEvent
	runs        map[string]Run
	runOrder    []string
	index       map[string]int
	textStreams map[string]StreamedText
	recovery    tailRecovery
	// restoredPlanRevision is the durable Plan watermark installed by the last
	// cold snapshot. Plan persistence and stream publication are separate facts:
	// an attached read can already contain a revision whose PlanChanged event is
	// published after unrelated preview or progress events. Keeping the watermark
	// on the Plan itself prevents those events from prematurely ending overlap
	// reconciliation while preserving strict ordering for later revisions.
	restoredPlanRevision uint64
}

// tailRecovery records how much of an active Segment's tail a restored
// projection may already hold. A cold read can miss non-durable previews; an
// attached read additionally overlaps the replay that follows it until the
// first event folds.
type tailRecovery uint8

const (
	recoveryNone tailRecovery = iota
	recoveryColdTail
	recoveryOverlap
)

type opening uint8

const (
	openingNone opening = iota
	openingPending
	openingCanceled
)

func New() *Conversation {
	return &Conversation{
		seen:        make(map[string]RunEvent),
		runs:        make(map[string]Run),
		index:       make(map[string]int),
		textStreams: make(map[string]StreamedText),
	}
}

// ValidateInterruptReview checks a frozen review against the current root's
// complete waiting set. Each interrupt retains its owning member Run ID.
func (c *Conversation) ValidateInterruptReview(rootRunID string, interrupts []Interrupt) error {
	if c.Phase() != Waiting || c.runID != rootRunID || !InterruptsEqual(c.interrupts, interrupts) {
		return errors.New("interrupt review no longer matches the waiting root")
	}
	return nil
}

func (c *Conversation) Blocks() []Block { return cloneBlocks(c.blocks) }

func (c *Conversation) Plan() *protocol.Plan { return ClonePlan(c.plan) }

func (c *Conversation) PlanItems() []protocol.PlanStep {
	if c.plan == nil || c.plan.State == nil {
		return nil
	}
	return append([]protocol.PlanStep{}, c.plan.State.Steps...)
}

func (c *Conversation) Usage() Usage {
	if c.runID == "" {
		return Usage{}
	}
	return c.runs[c.runID].Usage.Clone()
}

func (c *Conversation) Interrupts() []Interrupt { return CloneInterrupts(c.interrupts) }

func (c *Conversation) Outcome() Outcome {
	if c.runID == "" {
		if c.opening == openingCanceled {
			return Outcome{Status: protocol.OutcomeCanceled}
		}
		return Outcome{}
	}
	return c.runs[c.runID].Outcome.Clone()
}

func (c *Conversation) Phase() Phase {
	if c.runID == "" {
		if c.opening == openingPending {
			return Running
		}
		return Idle
	}
	switch c.runs[c.runID].Status {
	case protocol.RunStatusRunning:
		return Running
	case protocol.RunStatusWaiting:
		return Waiting
	default:
		return Idle
	}
}

func (c *Conversation) RunID() string { return c.runID }

func (c *Conversation) SegmentID() string { return c.segmentID }

func (c *Conversation) Checkpoint() string { return c.checkpoint }

func (c *Conversation) Busy() bool { return c.Phase() != Idle }

// CurrentRun returns the root Run whose lifecycle the conversation owns.
// Descendant activity never replaces this root identity.
func (c *Conversation) CurrentRun() (Run, bool) {
	if c == nil || c.runID == "" {
		return Run{}, false
	}
	run, exists := c.runs[c.runID]
	return run.Clone(), exists
}

// RunningDescendants reports how much delegated work is live beneath the
// current root run. The aggregate owns this derivation so presentation layers
// never infer lifecycle state from transcript blocks or copy the run-tree
// invariants maintained here.
func (c *Conversation) RunningDescendants() int {
	if c.runID == "" {
		return 0
	}
	running := 0
	for id, run := range c.runs {
		if id != c.runID && run.Lineage.RootRunID() == c.runID && run.Status == protocol.RunStatusRunning {
			running++
		}
	}
	return running
}

// Runs returns the session run catalog in creation order. The conversation
// retains ordering as part of the aggregate instead of exposing its internal
// identity map and asking consumers to reconstruct chronology.
func (c *Conversation) Runs() []Run {
	runs := make([]Run, 0, len(c.runOrder))
	for _, id := range c.runOrder {
		if run, exists := c.runs[id]; exists {
			runs = append(runs, run.Clone())
		}
	}
	return runs
}

// MatchesSnapshot reports whether a cold projection carries the same
// conversation state currently folded by this aggregate. Session metadata and
// historical run catalogs are deliberately outside this comparison.
func (c *Conversation) MatchesSnapshot(snapshot SessionSnapshot) bool {
	expected := New()
	if err := expected.RestoreSnapshot(snapshot); err != nil {
		return false
	}
	if len(c.blocks) != len(expected.blocks) {
		return false
	}
	for index, block := range c.blocks {
		if !block.Equal(expected.blocks[index]) {
			return false
		}
	}
	return equalPlans(c.plan, expected.plan) &&
		c.Usage().Equal(expected.Usage()) && equalInterrupts(c.interrupts, expected.interrupts) &&
		c.Outcome().Equal(expected.Outcome()) && c.Phase() == expected.Phase() && c.runID == expected.runID &&
		c.segmentID == expected.segmentID && slices.EqualFunc(c.Runs(), expected.Runs(), Run.Equal)
}

func (c *Conversation) Starting() error {
	if c.Busy() {
		return fmt.Errorf("%w: conversation is already busy", ErrInvalidTransition)
	}
	c.opening = openingPending
	c.runID = ""
	c.segmentID = ""
	c.checkpoint = ""
	c.seen = make(map[string]RunEvent)
	c.interrupts = nil
	c.recovery = recoveryNone
	return nil
}

func (c *Conversation) CancelStarting() error {
	if c.runID != "" || c.opening != openingPending {
		return fmt.Errorf("%w: conversation is not starting", ErrInvalidTransition)
	}
	c.opening = openingCanceled
	c.recovery = recoveryNone
	return nil
}

func (c *Conversation) ClearPresentation() {
	c.blocks = nil
	c.plan = nil
	if c.Phase() == Idle {
		c.runID = ""
		c.opening = openingNone
	}
	c.index = make(map[string]int)
	c.textStreams = make(map[string]StreamedText)
}

func (c *Conversation) put(block Block, completed bool) error {
	c.ensureStorage()
	key := BlockKey(block.RunID, block.ID)
	if at, ok := c.index[key]; ok {
		if !completed {
			return fmt.Errorf("%w: block %s started twice", ErrInvalidTransition, block.ID)
		}
		if c.blocks[at].Status != BlockStatusRunning {
			return fmt.Errorf("%w: block %s completed twice", ErrInvalidTransition, block.ID)
		}
		if err := validateBlockIdentity(c.blocks[at], block); err != nil {
			return err
		}
		c.blocks[at] = block.Clone()
		delete(c.textStreams, key)
		return nil
	}
	c.index[key] = len(c.blocks)
	c.blocks = append(c.blocks, block.Clone())
	if !completed && (block.Kind == BlockAssistant || block.Kind == BlockReasoning) {
		c.textStreams[key] = NewStreamedText(block.Text)
	}
	return nil
}

func validateBlockIdentity(started, completed Block) error {
	if started.Kind != completed.Kind {
		return fmt.Errorf("%w: block %s changed kind from %s to %s", ErrInvalidTransition, completed.ID, started.Kind, completed.Kind)
	}
	if started.Kind != BlockTool {
		return nil
	}
	if started.Tool.Kind != completed.Tool.Kind {
		return fmt.Errorf("%w: tool block %s changed kind from %s to %s", ErrInvalidTransition, completed.ID, started.Tool.Kind, completed.Tool.Kind)
	}
	if started.Tool.Name != completed.Tool.Name {
		return fmt.Errorf("%w: tool block %s changed name from %q to %q", ErrInvalidTransition, completed.ID, started.Tool.Name, completed.Tool.Name)
	}
	return nil
}

func (c *Conversation) ensureStorage() {
	if c.seen == nil {
		c.seen = make(map[string]RunEvent)
	}
	if c.index == nil {
		c.index = make(map[string]int)
	}
	if c.runs == nil {
		c.runs = make(map[string]Run)
	}
	if c.textStreams == nil {
		c.textStreams = make(map[string]StreamedText)
	}
}

func (c *Conversation) rememberRun(run Run) {
	if _, exists := c.runs[run.ID]; !exists {
		c.runOrder = append(c.runOrder, run.ID)
	}
	c.runs[run.ID] = run.Clone()
}

func (c *Conversation) rebuildBlockIndex() {
	c.index = make(map[string]int, len(c.blocks))
	c.textStreams = make(map[string]StreamedText)
	for i, block := range c.blocks {
		key := BlockKey(block.RunID, block.ID)
		c.index[key] = i
		if block.Status == BlockStatusRunning && (block.Kind == BlockAssistant || block.Kind == BlockReasoning) {
			c.textStreams[key] = NewStreamedText(block.Text)
		}
	}
}

func (c *Conversation) hasOpenBlocksForRun(runID string) bool {
	for _, block := range c.blocks {
		if block.Status == BlockStatusRunning && block.RunID == runID {
			return true
		}
	}
	return false
}

func (c *Conversation) requireRunRunning(runID, action string) error {
	run, exists := c.runs[runID]
	if !exists || run.Status != protocol.RunStatusRunning {
		return fmt.Errorf("%w: cannot %s without active run %s", ErrInvalidTransition, action, runID)
	}
	return nil
}

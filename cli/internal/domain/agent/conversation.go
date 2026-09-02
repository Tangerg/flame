package agent

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/protocol"
)

var (
	ErrUnknownBlock      = errors.New("unknown transcript block")
	ErrInvalidTransition = errors.New("invalid conversation transition")
)

type EventAcceptance struct{ Applied bool }

type ConversationPhase string

const (
	ConversationIdle    ConversationPhase = "idle"
	ConversationRunning ConversationPhase = "running"
	ConversationWaiting ConversationPhase = "waiting"
)

// Valid reports whether c belongs to the live conversation lifecycle.
func (c ConversationPhase) Valid() bool {
	return c == ConversationIdle || c == ConversationRunning || c == ConversationWaiting
}

// Conversation is the terminal-facing aggregate. Durable history is restored
// from Items; live state is folded from one exact segment stream at a time.
type Conversation struct {
	blocks       []Block
	plan         *protocol.Plan
	usage        Usage
	interactions []Interaction
	outcome      Outcome

	phase       ConversationPhase
	runID       string
	segmentID   string
	checkpoint  string
	seen        map[string]RunEvent
	runs        map[string]Run
	runOrder    []string
	index       map[string]int
	open        map[string]bool
	textStreams map[string]StreamedText
	reconciling bool
	coldTail    bool
	// restoredPlanRevision is the durable Plan watermark installed by the last
	// cold snapshot. Plan persistence and stream publication are separate facts:
	// an attached read can already contain a revision whose PlanChanged event is
	// published after unrelated preview or progress events. Keeping the watermark
	// on the Plan itself prevents those events from prematurely ending overlap
	// reconciliation while preserving strict ordering for later revisions.
	restoredPlanRevision uint64
}

func NewConversation() *Conversation {
	return &Conversation{
		phase:       ConversationIdle,
		seen:        make(map[string]RunEvent),
		runs:        make(map[string]Run),
		index:       make(map[string]int),
		open:        make(map[string]bool),
		textStreams: make(map[string]StreamedText),
	}
}

func (c *Conversation) Blocks() []Block      { return cloneBlocks(c.blocks) }
func (c *Conversation) Plan() *protocol.Plan { return clonePlan(c.plan) }
func (c *Conversation) PlanItems() []protocol.PlanStep {
	if c.plan == nil || c.plan.State == nil {
		return nil
	}
	return append([]protocol.PlanStep{}, c.plan.State.Steps...)
}
func (c *Conversation) Usage() Usage                { return c.usage.Clone() }
func (c *Conversation) Interactions() []Interaction { return CloneInteractions(c.interactions) }
func (c *Conversation) Outcome() Outcome            { return c.outcome.Clone() }
func (c *Conversation) Phase() ConversationPhase    { return c.phase }
func (c *Conversation) RunID() string               { return c.runID }
func (c *Conversation) SegmentID() string           { return c.segmentID }
func (c *Conversation) Checkpoint() string          { return c.checkpoint }
func (c *Conversation) Busy() bool                  { return c.phase != ConversationIdle }

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
	expected := NewConversation()
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
		c.usage.Equal(expected.usage) && equalInteractions(c.interactions, expected.interactions) &&
		c.outcome.Equal(expected.outcome) && c.phase == expected.phase && c.runID == expected.runID &&
		c.segmentID == expected.segmentID && equalRunCatalogs(c.Runs(), expected.Runs())
}

func equalRunCatalogs(left, right []Run) bool {
	if len(left) != len(right) {
		return false
	}
	for index, run := range left {
		if !run.Equal(right[index]) {
			return false
		}
	}
	return true
}

func (c *Conversation) Starting() error {
	if c.Busy() {
		return fmt.Errorf("%w: conversation is already busy", ErrInvalidTransition)
	}
	c.phase = ConversationRunning
	c.runID = ""
	c.segmentID = ""
	c.checkpoint = ""
	c.seen = make(map[string]RunEvent)
	c.usage = Usage{}
	c.outcome = Outcome{}
	c.interactions = nil
	c.reconciling = false
	c.coldTail = false
	return nil
}

func (c *Conversation) CancelStarting() error {
	if c.phase != ConversationRunning || c.runID != "" {
		return fmt.Errorf("%w: conversation is not starting", ErrInvalidTransition)
	}
	c.phase = ConversationIdle
	c.reconciling = false
	c.coldTail = false
	c.outcome = Outcome{Status: OutcomeCanceled}
	return nil
}

// SettleRun applies the authoritative result of an out-of-band control such as
// runs.cancel, whose response is durable even when no segment stream is open.
func (c *Conversation) SettleRun(run Run) error {
	if err := run.Validate(); err != nil {
		return err
	}
	if run.Status != protocol.RunStatusFinished {
		return errors.New("cannot settle conversation from an unfinished run")
	}
	if !run.Lineage.IsRoot() {
		return errors.New("cannot settle conversation from a child-run control result")
	}
	if c.runID != "" && c.runID != run.ID {
		return fmt.Errorf("%w: settled run %s does not match %s", ErrInvalidTransition, run.ID, c.runID)
	}
	if c.runID == run.ID {
		if err := validateUsageProgress(c.usage, run.Usage); err != nil {
			return fmt.Errorf("%w: settled run: %w", ErrInvalidTransition, err)
		}
	}
	toolStatus := ToolError
	if run.Outcome.Status == OutcomeCanceled {
		toolStatus = ToolCanceled
	}
	c.settleOpenBlocks(toolStatus)
	for memberID, member := range c.runs {
		if member.Lineage.RootRunID() != run.ID || member.Status == protocol.RunStatusFinished {
			continue
		}
		member.Status = protocol.RunStatusFinished
		member.ActiveSegmentID = ""
		member.Outcome = run.Outcome.Clone()
		c.runs[memberID] = member
	}
	c.rememberRun(run)
	c.runID = run.ID
	c.segmentID = ""
	c.phase = ConversationIdle
	c.interactions = nil
	c.reconciling = false
	c.coldTail = false
	c.outcome = run.Outcome.Clone()
	c.usage = run.Usage.Clone()
	return nil
}

func (c *Conversation) ClearPresentation() {
	c.blocks = nil
	c.plan = nil
	c.usage = Usage{}
	c.outcome = Outcome{}
	c.index = make(map[string]int)
	c.open = make(map[string]bool)
	c.textStreams = make(map[string]StreamedText)
}

func (c *Conversation) put(block Block, completed bool) error {
	c.ensureStorage()
	key := blockIdentity(block.RunID, block.ID)
	if at, ok := c.index[key]; ok {
		if !completed {
			return fmt.Errorf("%w: block %s started twice", ErrInvalidTransition, block.ID)
		}
		if !c.open[key] {
			return fmt.Errorf("%w: block %s completed twice", ErrInvalidTransition, block.ID)
		}
		if err := validateBlockIdentity(c.blocks[at], block); err != nil {
			return err
		}
		c.blocks[at] = block.Clone()
		c.open[key] = block.Status == BlockStatusRunning
		delete(c.textStreams, key)
		return nil
	}
	c.index[key] = len(c.blocks)
	c.blocks = append(c.blocks, block.Clone())
	c.open[key] = !completed
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
	if c.open == nil {
		c.open = make(map[string]bool)
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
	c.open = make(map[string]bool, len(c.blocks))
	c.textStreams = make(map[string]StreamedText)
	for i, block := range c.blocks {
		key := blockIdentity(block.RunID, block.ID)
		c.index[key] = i
		c.open[key] = block.Status == BlockStatusRunning
		if block.Status == BlockStatusRunning && (block.Kind == BlockAssistant || block.Kind == BlockReasoning) {
			c.textStreams[key] = NewStreamedText(block.Text)
		}
	}
}

func (c *Conversation) hasOpenBlocksForRun(runID string) bool {
	for key, open := range c.open {
		if open && c.blocks[c.index[key]].RunID == runID {
			return true
		}
	}
	return false
}

func (c *Conversation) settleOpenBlocks(toolStatus ToolStatus) {
	for key, open := range c.open {
		if !open {
			continue
		}
		block := &c.blocks[c.index[key]]
		if block.Kind == BlockTool && block.Tool != nil {
			block.Tool.Status = toolStatus
		}
		block.Status = BlockStatusIncomplete
		c.open[key] = false
		delete(c.textStreams, key)
	}
}

func (c *Conversation) settleOpenBlocksForRun(runID string, toolStatus ToolStatus) {
	for key, open := range c.open {
		if !open {
			continue
		}
		block := &c.blocks[c.index[key]]
		if block.RunID != runID {
			continue
		}
		if block.Kind == BlockTool && block.Tool != nil {
			block.Tool.Status = toolStatus
		}
		block.Status = BlockStatusIncomplete
		c.open[key] = false
		delete(c.textStreams, key)
	}
}

func blockIdentity(runID, blockID string) string {
	return (BlockIdentity{RunID: runID, BlockID: blockID}).Key()
}

func (c *Conversation) requireRunRunning(runID, action string) error {
	run, exists := c.runs[runID]
	if !exists || run.Status != protocol.RunStatusRunning {
		return fmt.Errorf("%w: cannot %s without active run %s", ErrInvalidTransition, action, runID)
	}
	return nil
}

package conversation

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/protocol"
)

// ApplyRunEvent validates and folds one event exactly once. It never assigns
// ordering meaning to EventID; the stream order is the order of delivery.
func (c *Conversation) ApplyRunEvent(envelope RunEvent) (EventAcceptance, error) {
	if err := envelope.Validate(); err != nil {
		return EventAcceptance{}, fmt.Errorf("conversation: %w", err)
	}
	streamSegmentID := envelope.StreamSegment()
	newStreamSegment := c.segmentID != streamSegmentID
	if newStreamSegment {
		if _, ok := envelope.Event.(SegmentStarted); !ok {
			return EventAcceptance{}, fmt.Errorf("conversation: event stream segment %s does not match active stream segment %s", streamSegmentID, c.segmentID)
		}
	} else if known, duplicate := c.seen[envelope.EventID]; duplicate {
		if !known.Equal(envelope) {
			return EventAcceptance{}, fmt.Errorf("%w: event %s changed on replay", ErrEventConflict, envelope.EventID)
		}
		return EventAcceptance{}, nil
	}
	if err := c.validateEventIdentity(envelope); err != nil {
		return EventAcceptance{}, err
	}
	ignored, err := c.ignoreRecoveredOverlap(envelope)
	if err != nil {
		return EventAcceptance{}, err
	}
	if !ignored {
		err = c.apply(envelope)
		if err == nil && c.recovery == recoveryOverlap {
			c.recovery = recoveryColdTail
		}
	}
	if err != nil {
		return EventAcceptance{}, err
	}
	if newStreamSegment {
		c.seen = make(map[string]RunEvent)
		c.checkpoint = ""
	}
	c.segmentID = streamSegmentID
	c.seen[envelope.EventID] = envelope.Clone()
	if ReplayableEvent(envelope.Event) {
		c.checkpoint = envelope.EventID
	}
	return EventAcceptance{Applied: !ignored}, nil
}

func (c *Conversation) ignoreRecoveredOverlap(envelope RunEvent) (bool, error) {
	event := envelope.Event
	if item, ok := event.(PlanChanged); ok {
		state, stateErr := committedPlanState(&item.Plan)
		if stateErr != nil {
			return false, stateErr
		}
		if c.plan != nil && c.plan.State != nil && state.Revision == c.plan.State.Revision {
			if !equalPlans(c.plan, &item.Plan) {
				return false, fmt.Errorf("%w: plan revision %d changed content", ErrEventConflict, state.Revision)
			}
			// The Runtime deliberately repeats the segment's final Plan immediately
			// before segment.finished. Revision + content are the business identity;
			// the fence has a distinct transport EventID but folds to the same value.
			return true, nil
		}
		if state.Revision <= c.restoredPlanRevision {
			return true, nil
		}
	}
	if completed, ok := event.(BlockCompleted); ok && completed.Block.Kind == BlockQuestion {
		// A resumed continuation re-completes each answered Question. A projection
		// that already holds the answered Item, from a cold read, has nothing to fold.
		if at, exists := c.index[BlockKey(completed.Block.RunID, completed.Block.ID)]; exists &&
			c.blocks[at].Status != BlockStatusRunning && c.blocks[at].Equal(completed.Block) {
			return true, nil
		}
	}
	if delta, ok := event.(BlockDelta); ok {
		key := BlockKey(envelope.RunID, delta.BlockID)
		if _, exists := c.index[key]; !exists && c.recovery != recoveryNone {
			// Agent-message and reasoning starts are non-durable previews. A
			// head attachment can therefore observe their later deltas without
			// either a replayable start or a cold Item. Their completed Item is
			// authoritative and will restore the missing presentation block.
			return true, nil
		}
	}
	if c.recovery != recoveryOverlap {
		return false, nil
	}
	switch item := event.(type) {
	case BlockStarted:
		at, exists := c.index[BlockKey(item.Block.RunID, item.Block.ID)]
		if !exists {
			return false, nil
		}
		if err := validateBlockIdentity(c.blocks[at], item.Block); err != nil {
			return false, fmt.Errorf("%w: replayed start conflicts with the cold snapshot: %w", ErrEventConflict, err)
		}
		return true, nil
	case BlockDelta:
		key := BlockKey(envelope.RunID, item.BlockID)
		at, exists := c.index[key]
		return !exists || c.blocks[at].Status != BlockStatusRunning, nil
	case BlockCompleted:
		key := BlockKey(item.Block.RunID, item.Block.ID)
		at, exists := c.index[key]
		if !exists || c.blocks[at].Status == BlockStatusRunning || answersQuestion(c.blocks[at], item.Block) {
			return false, nil
		}
		if !c.blocks[at].Equal(item.Block) {
			return false, fmt.Errorf("%w: completed block %s differs from the cold snapshot", ErrEventConflict, item.Block.ID)
		}
		return true, nil
	default:
		return false, nil
	}
}

func (c *Conversation) validateEventIdentity(envelope RunEvent) error {
	if started, ok := envelope.Event.(SegmentStarted); ok {
		if !started.Run.Lineage.IsRoot() && started.Run.Lineage.RootRunID() != c.runID {
			return fmt.Errorf("conversation: child run %s belongs to root %s, not %s", envelope.RunID, started.Run.Lineage.RootRunID(), c.runID)
		}
		if c.Phase() == Waiting && started.Run.Lineage.IsRoot() && c.runID != envelope.RunID {
			return fmt.Errorf("conversation: resumed root run %s does not match waiting run %s", envelope.RunID, c.runID)
		}
		return nil
	}
	run, exists := c.runs[envelope.RunID]
	if !exists {
		return fmt.Errorf("conversation: event references unknown run %s", envelope.RunID)
	}
	if run.Status != protocol.RunStatusRunning || run.ActiveSegmentID != envelope.SegmentID {
		return fmt.Errorf("conversation: event segment %s does not match active run %s segment %s", envelope.SegmentID, envelope.RunID, run.ActiveSegmentID)
	}
	return nil
}

func (c *Conversation) apply(envelope RunEvent) error {
	c.ensureStorage()
	var err error
	switch item := envelope.Event.(type) {
	case SegmentStarted:
		err = c.applySegmentStarted(item)
	case BlockStarted:
		err = c.applyBlockStarted(envelope.RunID, item)
	case BlockDelta:
		err = c.applyBlockDelta(envelope.RunID, item)
	case ToolArgumentsDelta:
		err = c.applyToolArgumentsDelta(envelope.RunID, item)
	case RunProgress:
		err = c.applyRunProgress(envelope.RunID, item)
	case CustomEvent:
		err = c.requireRunRunning(envelope.RunID, "publish a custom event")
	case BlockCompleted:
		err = c.applyBlockCompleted(envelope.RunID, item)
	case PlanChanged:
		err = c.applyPlanChanged(envelope.RunID, item)
	case SegmentFinished:
		err = c.applySegmentFinished(envelope.RunID, item)
	default:
		err = fmt.Errorf("conversation: event %T is unsupported", envelope.Event)
	}
	if err != nil {
		return err
	}
	return nil
}

func (c *Conversation) applySegmentStarted(event SegmentStarted) error {
	c.recovery = recoveryNone
	run := event.Run
	previous, exists := c.runs[run.ID]
	if run.Lineage.IsRoot() {
		if err := c.applyRootSegmentStarted(run, previous, exists); err != nil {
			return err
		}
	} else if err := c.applyChildSegmentStarted(run, previous, exists); err != nil {
		return err
	}
	c.rememberRun(run)
	return nil
}

func (c *Conversation) applyRootSegmentStarted(run, previous Run, exists bool) error {
	previousUsage := c.Usage()
	switch c.Phase() {
	case Idle:
		previousUsage = Usage{}
	case Waiting:
		if c.runID != run.ID {
			return fmt.Errorf("%w: cannot resume %s while %s is waiting", ErrInvalidTransition, run.ID, c.runID)
		}
	case Running:
		if c.runID != "" && (!exists || previous.Status == protocol.RunStatusRunning) {
			return fmt.Errorf("%w: cannot start root segment while %s is running", ErrInvalidTransition, c.runID)
		}
	}
	if c.runID != "" && c.runID != run.ID {
		return fmt.Errorf("%w: root run changed from %s to %s", ErrInvalidTransition, c.runID, run.ID)
	}
	if err := validateUsageProgress(previousUsage, run.Usage); err != nil {
		return fmt.Errorf("%w: root segment started: %w", ErrInvalidTransition, err)
	}
	c.runID = run.ID
	c.opening = openingNone
	c.interrupts = nil
	return nil
}

func (c *Conversation) applyChildSegmentStarted(run, previous Run, exists bool) error {
	if c.runID == "" || run.Lineage.RootRunID() != c.runID {
		return fmt.Errorf("%w: child run %s has no active root", ErrInvalidTransition, run.ID)
	}
	if _, parentExists := c.runs[run.Lineage.ParentRunID()]; !parentExists {
		return fmt.Errorf("%w: child run %s has unknown parent %s", ErrInvalidTransition, run.ID, run.Lineage.ParentRunID())
	}
	if exists && previous.Lineage != run.Lineage {
		return fmt.Errorf("%w: child run %s changed lineage", ErrInvalidTransition, run.ID)
	}
	if exists && previous.Status == protocol.RunStatusRunning {
		return fmt.Errorf("%w: child run %s started twice", ErrInvalidTransition, run.ID)
	}
	if exists {
		if err := validateUsageProgress(previous.Usage, run.Usage); err != nil {
			return fmt.Errorf("%w: child segment started: %w", ErrInvalidTransition, err)
		}
	}
	return nil
}

func (c *Conversation) applyBlockStarted(runID string, event BlockStarted) error {
	if err := c.requireRunRunning(runID, "start a block"); err != nil {
		return err
	}
	return c.put(event.Block, false)
}

func (c *Conversation) applyBlockDelta(runID string, event BlockDelta) error {
	if err := c.requireRunRunning(runID, "append a block delta"); err != nil {
		return err
	}
	key := BlockKey(runID, event.BlockID)
	at, ok := c.index[key]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownBlock, event.BlockID)
	}
	if c.blocks[at].Status != BlockStatusRunning {
		return fmt.Errorf("%w: block %s is already complete", ErrInvalidTransition, event.BlockID)
	}
	block := &c.blocks[at]
	switch block.Kind {
	case BlockAssistant, BlockReasoning:
		stream := c.textStreams[key]
		if err := stream.Apply(event); err != nil {
			return fmt.Errorf("%w: block %s: %w", ErrInvalidTransition, event.BlockID, err)
		}
		c.textStreams[key] = stream
		block.Text = stream.String()
	case BlockTool:
		block.Tool.Output += event.Text
	default:
		return fmt.Errorf("%w: block %s of kind %s cannot stream", ErrInvalidTransition, event.BlockID, block.Kind)
	}
	return nil
}

func (c *Conversation) applyToolArgumentsDelta(runID string, event ToolArgumentsDelta) error {
	if err := c.requireRunRunning(runID, "append tool arguments"); err != nil {
		return err
	}
	key := BlockKey(runID, event.BlockID)
	at, exists := c.index[key]
	if !exists {
		return fmt.Errorf("%w: %s", ErrUnknownBlock, event.BlockID)
	}
	block := c.blocks[at]
	if block.Status != BlockStatusRunning || block.Kind != BlockTool {
		return fmt.Errorf("%w: block %s cannot stream tool arguments", ErrInvalidTransition, event.BlockID)
	}
	return nil
}

func (c *Conversation) applyRunProgress(runID string, event RunProgress) error {
	if err := c.requireRunRunning(runID, "report progress"); err != nil {
		return err
	}
	run := c.runs[runID]
	if event.ContextTokens != nil {
		run.ContextTokens = *event.ContextTokens
	}
	usage := run.Usage.Clone()
	if event.Usage != nil {
		usage = event.Usage.Clone()
		usage.Steps, usage.Duration = run.Usage.Steps, run.Usage.Duration
	}
	if event.Step != nil {
		usage.Steps = *event.Step
	}
	if err := validateUsageProgress(run.Usage, usage); err != nil {
		return fmt.Errorf("%w: run progress: %w", ErrInvalidTransition, err)
	}
	run.Usage = usage
	c.runs[runID] = run
	return nil
}

func (c *Conversation) applyBlockCompleted(runID string, event BlockCompleted) error {
	if err := c.requireRunRunning(runID, "complete a block"); err != nil {
		return err
	}
	if at, exists := c.index[BlockKey(event.Block.RunID, event.Block.ID)]; exists && answersQuestion(c.blocks[at], event.Block) {
		c.blocks[at] = event.Block.Clone()
		return nil
	}
	return c.put(event.Block, true)
}

// answersQuestion reports whether completed is the answered form of a Question
// that parked unanswered: the only Item a resumed Run completes a second time.
func answersQuestion(current, completed Block) bool {
	if current.Kind != BlockQuestion || completed.Kind != BlockQuestion ||
		current.Status == BlockStatusRunning || current.Question == nil || completed.Question == nil {
		return false
	}
	if current.Question.Answered() || !completed.Question.Answered() {
		return false
	}
	unanswered := completed.Question.Clone()
	unanswered.Answers = nil
	return current.Question.Equal(unanswered)
}

func (c *Conversation) applyPlanChanged(runID string, event PlanChanged) error {
	if runID != c.runID {
		return fmt.Errorf("%w: child run %s cannot change session plan", ErrInvalidTransition, runID)
	}
	if err := c.requireRunRunning(runID, "change the plan"); err != nil {
		return err
	}
	if event.Plan.SessionID != c.runs[runID].SessionID {
		return fmt.Errorf(
			"%w: plan belongs to session %q, want %q",
			ErrInvalidTransition,
			event.Plan.SessionID,
			c.runs[runID].SessionID,
		)
	}
	state, err := committedPlanState(&event.Plan)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidTransition, err)
	}
	if c.plan != nil && c.plan.State != nil && state.Revision <= c.plan.State.Revision {
		return fmt.Errorf("%w: plan revision %d does not advance %d", ErrInvalidTransition, state.Revision, c.plan.State.Revision)
	}
	c.plan = ClonePlan(&event.Plan)
	return nil
}

// applySegmentFinished replaces the Run with the record Runtime committed at
// the boundary. The fold checks only what the CLI composes across events: the
// interrupt set assembled for the tree, and the blocks and members still open.
func (c *Conversation) applySegmentFinished(runID string, event SegmentFinished) error {
	if err := c.requireRunRunning(runID, "finish a segment"); err != nil {
		return err
	}
	previous := c.runs[runID]
	if event.Run.ID != runID {
		return fmt.Errorf("%w: segment of run %s finished run %s", ErrInvalidTransition, runID, event.Run.ID)
	}
	if event.Run.Lineage != previous.Lineage {
		return fmt.Errorf("%w: run %s changed lineage", ErrInvalidTransition, runID)
	}
	if err := validateUsageProgress(previous.Usage, event.Run.Usage); err != nil {
		return fmt.Errorf("%w: segment finished: %w", ErrInvalidTransition, err)
	}
	var err error
	if event.Run.Status == protocol.RunStatusFinished {
		err = c.validateRunFinished(runID)
	} else {
		err = c.parkRun(runID, event.Interrupts)
	}
	if err != nil {
		return err
	}
	c.rememberRun(event.Run)
	if runID == c.runID {
		c.recovery = recoveryNone
		if event.Run.Status == protocol.RunStatusFinished {
			c.interrupts = nil
		}
	}
	return nil
}

func (c *Conversation) parkRun(runID string, interrupts []Interrupt) error {
	if len(interrupts) == 0 {
		if runID == c.runID {
			if err := ValidateInterrupts(c.interrupts); err != nil {
				return fmt.Errorf("%w: root run suspended without a valid tree interrupt: %w", ErrInvalidTransition, err)
			}
		}
		return nil
	}
	for _, interrupt := range interrupts {
		itemID := InterruptItemID(interrupt)
		at, exists := c.index[BlockKey(runID, itemID)]
		if !exists {
			return fmt.Errorf("%w: interrupt references unknown item %s", ErrInvalidTransition, itemID)
		}
		if err := validateInterruptItem(interrupt, c.blocks[at]); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidTransition, err)
		}
	}
	pending := append(CloneInterrupts(c.interrupts), CloneInterrupts(interrupts)...)
	if err := ValidateInterrupts(pending); err != nil {
		return fmt.Errorf("%w: tree interrupt set: %w", ErrInvalidTransition, err)
	}
	c.interrupts = pending
	c.recovery = recoveryNone
	return nil
}

func (c *Conversation) validateRunFinished(runID string) error {
	if c.hasOpenBlocksForRun(runID) {
		return fmt.Errorf("%w: run %s finished with open blocks", ErrInvalidTransition, runID)
	}
	if runID != c.runID {
		return nil
	}
	for memberID, member := range c.runs {
		if memberID != runID && member.Lineage.RootRunID() == runID && member.Status != protocol.RunStatusFinished {
			return fmt.Errorf("%w: root run finished while child %s is %s", ErrInvalidTransition, memberID, member.Status)
		}
	}
	return nil
}

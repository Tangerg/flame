package agent

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
		if err == nil {
			c.reconciling = false
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
	if delta, ok := event.(BlockDelta); ok {
		key := blockIdentity(envelope.RunID, delta.BlockID)
		if _, exists := c.index[key]; !exists && c.coldTail {
			// Agent-message and reasoning starts are non-durable previews. A
			// head attachment can therefore observe their later deltas without
			// either a replayable start or a cold Item. Their completed Item is
			// authoritative and will restore the missing presentation block.
			return true, nil
		}
	}
	if !c.reconciling {
		return false, nil
	}
	switch item := event.(type) {
	case BlockStarted:
		at, exists := c.index[blockIdentity(item.Block.RunID, item.Block.ID)]
		if !exists {
			return false, nil
		}
		if err := validateBlockIdentity(c.blocks[at], item.Block); err != nil {
			return false, fmt.Errorf("%w: replayed start conflicts with the cold snapshot: %w", ErrEventConflict, err)
		}
		return true, nil
	case BlockDelta:
		key := blockIdentity(envelope.RunID, item.BlockID)
		_, exists := c.index[key]
		return !exists || !c.open[key], nil
	case BlockCompleted:
		key := blockIdentity(item.Block.RunID, item.Block.ID)
		at, exists := c.index[key]
		if !exists || c.open[key] {
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
		if c.phase == ConversationWaiting && started.Run.Lineage.IsRoot() && c.runID != envelope.RunID {
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
	case RunInterrupted:
		err = c.applyInterrupted(envelope.RunID, item)
	case RunSuspended:
		err = c.applySuspended(envelope.RunID, item)
	case RunFinished:
		err = c.applyFinished(envelope.RunID, item)
	default:
		err = fmt.Errorf("conversation: event %T is unsupported", envelope.Event)
	}
	if err != nil {
		return err
	}
	return nil
}

func (c *Conversation) applySegmentStarted(event SegmentStarted) error {
	c.reconciling = false
	c.coldTail = false
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
	previousUsage := c.usage
	switch c.phase {
	case ConversationIdle:
		c.outcome = Outcome{}
		previousUsage = Usage{}
	case ConversationWaiting:
		if c.runID != run.ID {
			return fmt.Errorf("%w: cannot resume %s while %s is waiting", ErrInvalidTransition, run.ID, c.runID)
		}
	case ConversationRunning:
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
	c.phase = ConversationRunning
	c.interactions = nil
	c.usage = run.Usage.Clone()
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
	key := blockIdentity(runID, event.BlockID)
	at, ok := c.index[key]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownBlock, event.BlockID)
	}
	if !c.open[key] {
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
	key := blockIdentity(runID, event.BlockID)
	at, exists := c.index[key]
	if !exists {
		return fmt.Errorf("%w: %s", ErrUnknownBlock, event.BlockID)
	}
	block := c.blocks[at]
	if !c.open[key] || block.Kind != BlockTool {
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
	if runID == c.runID {
		c.usage = usage.Clone()
	}
	return nil
}

func (c *Conversation) applyBlockCompleted(runID string, event BlockCompleted) error {
	if err := c.requireRunRunning(runID, "complete a block"); err != nil {
		return err
	}
	return c.put(event.Block, true)
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
	c.plan = clonePlan(&event.Plan)
	return nil
}

func (c *Conversation) applyInterrupted(runID string, event RunInterrupted) error {
	if err := c.requireRunRunning(runID, "interrupt a run"); err != nil {
		return err
	}
	for _, interaction := range event.Interactions {
		itemID := InteractionItemID(interaction)
		at, exists := c.index[blockIdentity(runID, itemID)]
		if !exists {
			return fmt.Errorf("%w: interrupt references unknown item %s", ErrInvalidTransition, itemID)
		}
		if err := validateInteractionItem(interaction, c.blocks[at]); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidTransition, err)
		}
	}
	run := c.runs[runID]
	if err := validateUsageProgress(run.Usage, event.Usage); err != nil {
		return fmt.Errorf("%w: run interrupted: %w", ErrInvalidTransition, err)
	}
	pending := append(CloneInteractions(c.interactions), CloneInteractions(event.Interactions)...)
	if err := ValidateInteractions(pending); err != nil {
		return fmt.Errorf("%w: tree interrupt set: %v", ErrInvalidTransition, err)
	}
	run.Status = protocol.RunStatusWaiting
	run.ActiveSegmentID = ""
	run.Usage = event.Usage.Clone()
	run.ContextTokens = event.ContextTokens
	c.runs[runID] = run
	if runID == c.runID {
		c.phase = ConversationWaiting
		c.usage = event.Usage.Clone()
	}
	c.reconciling = false
	c.coldTail = false
	c.interactions = pending
	return nil
}

func (c *Conversation) applySuspended(runID string, event RunSuspended) error {
	if err := c.requireRunRunning(runID, "suspend a run"); err != nil {
		return err
	}
	run := c.runs[runID]
	if err := validateUsageProgress(run.Usage, event.Usage); err != nil {
		return fmt.Errorf("%w: run suspended: %w", ErrInvalidTransition, err)
	}
	if runID == c.runID {
		if err := ValidateInteractions(c.interactions); err != nil {
			return fmt.Errorf("%w: root run suspended without a valid tree interrupt: %v", ErrInvalidTransition, err)
		}
	}
	run.Status = protocol.RunStatusWaiting
	run.ActiveSegmentID = ""
	run.Usage = event.Usage.Clone()
	run.ContextTokens = event.ContextTokens
	c.runs[runID] = run
	if runID == c.runID {
		c.phase = ConversationWaiting
		c.usage = event.Usage.Clone()
		c.reconciling = false
		c.coldTail = false
	}
	return nil
}

func (c *Conversation) applyFinished(runID string, event RunFinished) error {
	run, exists := c.runs[runID]
	if !exists {
		return fmt.Errorf("%w: cannot finish unknown run %s", ErrInvalidTransition, runID)
	}
	if run.Status == protocol.RunStatusWaiting && event.Outcome.Status != OutcomeCanceled {
		return fmt.Errorf("%w: a waiting run can only finish by cancellation", ErrInvalidTransition)
	}
	if event.Outcome.Status == OutcomeCompleted && c.hasOpenBlocksForRun(runID) {
		return fmt.Errorf("%w: completed run %s still has open blocks", ErrInvalidTransition, runID)
	}
	if err := validateUsageProgress(run.Usage, event.Usage); err != nil {
		return fmt.Errorf("%w: run finished: %w", ErrInvalidTransition, err)
	}
	if runID == c.runID {
		for memberID, member := range c.runs {
			if memberID != runID && member.Lineage.RootRunID() == runID && member.Status != protocol.RunStatusFinished {
				return fmt.Errorf("%w: root run finished while child %s is %s", ErrInvalidTransition, memberID, member.Status)
			}
		}
	}
	toolStatus := ToolError
	if event.Outcome.Status == OutcomeCanceled {
		toolStatus = ToolCanceled
	}
	c.settleOpenBlocksForRun(runID, toolStatus)
	run.Status = protocol.RunStatusFinished
	run.ActiveSegmentID = ""
	run.Outcome = event.Outcome.Clone()
	run.Usage = event.Usage.Clone()
	run.ContextTokens = event.ContextTokens
	c.runs[runID] = run
	if runID == c.runID {
		c.phase = ConversationIdle
		c.reconciling = false
		c.coldTail = false
		c.interactions = nil
		c.outcome = event.Outcome.Clone()
		c.usage = event.Usage.Clone()
	}
	return nil
}

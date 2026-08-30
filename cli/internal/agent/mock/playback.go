package mock

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/agent"
	"github.com/Tangerg/flame/cli/internal/failure"
)

func (r *Runtime) play(run *runState, steps []Step, interrupt bool) {
	if !r.playSteps(run, steps) || !interrupt {
		return
	}
	r.park(run)
}

func (r *Runtime) playSteps(run *runState, steps []Step) bool {
	for _, step := range steps {
		if err := r.pause(run, step.Delay); err != nil {
			if errors.Is(err, errCanceled) {
				r.finish(run, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCanceled}})
			}
			return false
		}
		switch {
		case step.Event != nil:
			if finished, done := step.Event.(agent.RunFinished); done {
				r.finish(run, finished)
				return false
			}
			if !r.emit(run, step.Event) {
				return false
			}
		case step.plan != nil:
			if !r.replacePlan(run, step.plan.content) {
				return false
			}
		default:
			panic("mock: script step has no action")
		}
	}
	return true
}

func (r *Runtime) park(run *runState) {
	r.mu.Lock()
	if run.status != agent.RunStatusRunning {
		r.mu.Unlock()
		return
	}
	interactionEvents, err := r.interruptItemEventsLocked(run)
	if err != nil {
		r.mu.Unlock()
		r.finish(run, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeFailed, Problem: &failure.Problem{Type: "mock_interrupt_failed", Detail: err.Error()}}})
		return
	}
	resolved, pending := r.resolveRememberedLocked(run, run.script.Interactions)
	approvalEvents := approvalCompletionEvents(run, resolved)
	revisionChanges := sessionEventRevisionChanges(len(interactionEvents) + len(approvalEvents))
	if len(resolved) != 0 {
		revisionChanges = revisionChanges.plus(sessionEventRevisionChange())
	}
	if len(pending) != 0 {
		revisionChanges = revisionChanges.plus(sessionEventRevisionChange())
		revisionChanges = revisionChanges.plus(sessionStatusRevisionChanges(r.sessions[run.sessionID], agent.SessionWaiting))
	}
	if err := r.sessions[run.sessionID].requireRevisionCapacity(revisionChanges); err != nil {
		r.failSegmentLocked(run, err)
		r.mu.Unlock()
		return
	}
	if err := r.emitAllLocked(run, interactionEvents); err != nil {
		r.failSegmentLocked(run, err)
		r.mu.Unlock()
		return
	}
	for _, answer := range resolved {
		run.answers[answer.ItemID] = agent.CloneAnswer(answer.Answer)
	}
	if len(resolved) != 0 {
		if err := r.emitAllLocked(run, approvalEvents); err != nil {
			r.failSegmentLocked(run, err)
			r.mu.Unlock()
			return
		}
		if err := r.emitLocked(run, agent.BlockCompleted{Block: agent.Block{
			ID: run.id + "_approval_rule", Kind: agent.BlockNotice,
			Text: "Applied remembered approval rules.",
		}}); err != nil {
			r.failSegmentLocked(run, err)
			r.mu.Unlock()
			return
		}
	}
	if len(pending) == 0 {
		answers, err := completeScriptAnswers(run, nil)
		var steps []Step
		r.mu.Unlock()
		if err == nil {
			steps, err = continueSafely(run.script, answers)
		}
		if err != nil {
			r.finish(run, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeFailed, Problem: &failure.Problem{Type: "mock_continuation_failed", Detail: err.Error()}}})
			return
		}
		r.mu.Lock()
		if run.status != agent.RunStatusRunning {
			r.mu.Unlock()
			return
		}
		r.mu.Unlock()
		r.play(run, steps, false)
		return
	}
	run.status = agent.RunStatusWaiting
	run.interactions = agent.CloneInteractions(pending)
	run.usage = run.script.InterruptUsage.Clone()
	if err := r.emitLocked(run, agent.RunInterrupted{Interactions: agent.CloneInteractions(run.interactions), Usage: run.usage}); err != nil {
		r.failSegmentLocked(run, err)
		r.mu.Unlock()
		return
	}
	run.active = ""
	if err := r.setSessionStatusLocked(r.sessions[run.sessionID], agent.SessionWaiting); err != nil {
		r.failSegmentLocked(run, err)
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
}

func (r *Runtime) interruptItemEventsLocked(run *runState) ([]agent.Event, error) {
	session := r.sessions[run.sessionID]
	events := make([]agent.Event, 0, len(run.script.Interactions))
	for _, interaction := range run.script.Interactions {
		itemID := agent.InteractionItemID(interaction)
		if block, exists := durableBlock(session, run.id, itemID); exists {
			switch interaction.(type) {
			case agent.Approval:
				if block.Kind != agent.BlockTool || block.Status != agent.BlockStatusRunning {
					return nil, fmt.Errorf("approval item %s is not a running tool", itemID)
				}
			case agent.Question:
				if block.Kind != agent.BlockQuestion || block.Status != agent.BlockStatusCompleted {
					return nil, fmt.Errorf("question item %s is not a completed question", itemID)
				}
			}
			continue
		}
		switch item := interaction.(type) {
		case agent.Approval:
			events = append(events, agent.BlockStarted{Block: agent.Block{
				ID: item.ItemID, Kind: agent.BlockTool, Tool: cloneTool(item.Tool),
			}})
		case agent.Question:
			question := item.Clone()
			events = append(events, agent.BlockCompleted{Block: agent.Block{
				ID: item.ItemID, Kind: agent.BlockQuestion, Question: &question,
			}})
		}
	}
	return events, nil
}

func approvalCompletionEvents(run *runState, answers []agent.InterruptAnswer) []agent.Event {
	events := make([]agent.Event, 0, len(answers))
	for _, response := range answers {
		approval := findApproval(run.script.Interactions, response.ItemID)
		answer, ok := response.Answer.(agent.ApprovalAnswer)
		if approval == nil || !ok {
			continue
		}
		tool := cloneTool(approval.Tool)
		tool.Status = agent.ToolOK
		if answer.ArgumentOverride != nil {
			tool.ArgumentsJSON = answer.ArgumentOverride.JSON()
		}
		if answer.Decision == agent.ApprovalDeny {
			tool.Status = agent.ToolError
			tool.Output = strings.TrimSpace(answer.Reason)
			if tool.Output == "" {
				tool.Output = "tool call denied by user"
			}
		}
		events = append(events, agent.BlockCompleted{Block: agent.Block{ID: approval.ItemID, Kind: agent.BlockTool, Tool: tool}})
	}
	return events
}

func durableBlock(session *sessionState, runID, itemID string) (agent.Block, bool) {
	for _, item := range session.items {
		if item.runID == runID && item.block.ID == itemID {
			return item.block.Clone(), true
		}
	}
	return agent.Block{}, false
}

func cloneTool(tool *agent.ToolCall) *agent.ToolCall {
	if tool == nil {
		return nil
	}
	cloned := tool.Clone()
	return &cloned
}

func (r *Runtime) pause(run *runState, delay time.Duration) error {
	if r.Instant || delay <= 0 {
		select {
		case <-run.cancel:
			return errCanceled
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-run.cancel:
		return errCanceled
	}
}

func (r *Runtime) emit(run *runState, event agent.Event) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.status != agent.RunStatusRunning {
		return false
	}
	if err := r.emitLocked(run, event); err != nil {
		r.failSegmentLocked(run, err)
		return false
	}
	return true
}

func (r *Runtime) replacePlan(run *runState, content agent.PlanContent) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.status != agent.RunStatusRunning {
		return false
	}
	session := r.sessions[run.sessionID]
	plan, err := agent.CommitNextPlan(session.plan, content)
	if err != nil {
		r.failSegmentLocked(run, fmt.Errorf("mock: commit scripted Plan: %w", err))
		return false
	}
	if err := r.emitLocked(run, agent.PlanChanged{Plan: plan}); err != nil {
		r.failSegmentLocked(run, err)
		return false
	}
	return true
}

func (r *Runtime) emitAllLocked(run *runState, events []agent.Event) error {
	for _, event := range events {
		if err := r.emitLocked(run, event); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) emitLocked(run *runState, event agent.Event) error {
	segment := run.segments[run.active]
	if segment == nil {
		return errors.New("mock: active run has no segment")
	}
	session := r.sessions[run.sessionID]
	meta := session.meta
	meta.UpdatedAt = r.now()
	var err error
	meta, err = nextSessionMeta(session.meta, meta)
	if err != nil {
		return err
	}
	switch item := event.(type) {
	case agent.BlockStarted:
		item.Block.RunID = run.id
		item.Block.Status = agent.BlockStatusRunning
		event = item
	case agent.BlockCompleted:
		item.Block.RunID = run.id
		if item.Block.Status != agent.BlockStatusIncomplete {
			item.Block.Status = completedBlockStatus(item.Block)
		}
		event = item
	}
	envelope := agent.RunEvent{
		EventID: r.identities.next(eventIdentity), RunID: run.id,
		SegmentID: segment.id, At: meta.UpdatedAt, Event: agent.CloneEvent(event),
	}
	segment.events = append(segment.events, envelope)
	session.meta = meta
	switch item := event.(type) {
	case agent.BlockStarted:
		if item.Block.Kind == agent.BlockTool {
			persistBlock(session, run.id, item.Block)
		}
	case agent.BlockCompleted:
		persistBlock(session, run.id, item.Block)
	case agent.PlanChanged:
		plan := item.Plan.Clone()
		session.plan = &plan
	case agent.RunInterrupted:
		r.closeSegmentLocked(segment)
	case agent.RunFinished:
		r.closeSegmentLocked(segment)
	}
	close(segment.changed)
	segment.changed = make(chan struct{})
	return nil
}

func persistBlock(session *sessionState, runID string, block agent.Block) {
	for i := range session.items {
		if session.items[i].runID == runID && session.items[i].block.ID == block.ID {
			session.items[i] = durableItem{runID: runID, block: block.Clone()}
			return
		}
	}
	session.items = append(session.items, durableItem{runID: runID, block: block.Clone()})
}

func (r *Runtime) closeSegmentLocked(segment *segmentState) {
	segment.closed = true
}

func (r *Runtime) failSegmentLocked(run *runState, err error) {
	if err == nil || run.active == "" {
		return
	}
	segment := run.segments[run.active]
	if segment == nil || segment.terminalErr != nil {
		return
	}
	segment.terminalErr = err
	segment.closed = true
	close(segment.changed)
	segment.changed = make(chan struct{})
}

func (r *Runtime) finish(run *runState, event agent.RunFinished) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.finishLocked(run, event); err != nil {
		r.failSegmentLocked(run, err)
	}
}

func (r *Runtime) finishLocked(run *runState, event agent.RunFinished) error {
	if run.status == agent.RunStatusFinished {
		return nil
	}
	session := r.sessions[run.sessionID]
	settlements := r.runningItemSettlementsLocked(run, event.Outcome)
	revisionChanges := sessionStatusRevisionChanges(session, agent.SessionIdle)
	if run.active != "" {
		if run.segments[run.active] == nil {
			return errors.New("mock: active run has no segment")
		}
		revisionChanges = revisionChanges.plus(sessionEventRevisionChanges(len(settlements) + 1))
	}
	if err := session.requireRevisionCapacity(revisionChanges); err != nil {
		return err
	}

	run.outcome = event.Outcome
	run.usage = event.Usage.Clone()
	if run.active != "" {
		for _, block := range settlements {
			if err := r.emitLocked(run, agent.BlockCompleted{Block: block}); err != nil {
				return err
			}
		}
		if err := r.emitLocked(run, event); err != nil {
			return err
		}
	} else {
		for _, block := range settlements {
			persistBlock(session, run.id, block)
		}
	}
	run.status = agent.RunStatusFinished
	run.active = ""
	run.interactions = nil
	if session.planAtRun == nil {
		session.planAtRun = make(map[string]*agent.Plan)
	}
	session.planAtRun[run.id] = cloneCommittedPlan(session.plan)
	session.active = ""
	return r.setSessionStatusLocked(session, agent.SessionIdle)
}

func (r *Runtime) runningItemSettlementsLocked(run *runState, outcome agent.Outcome) []agent.Block {
	session := r.sessions[run.sessionID]
	var unsettled []agent.Block
	for _, item := range session.items {
		if item.runID == run.id && item.block.Status == agent.BlockStatusRunning {
			block := item.block.Clone()
			block.Status = agent.BlockStatusIncomplete
			if block.Tool != nil {
				block.Tool.Status = agent.ToolError
				if outcome.Status == agent.OutcomeCanceled {
					block.Tool.Status = agent.ToolCanceled
				}
			}
			unsettled = append(unsettled, block)
		}
	}
	return unsettled
}

func (r *Runtime) setSessionStatusLocked(session *sessionState, status agent.SessionStatus) error {
	if session.meta.Status == status {
		return nil
	}
	candidate := session.meta
	candidate.Status = status
	candidate.UpdatedAt = r.now()
	return session.commitMeta(candidate)
}

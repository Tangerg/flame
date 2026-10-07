package runtimefixture

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
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
				r.finish(run, Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCanceled}})
			}
			return false
		}
		switch {
		case step.Finish != nil:
			r.finish(run, *step.Finish)
			return false
		case step.Event != nil:
			if !r.emit(run, step.Event) {
				return false
			}
		case step.plan != nil:
			if !r.replacePlan(run, step.plan.steps) {
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
	if run.status != protocol.RunStatusRunning {
		r.mu.Unlock()
		return
	}
	interruptEvents, err := r.interruptItemEventsLocked(run)
	if err != nil {
		r.mu.Unlock()
		r.finish(run, Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeFailed, Problem: &protocol.ProblemData{Type: protocol.ProblemInternalError, Detail: err.Error()}}})
		return
	}
	resolved, pending := r.resolveRememberedLocked(run, run.script.Interrupts)
	approvalEvents := approvalCompletionEvents(run, resolved)
	revisionChanges := sessionEventRevisionChanges(len(interruptEvents) + len(approvalEvents))
	if len(resolved) != 0 {
		revisionChanges = revisionChanges.plus(sessionEventRevisionChange())
	}
	if len(pending) != 0 {
		revisionChanges = revisionChanges.plus(sessionEventRevisionChange())
		revisionChanges = revisionChanges.plus(sessionStatusRevisionChanges(r.sessions[run.sessionID], protocol.SessionStatusWaiting))
	}
	if err := r.sessions[run.sessionID].requireRevisionCapacity(revisionChanges); err != nil {
		r.failSegmentLocked(run, err)
		r.mu.Unlock()
		return
	}
	if err := r.emitAllLocked(run, interruptEvents); err != nil {
		r.failSegmentLocked(run, err)
		r.mu.Unlock()
		return
	}
	for _, answer := range resolved {
		run.answers[answer.ItemID] = conversation.CloneAnswer(answer.Answer)
	}
	if len(resolved) != 0 {
		if err := r.emitAllLocked(run, approvalEvents); err != nil {
			r.failSegmentLocked(run, err)
			r.mu.Unlock()
			return
		}
		if err := r.emitLocked(run, conversation.BlockCompleted{Block: conversation.Block{
			ID: run.id + "_approval_rule", Kind: conversation.BlockNotice,
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
			r.finish(run, Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeFailed, Problem: &protocol.ProblemData{Type: protocol.ProblemInternalError, Detail: err.Error()}}})
			return
		}
		r.mu.Lock()
		if run.status != protocol.RunStatusRunning {
			r.mu.Unlock()
			return
		}
		r.mu.Unlock()
		r.play(run, steps, false)
		return
	}
	run.status = protocol.RunStatusWaiting
	run.interrupts = conversation.CloneInterrupts(pending)
	run.usage = run.script.InterruptUsage.Clone()
	if err := r.emitLocked(run, conversation.SegmentFinished{Run: boundaryRun(run), Interrupts: conversation.CloneInterrupts(run.interrupts)}); err != nil {
		r.failSegmentLocked(run, err)
		r.mu.Unlock()
		return
	}
	run.active = ""
	if err := r.setSessionStatusLocked(r.sessions[run.sessionID], protocol.SessionStatusWaiting); err != nil {
		r.failSegmentLocked(run, err)
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
}

func (r *Runtime) interruptItemEventsLocked(run *runState) ([]conversation.Event, error) {
	session := r.sessions[run.sessionID]
	events := make([]conversation.Event, 0, len(run.script.Interrupts))
	for _, interrupt := range run.script.Interrupts {
		itemID := conversation.InterruptItemID(interrupt)
		if block, exists := durableBlock(session, run.id, itemID); exists {
			switch interrupt.(type) {
			case conversation.Approval:
				if block.Kind != conversation.BlockTool || block.Status != conversation.BlockStatusRunning {
					return nil, fmt.Errorf("approval item %s is not a running tool", itemID)
				}
			case conversation.Question:
				if block.Kind != conversation.BlockQuestion || block.Status != conversation.BlockStatusCompleted {
					return nil, fmt.Errorf("question item %s is not a completed question", itemID)
				}
			}
			continue
		}
		switch item := interrupt.(type) {
		case conversation.Approval:
			events = append(events, conversation.BlockStarted{Block: conversation.Block{
				ID: item.ItemID, Kind: conversation.BlockTool, Tool: cloneTool(item.Tool),
			}})
		case conversation.Question:
			question := item.Clone()
			events = append(events, conversation.BlockCompleted{Block: conversation.Block{
				ID: item.ItemID, Kind: conversation.BlockQuestion, Question: &question,
			}})
		}
	}
	return events, nil
}

func approvalCompletionEvents(run *runState, answers []conversation.InterruptAnswer) []conversation.Event {
	events := make([]conversation.Event, 0, len(answers))
	for _, response := range answers {
		approval := findApproval(run.script.Interrupts, response.ItemID)
		answer, ok := response.Answer.(conversation.ApprovalAnswer)
		if approval == nil || !ok {
			continue
		}
		tool := cloneTool(approval.Tool)
		tool.Status = conversation.ToolOK
		if answer.ArgumentOverride != nil {
			tool.ArgumentsJSON = answer.ArgumentOverride.JSON()
		}
		if answer.Decision == protocol.ApprovalDeny {
			tool.Status = conversation.ToolError
			tool.Output = strings.TrimSpace(answer.Reason)
			if tool.Output == "" {
				tool.Output = "tool call denied by user"
			}
		}
		events = append(events, conversation.BlockCompleted{Block: conversation.Block{ID: approval.ItemID, Kind: conversation.BlockTool, Tool: tool}})
	}
	return events
}

func durableBlock(session *sessionState, runID, itemID string) (conversation.Block, bool) {
	for _, item := range session.items {
		if item.runID == runID && item.block.ID == itemID {
			return item.block.Clone(), true
		}
	}
	return conversation.Block{}, false
}

func cloneTool(tool *conversation.ToolCall) *conversation.ToolCall {
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

func (r *Runtime) emit(run *runState, event conversation.Event) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.status != protocol.RunStatusRunning {
		return false
	}
	if err := r.emitLocked(run, event); err != nil {
		r.failSegmentLocked(run, err)
		return false
	}
	return true
}

func (r *Runtime) replacePlan(run *runState, steps []protocol.PlanStep) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.status != protocol.RunStatusRunning {
		return false
	}
	session := r.sessions[run.sessionID]
	plan, err := commitNextPlan(session.plan, session.meta.ID, r.now(), steps)
	if err != nil {
		r.failSegmentLocked(run, fmt.Errorf("mock: commit scripted Plan: %w", err))
		return false
	}
	if err := r.emitLocked(run, conversation.PlanChanged{Plan: *plan}); err != nil {
		r.failSegmentLocked(run, err)
		return false
	}
	return true
}

func (r *Runtime) emitAllLocked(run *runState, events []conversation.Event) error {
	for _, event := range events {
		if err := r.emitLocked(run, event); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) emitLocked(run *runState, event conversation.Event) error {
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
	case conversation.BlockStarted:
		item.Block.RunID = run.id
		item.Block.Status = conversation.BlockStatusRunning
		event = item
	case conversation.BlockCompleted:
		item.Block.RunID = run.id
		if item.Block.Status != conversation.BlockStatusIncomplete {
			item.Block.Status = completedBlockStatus(item.Block)
		}
		event = item
	}
	envelope := conversation.RunEvent{
		EventID: r.identities.next(eventIdentity), RunID: run.id,
		SegmentID: segment.id, At: meta.UpdatedAt, Event: conversation.CloneEvent(event),
	}
	segment.events = append(segment.events, envelope)
	session.meta = meta
	switch item := event.(type) {
	case conversation.BlockStarted:
		if item.Block.Kind == conversation.BlockTool {
			persistBlock(session, run.id, item.Block)
		} else {
			run.streaming = append(run.streaming, item.Block.Clone())
		}
	case conversation.BlockDelta:
		for i := range run.streaming {
			if run.streaming[i].ID == item.BlockID {
				run.streaming[i].Text += item.Text
			}
		}
	case conversation.BlockCompleted:
		run.streaming = slices.DeleteFunc(run.streaming, func(open conversation.Block) bool { return open.ID == item.Block.ID })
		persistBlock(session, run.id, item.Block)
	case conversation.PlanChanged:
		session.plan = conversation.ClonePlan(&item.Plan)
	case conversation.RunProgress:
		if item.ContextTokens != nil {
			run.contextTokens = *item.ContextTokens
		}
		if item.Usage != nil {
			run.usage = item.Usage.Clone()
		}
	case conversation.SegmentFinished:
		r.closeSegmentLocked(segment)
	}
	close(segment.changed)
	segment.changed = make(chan struct{})
	return nil
}

func persistBlock(session *sessionState, runID string, block conversation.Block) {
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

func (r *Runtime) finish(run *runState, finish Finish) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.finishLocked(run, finish); err != nil {
		r.failSegmentLocked(run, err)
	}
}

func (r *Runtime) finishLocked(run *runState, finish Finish) error {
	if run.status == protocol.RunStatusFinished {
		return nil
	}
	session := r.sessions[run.sessionID]
	settlements := r.runningItemSettlementsLocked(run, finish.Outcome)
	revisionChanges := sessionStatusRevisionChanges(session, protocol.SessionStatusIdle)
	if run.active != "" {
		if run.segments[run.active] == nil {
			return errors.New("mock: active run has no segment")
		}
		revisionChanges = revisionChanges.plus(sessionEventRevisionChanges(len(settlements) + 1))
	}
	if err := session.requireRevisionCapacity(revisionChanges); err != nil {
		return err
	}

	run.outcome = finish.Outcome.Clone()
	run.usage = finish.Usage.Clone()
	if run.active != "" {
		for _, block := range settlements {
			if err := r.emitLocked(run, conversation.BlockCompleted{Block: block}); err != nil {
				return err
			}
		}
		finished := boundaryRun(run)
		finished.Status = protocol.RunStatusFinished
		if err := r.emitLocked(run, conversation.SegmentFinished{Run: finished}); err != nil {
			return err
		}
	} else {
		for _, block := range settlements {
			persistBlock(session, run.id, block)
		}
	}
	run.status = protocol.RunStatusFinished
	run.active = ""
	run.interrupts = nil
	run.streaming = nil
	if session.planAtRun == nil {
		session.planAtRun = make(map[string]*protocol.Plan)
	}
	session.planAtRun[run.id] = conversation.ClonePlan(session.plan)
	session.active = ""
	return r.setSessionStatusLocked(session, protocol.SessionStatusIdle)
}

func (r *Runtime) runningItemSettlementsLocked(run *runState, outcome conversation.Outcome) []conversation.Block {
	session := r.sessions[run.sessionID]
	var unsettled []conversation.Block
	for _, open := range run.streaming {
		block := open.Clone()
		block.Status = conversation.BlockStatusIncomplete
		unsettled = append(unsettled, block)
	}
	for _, item := range session.items {
		if item.runID == run.id && item.block.Status == conversation.BlockStatusRunning {
			block := item.block.Clone()
			block.Status = conversation.BlockStatusIncomplete
			if block.Tool != nil {
				block.Tool.Status = conversation.ToolError
				if outcome.Status == protocol.OutcomeCanceled {
					block.Tool.Status = conversation.ToolCanceled
				}
			}
			unsettled = append(unsettled, block)
		}
	}
	return unsettled
}

func (r *Runtime) setSessionStatusLocked(session *sessionState, status protocol.SessionStatus) error {
	if session.meta.Status == status {
		return nil
	}
	candidate := session.meta
	candidate.Status = status
	candidate.UpdatedAt = r.now()
	return session.commitMeta(candidate)
}

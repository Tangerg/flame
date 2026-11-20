package sessions

import (
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

// parkedRunTerminalization derives the atomic Application write-set that ends
// one waiting Run tree. It combines a coherent Session snapshot with the exact
// Pending hand-off, but owns neither persistence nor executor cleanup.
type parkedRunTerminalization struct {
	sessionID            identity.SessionID
	finishedAt           time.Time
	outcome              rundomain.Outcome
	detail               string
	pending              runs.Pending
	snapshot             Snapshot
	conversationMessages []corechat.Message
}

func (p parkedRunTerminalization) build() (TerminalPlan, error) {
	if err := p.pending.Validate(); err != nil {
		return TerminalPlan{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: invalid Pending: %w",
			p.pending.RootRunID,
			err,
		)
	}
	if p.outcome != rundomain.OutcomeCanceled &&
		p.outcome != rundomain.OutcomeLost {
		return TerminalPlan{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: unsupported outcome %s",
			p.pending.RootRunID,
			p.outcome,
		)
	}
	runsByID, err := p.indexRuns()
	if err != nil {
		return TerminalPlan{}, err
	}
	conversationMessages := p.conversationMessages
	terminalRuns, err := p.terminalRuns(runsByID, len(p.snapshot.Messages)+len(conversationMessages))
	if err != nil {
		return TerminalPlan{}, err
	}
	items, err := p.terminalItems()
	if err != nil {
		return TerminalPlan{}, err
	}
	root, ok := p.pending.RootContinuation()
	if !ok {
		return TerminalPlan{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: root continuation is missing",
			p.pending.RootRunID,
		)
	}
	plan, err := NewTerminalPlan(terminalRuns, items, conversationMessages, root.MemberID)
	if err != nil {
		return TerminalPlan{}, err
	}
	rootRun, _ := plan.RootRun()
	if rootRun.ID() != p.pending.RootRunID {
		return TerminalPlan{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: root admission differs from Pending",
			p.pending.RootRunID,
		)
	}
	return plan, nil
}

func (p parkedRunTerminalization) indexRuns() (map[identity.RunID]rundomain.Run, error) {
	runsByID := make(map[identity.RunID]rundomain.Run, len(p.snapshot.Runs))
	for _, run := range p.snapshot.Runs {
		if _, duplicate := runsByID[run.ID()]; duplicate {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: duplicate Run %q",
				p.pending.RootRunID,
				run.ID(),
			)
		}
		runsByID[run.ID()] = run
	}
	root, found := runsByID[p.pending.RootRunID]
	if !found || !root.Lineage().IsRoot() {
		return nil, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: root Run is missing",
			p.pending.RootRunID,
		)
	}
	pendingRunIDs := make(map[identity.RunID]struct{}, len(p.pending.Continuations))
	for _, continuation := range p.pending.Continuations {
		pendingRunIDs[continuation.RunID] = struct{}{}
	}
	for _, run := range p.snapshot.Runs {
		if run.Lineage().TreeRootID(run.ID()) != p.pending.RootRunID || run.State().IsTerminal() {
			continue
		}
		if _, covered := pendingRunIDs[run.ID()]; !covered {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: non-terminal Run %q has no Pending continuation",
				p.pending.RootRunID,
				run.ID(),
			)
		}
	}
	return runsByID, nil
}

func (p parkedRunTerminalization) terminalRuns(
	runsByID map[identity.RunID]rundomain.Run,
	messageMark int,
) ([]rundomain.Replacement, error) {
	terminalRuns := make([]rundomain.Replacement, 0, len(p.pending.Continuations))
	for _, continuation := range p.pending.Continuations {
		run, found := runsByID[continuation.RunID]
		if !found {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: Run %q is missing",
				p.pending.RootRunID,
				continuation.RunID,
			)
		}
		if run.SessionID() != p.sessionID || run.State() != rundomain.Waiting {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: Run %q differs from its continuation",
				p.pending.RootRunID,
				run.ID(),
			)
		}
		var replacement rundomain.Replacement
		var err error
		mark := run.Lineage().MessageMark(messageMark)
		if p.outcome == rundomain.OutcomeLost {
			replacement, err = run.Replace().RecoverLost(rundomain.Loss{Reason: rundomain.LossWaitingStateUnavailable}, p.finishedAt, mark)
		} else {
			replacement, err = run.Replace().CancelWaiting(p.detail, p.finishedAt, mark)
		}
		if err != nil {
			return nil, fmt.Errorf("sessions: terminalize parked Run %q: %w", run.ID(), err)
		}
		terminalRuns = append(terminalRuns, replacement)
	}
	return terminalRuns, nil
}

func (p parkedRunTerminalization) terminalItems() ([]transcript.Replacement, error) {
	projection := newParkedTerminalItemProjection(p)
	for _, item := range p.snapshot.Items {
		if err := projection.project(item); err != nil {
			return nil, err
		}
	}
	if err := projection.validateComplete(); err != nil {
		return nil, err
	}
	return projection.items, nil
}

// parkedInterrupt is one open interrupt and the Run whose member it binds.
type parkedInterrupt struct {
	open  runs.OpenInterrupt
	runID identity.RunID
}

type parkedTerminalItemProjection struct {
	owner         parkedRunTerminalization
	interrupts    map[identity.ItemID]parkedInterrupt
	pendingRunIDs map[identity.RunID]struct{}
	drained       map[identity.ItemID]runs.DrainedTool
	items         []transcript.Replacement
}

func newParkedTerminalItemProjection(owner parkedRunTerminalization) *parkedTerminalItemProjection {
	runByMember := make(map[string]identity.RunID, len(owner.pending.Continuations))
	for _, continuation := range owner.pending.Continuations {
		runByMember[continuation.MemberID] = continuation.RunID
	}
	interrupts := make(map[identity.ItemID]parkedInterrupt, len(owner.pending.Interrupts))
	for index, interruption := range owner.pending.Interrupts {
		interrupts[interruption.ItemID] = parkedInterrupt{
			open: interruption, runID: runByMember[owner.pending.Bindings[index].MemberID],
		}
	}
	pendingRunIDs := make(map[identity.RunID]struct{}, len(owner.pending.Continuations))
	drained := make(map[identity.ItemID]runs.DrainedTool)
	for _, continuation := range owner.pending.Continuations {
		pendingRunIDs[continuation.RunID] = struct{}{}
		for _, tool := range continuation.DrainedTools {
			drained[tool.ItemID] = tool
		}
	}
	return &parkedTerminalItemProjection{
		owner:         owner,
		interrupts:    interrupts,
		pendingRunIDs: pendingRunIDs,
		drained:       drained,
		items:         make([]transcript.Replacement, 0, len(interrupts)+len(drained)),
	}
}

func (projection *parkedTerminalItemProjection) project(item transcript.Item) error {
	if request, found := projection.interrupts[item.ID()]; found {
		return projection.projectInterrupt(item, request)
	}
	if _, found := projection.drained[item.ID()]; found {
		return projection.projectDrained(item)
	}
	if _, pending := projection.pendingRunIDs[item.RunID()]; pending && item.Status() == transcript.ItemRunning {
		return fmt.Errorf(
			"sessions: terminalize parked Run tree %q: Running Item %q has no matching interrupt",
			projection.owner.pending.RootRunID,
			item.ID(),
		)
	}
	return nil
}

func (projection *parkedTerminalItemProjection) projectDrained(item transcript.Item) error {
	settled, err := projection.owner.terminalDrainedItem(item)
	if err != nil {
		return err
	}
	projection.items = append(projection.items, settled)
	delete(projection.drained, item.ID())
	return nil
}

func (projection *parkedTerminalItemProjection) projectInterrupt(
	item transcript.Item,
	request parkedInterrupt,
) error {
	owner := projection.owner
	if item.SessionID() != owner.sessionID || item.RunID() != request.runID {
		return fmt.Errorf(
			"sessions: terminalize parked Run tree %q: interrupt Item %q is not owned by Run %q",
			owner.pending.RootRunID,
			item.ID(),
			request.runID,
		)
	}
	switch request.open.Type() {
	case rundomain.InterruptQuestion:
		if item.Kind() != transcript.QuestionItem || item.Status() != transcript.ItemCompleted {
			return fmt.Errorf("sessions: parked question Item %q is not a complete prompt", item.ID())
		}
	case rundomain.InterruptApproval:
		settled, err := item.AbandonToolCall(owner.abandonmentFailure(), owner.finishedAt)
		if err != nil {
			return fmt.Errorf(
				"sessions: terminalize parked ToolCall %q: %w",
				item.ID(),
				err,
			)
		}
		projection.items = append(projection.items, settled)
	default:
		return fmt.Errorf("sessions: parked interrupt Item %q has unknown kind %s", item.ID(), request.open.Type())
	}
	delete(projection.interrupts, item.ID())
	return nil
}

func (projection *parkedTerminalItemProjection) validateComplete() error {
	if len(projection.interrupts) != 0 {
		return fmt.Errorf(
			"sessions: terminalize parked Run tree %q: transcript is missing an interrupt Item",
			projection.owner.pending.RootRunID,
		)
	}
	if len(projection.drained) != 0 {
		return fmt.Errorf(
			"sessions: terminalize parked Run tree %q: transcript is missing %d drained Tool Items",
			projection.owner.pending.RootRunID,
			len(projection.drained),
		)
	}
	return nil
}

func (p parkedRunTerminalization) terminalDrainedItem(item transcript.Item) (transcript.Replacement, error) {
	if _, present := item.ToolCall(); item.SessionID() != p.sessionID || !present {
		return transcript.Replacement{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: drained Tool Item %q is not a ToolCall of this Session",
			p.pending.RootRunID,
			item.ID(),
		)
	}
	switch item.Status() {
	case transcript.ItemRunning:
		settled, err := item.AbandonToolCall(p.abandonmentFailure(), p.finishedAt)
		if err != nil {
			return transcript.Replacement{}, fmt.Errorf(
				"sessions: terminalize parked ToolCall %q: %w",
				item.ID(),
				err,
			)
		}
		return settled, nil
	default:
		return transcript.Replacement{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: drained Tool Item %q is %s",
			p.pending.RootRunID,
			item.ID(),
			item.Status(),
		)
	}
}

func (p parkedRunTerminalization) abandonmentFailure() *tool.Failure {
	if p.outcome != rundomain.OutcomeLost {
		return nil
	}
	return &tool.Failure{
		Kind:   tool.FailureExecution,
		Detail: "tool call abandoned because its run could not be resumed",
	}
}

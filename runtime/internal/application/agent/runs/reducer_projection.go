package runs

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

// reduction is one canonical output plus the persisted fact and live nudge that
// arise from the same ExecutionFact decision. The pump commits it before placing
// Event on the journal.
type reduction struct {
	Event  ProjectionEvent
	Commit *EventCommit
	Nudge  *Nudge
}

// reductionBatch is the complete publication unit for one executor event. A
// normal batch commits individual event projections in order. A park batch owns
// one explicit write-set that must commit before any of its events become
// visible; keeping that boundary on the batch avoids encoding it as a boolean
// or a privileged first element in the event slice.
type reductionBatch struct {
	events             []reduction
	parkCommit         *EventCommit
	settledToolCallIDs []string
}

// factReduction is the complete in-memory consequence of one executor fact
// before Run events are projected into their durable publication shape.
type factReduction struct {
	events               []ProjectionEvent
	items                []transcript.Item
	parkItems            []transcript.Item
	conversationMessages []corechat.Message
	modelInvocations     []ModelInvocationCommit
	toolInvocations      []ToolInvocationCommit
	settledToolCallIDs   []string
	progress             *ProgressCommit
}

func (r *reducer) projectFact(reduced factReduction) (reductionBatch, error) {
	batch, err := r.project(reduced.events)
	if err != nil {
		return reductionBatch{}, err
	}
	batch.settledToolCallIDs = slices.Clone(reduced.settledToolCallIDs)
	if len(reduced.parkItems) != 0 {
		if batch.parkCommit == nil {
			return reductionBatch{}, fmt.Errorf("%w: parked Items have no park boundary", errReducerInvariant)
		}
		batch.parkCommit.Items = append(batch.parkCommit.Items, reduced.parkItems...)
		if err := validateReductionBatch(batch); err != nil {
			return reductionBatch{}, err
		}
	}
	if err := r.attachDurableItems(&batch, reduced.items); err != nil {
		return reductionBatch{}, err
	}
	if err := r.attachDurableObservation(
		&batch,
		reduced.conversationMessages,
		reduced.modelInvocations,
		reduced.toolInvocations,
		reduced.progress,
	); err != nil {
		return reductionBatch{}, err
	}
	return batch, nil
}

func (r *reducer) attachDurableObservation(
	batch *reductionBatch,
	conversationMessages []corechat.Message,
	modelInvocations []ModelInvocationCommit,
	toolInvocations []ToolInvocationCommit,
	progress *ProgressCommit,
) error {
	if len(conversationMessages) == 0 && len(modelInvocations) == 0 && len(toolInvocations) == 0 && progress == nil {
		return nil
	}
	if batch == nil || len(batch.events) == 0 {
		return fmt.Errorf("%w: durable observation has no ordinary reduction", errReducerInvariant)
	}
	var commit *EventCommit
	if batch.parkCommit != nil {
		commit = batch.parkCommit
	} else {
		last := &batch.events[len(batch.events)-1]
		if last.Commit == nil {
			last.Commit = &EventCommit{RunID: r.cfg.RunID, SessionID: r.cfg.SessionID, SegmentID: r.cfg.SegmentID}
		}
		commit = last.Commit
	}
	commit.ModelInvocations = append(commit.ModelInvocations, modelInvocations...)
	commit.ToolInvocations = append(commit.ToolInvocations, toolInvocations...)
	commit.ConversationMessages = appendClonedMessages(commit.ConversationMessages, conversationMessages...)
	if progress != nil {
		cloned := *progress
		commit.Progress = &cloned
	}
	return validateReductionBatch(*batch)
}

func (r *reducer) attachDurableItems(batch *reductionBatch, items []transcript.Item) error {
	if len(items) == 0 {
		return nil
	}
	if batch == nil || len(batch.events) == 0 || batch.parkCommit != nil {
		return fmt.Errorf("%w: durable Items have no ordinary reduction", errReducerInvariant)
	}
	last := &batch.events[len(batch.events)-1]
	if last.Commit == nil {
		last.Commit = &EventCommit{RunID: r.cfg.RunID, SessionID: r.cfg.SessionID, SegmentID: r.cfg.SegmentID}
	}
	last.Commit.Items = append(last.Commit.Items, items...)
	return validateReductionBatch(*batch)
}

func (r *reducer) attachConversationMessages(batch *reductionBatch, messages []corechat.Message) error {
	if len(messages) == 0 {
		return nil
	}
	if batch == nil || len(batch.events) == 0 || batch.parkCommit != nil {
		return fmt.Errorf("%w: conversation projection has no ordinary reduction", errReducerInvariant)
	}
	last := &batch.events[len(batch.events)-1]
	if last.Commit == nil {
		last.Commit = &EventCommit{RunID: r.cfg.RunID, SessionID: r.cfg.SessionID, SegmentID: r.cfg.SegmentID}
	}
	last.Commit.ConversationMessages = appendClonedMessages(last.Commit.ConversationMessages, messages...)
	return validateReductionBatch(*batch)
}

func (r *reducer) project(events []ProjectionEvent) (reductionBatch, error) {
	events = r.fenceFinalPlan(events)
	reductions := make([]reduction, 0, len(events))
	for _, event := range events {
		reduced, err := r.projectOne(event)
		if err != nil {
			return reductionBatch{}, err
		}
		reductions = append(reductions, reduced)
	}

	// A park is one persistence boundary: any drained/closed items, its running
	// approval/question items, open interrupt record, waiting transcript Run,
	// and admission transition must commit together before ANY event in this
	// batch is published. Build an explicit batch-owned write-set instead of
	// moving it onto a privileged event position.
	parkBoundary, err := parkBoundaryIndex(reductions)
	if err != nil {
		return reductionBatch{}, err
	}
	batch := reductionBatch{events: reductions}
	if parkBoundary >= 0 {
		batch, err = parkReductionBatch(reductions, parkBoundary)
		if err != nil {
			return reductionBatch{}, err
		}
	}
	if err := validateReductionBatch(batch); err != nil {
		return reductionBatch{}, err
	}
	return batch, nil
}

func parkBoundaryIndex(reductions []reduction) (int, error) {
	parkBoundary := -1
	for index := range reductions {
		commit := reductions[index].Commit
		if commit == nil || commit.State != StateSuspend {
			continue
		}
		if parkBoundary >= 0 {
			return -1, fmt.Errorf("%w: reduction batch has multiple park boundaries", errReducerInvariant)
		}
		parkBoundary = index
	}
	return parkBoundary, nil
}

func parkReductionBatch(reductions []reduction, parkBoundary int) (reductionBatch, error) {
	parkCommit := reductions[parkBoundary].Commit
	if parkCommit == nil {
		return reductionBatch{}, fmt.Errorf("%w: park boundary has no projection commit", errReducerInvariant)
	}
	for index, reduced := range reductions {
		if index != parkBoundary && reduced.Commit != nil {
			if reduced.Commit.Run != nil || reduced.Commit.State != StateUnchanged {
				return reductionBatch{}, fmt.Errorf("%w: park batch contains another lifecycle transition", errReducerInvariant)
			}
		}
		reductions[index].Commit = nil
	}
	return reductionBatch{events: reductions, parkCommit: parkCommit}, nil
}

// fenceFinalPlan republishes the segment's last Plan immediately before the
// segment finishes when that segment changed it.
//
// Without it, a client only holds the Plan if it received the change event itself.
// A subscriber that attached later — or replayed from a cursor past that event —
// reaches segment.finished having never seen a snapshot, and renders a stale panel
// until something makes it refetch. The fence makes the guarantee positional:
// whoever receives the finish has received the final value, because it is the
// replayable event immediately before it.
//
// The repeat is the point, not waste: a latest-value projection carries its own
// revision, so folding it twice is folding it once.
//
// It belongs to the batch rather than to either finish path: a park and a terminal
// are two reasons for one boundary, and a rule stated in both places is a rule that
// drifts in one of them.
func (r *reducer) fenceFinalPlan(events []ProjectionEvent) []ProjectionEvent {
	if r.plan == nil {
		return events
	}
	for i, event := range events {
		if _, finishing := event.(SegmentFinished); !finishing {
			continue
		}
		fence := r.plan.clone()
		// One fence per segment: a resumed segment fences again only if it changes
		// the projection again.
		r.plan = nil
		return slices.Insert(events, i, ProjectionEvent(fence))
	}
	return events
}

func (r *reducer) projectOne(event ProjectionEvent) (reduction, error) {
	if event == nil {
		return reduction{}, fmt.Errorf("%w: nil run event", errReducerInvariant)
	}
	if err := event.validate(); err != nil {
		return reduction{}, fmt.Errorf("%w: %T: %v", errReducerInvariant, event, err)
	}
	commit := EventCommit{RunID: r.cfg.RunID, SessionID: r.cfg.SessionID, SegmentID: r.cfg.SegmentID}
	var nudge *Nudge
	switch e := event.(type) {
	case ItemCompleted:
		commit.Items = []transcript.Item{e.Item}
		if len(e.mutatedPaths) > 0 {
			nudge = &Nudge{CWD: r.cfg.CWD, Paths: slices.Clone(e.mutatedPaths)}
		}
	case SegmentFinished:
		commit.Run = &e.Run
		if e.Run.State() == run.Waiting {
			commit.State = StateSuspend
			return reduction{Event: event, Commit: &commit}, nil
		}
		commit.State = StateTerminalize
		commit.CommitID = newRunCommitID()
		if outcome, terminal := e.Run.Outcome(); terminal {
			commit.Outcome = outcome
			goalRun, err := r.goalTurn(e.Run)
			if err != nil {
				return reduction{}, err
			}
			commit.GoalRun = goalRun
		}
	case ItemStarted, ItemChanged, SegmentProgressed, PlanSnapshot, SegmentStarted:
		// These events have no standalone EventCommit. SegmentStarted carries a Run
		// for the stream, but the Run's durable opening IS its admission (or its
		// resume) — recording it a second time here would be a second writer of
		// facts admission already owns. Interrupt starts are folded into the atomic
		// park write-set by project.
	default:
		return reduction{}, fmt.Errorf("%w: unhandled run event %T", errReducerInvariant, event)
	}
	var eventCommit *EventCommit
	if !commit.isEmpty() {
		eventCommit = &commit
	}
	return reduction{Event: event, Commit: eventCommit, Nudge: nudge}, nil
}

func (r *reducer) goalTurn(run run.Run) (*goal.RunRecord, error) {
	outcome, terminal := run.Outcome()
	if r.cfg.GoalIncarnationID == "" || !terminal {
		return nil, nil
	}
	record := &goal.RunRecord{
		SessionID:     r.cfg.SessionID,
		IncarnationID: r.cfg.GoalIncarnationID,
		RunID:         r.cfg.RunID,
		Outcome:       outcome,
		CompletedAt:   run.FinishedAt(),
	}
	if record.CompletedAt.IsZero() {
		record.CompletedAt = r.now()
	}
	record.Steps = run.Metrics().Steps()
	cost, err := costFromRunMetrics(run.Metrics())
	if err != nil {
		return nil, fmt.Errorf("runs: project Goal Run cost: %w", err)
	}
	record.Cost = cost
	return record, nil
}

func costFromRunMetrics(metrics run.Metrics) (accounting.Cost, error) {
	usage, reported := metrics.Usage()
	if !reported {
		return accounting.Cost{}, nil
	}
	return accounting.CostFromOptional(usage.Total.CostUSD)
}

package runs

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

type reduction struct {
	Event  ProjectionEvent
	Commit *EventCommit
	Nudge  *Nudge
}

type reductionBatch struct {
	events     []reduction
	parkCommit *EventCommit
}

type factReduction struct {
	executionContext     *ExecutorCheckpoint
	resultPublication    *ResultPublication
	toolResults          []corechat.ToolResult
	events               []ProjectionEvent
	items                []transcript.Item
	parkItems            []transcript.Item
	conversationMessages []corechat.Message
	modelInvocations     []ModelInvocationCommit
	toolInvocations      []ToolInvocationCommit
	progress             *run.Run
}

func (r *reducer) newEventCommit() *EventCommit {
	return &EventCommit{
		RunID: r.cfg.Opened.ID(), SessionID: r.cfg.Opened.SessionID(), SegmentID: r.cfg.Opened.ActiveSegmentID(),
	}
}

func (r *reducer) ensureLastEventCommit(batch *reductionBatch) *EventCommit {
	last := &batch.events[len(batch.events)-1]
	if last.Commit == nil {
		last.Commit = r.newEventCommit()
	}
	return last.Commit
}

func (r *reducer) projectFact(reduced factReduction) (reductionBatch, error) {
	batch, err := r.project(reduced.events)
	if err != nil {
		return reductionBatch{}, err
	}
	if len(reduced.parkItems) != 0 {
		if batch.parkCommit == nil {
			return reductionBatch{}, fmt.Errorf("%w: parked Items have no park boundary", errReducerInvariant)
		}
		batch.parkCommit.Items = append(batch.parkCommit.Items, NewItemWrites(reduced.parkItems...)...)
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
	if reduced.resultPublication != nil {
		if len(batch.events) == 0 || batch.parkCommit != nil {
			return reductionBatch{}, fmt.Errorf("%w: result publication has no ordinary batch", errReducerInvariant)
		}
		commit := r.ensureLastEventCommit(&batch)
		commit.ResultPublication = new(*reduced.resultPublication)
		commit.ToolResults = cloneToolResults(reduced.toolResults)
	}
	if reduced.executionContext != nil {
		if len(batch.events) == 0 || batch.parkCommit != nil {
			return reductionBatch{}, errors.New("runs: execution context has no ordinary publication")
		}
		r.ensureLastEventCommit(&batch).ExecutionContext = new(reduced.executionContext.Clone())
	}
	return batch, nil
}

func (r *reducer) attachDurableObservation(
	batch *reductionBatch,
	conversationMessages []corechat.Message,
	modelInvocations []ModelInvocationCommit,
	toolInvocations []ToolInvocationCommit,
	progress *run.Run,
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
		commit = r.ensureLastEventCommit(batch)
	}
	commit.ModelInvocations = append(commit.ModelInvocations, modelInvocations...)
	commit.ToolInvocations = append(commit.ToolInvocations, toolInvocations...)
	commit.ConversationMessages = appendClonedMessages(commit.ConversationMessages, conversationMessages...)
	commit.Progress = progress
	if commit.Progress != nil {
		commit.Progress = new(*commit.Progress)
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
	commit := r.ensureLastEventCommit(batch)
	commit.Items = append(commit.Items, NewItemWrites(items...)...)
	return validateReductionBatch(*batch)
}

func (r *reducer) attachConversationMessages(batch *reductionBatch, messages []corechat.Message) error {
	if len(messages) == 0 {
		return nil
	}
	if batch == nil || len(batch.events) == 0 || batch.parkCommit != nil {
		return fmt.Errorf("%w: conversation projection has no ordinary reduction", errReducerInvariant)
	}
	commit := r.ensureLastEventCommit(batch)
	commit.ConversationMessages = appendClonedMessages(commit.ConversationMessages, messages...)
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

	// A park is one persistence boundary: its items, open interrupt record, waiting
	// Run and admission transition commit together before any event in the batch is
	// published.
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

// A subscriber that attached late or replayed past the change event would
// otherwise reach segment.finished without the Plan; republishing places the final
// value just before the finish, and folding a revisioned value twice is folding it
// once.
func (r *reducer) fenceFinalPlan(events []ProjectionEvent) []ProjectionEvent {
	if r.plan == nil {
		return events
	}
	for i, event := range events {
		if _, finishing := event.(SegmentFinished); !finishing {
			continue
		}
		fence := r.plan.clone()
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
		return reduction{}, fmt.Errorf("%w: %T: %w", errReducerInvariant, event, err)
	}
	commit := r.newEventCommit()
	var nudge *Nudge
	switch e := event.(type) {
	case ItemCompleted:
		if e.settlement != nil {
			commit.Items = []ItemWrite{SettlementWrite(*e.settlement)}
		} else {
			commit.Items = NewItemWrites(e.Item)
		}
		// An isolated Run's tools write into a scratch copy, so the workspace is
		// unchanged; announcing it would send subscribers to re-read an unchanged tree.
		if len(e.mutatedPaths) > 0 && !r.cfg.Isolated {
			nudge = &Nudge{WorkspaceCWD: r.cfg.WorkspaceCWD, Paths: slices.Clone(e.mutatedPaths)}
		}
	case SegmentFinished:
		commit.Run = &e.Run
		if e.Run.State() == run.Waiting {
			commit.State = StateSuspend
			return reduction{Event: event, Commit: commit}, nil
		}
		commit.State = StateTerminalize
		commit.CommitID = newRunCommitID()
	case ItemStarted, ItemChanged, SegmentProgressed, PlanSnapshot, SegmentStarted, QuestionAnswered:
		// These events have no standalone EventCommit: the Run's durable opening is its
		// admission, and recording it here would be a second writer.
	default:
		return reduction{}, fmt.Errorf("%w: unhandled run event %T", errReducerInvariant, event)
	}
	var eventCommit *EventCommit
	if !commit.isEmpty() {
		eventCommit = commit
	}
	return reduction{Event: event, Commit: eventCommit, Nudge: nudge}, nil
}

package persistence

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	corechat "github.com/Tangerg/scope/core/chat"
)

// A recovery commit applies all of its repairs in one transaction, so a crash
// during startup recovery leaves every repair or none.
type RecoveryStore struct {
	sessions            *SessionStore
	runs                *RunStore
	interrupts          *InterruptStore
	transcript          *TranscriptStore
	messages            *ConversationStore
	goalRuns            *GoalStore
	executorCheckpoints *ExecutorCheckpointStore
	modelInvocations    *ModelInvocationStore
	toolInvocations     *ToolInvocationStore
	childRunStarts      *ChildRunStartReservationStore
	tx                  Transactor
}

func (b *Bundle) Recovery() *RecoveryStore {
	return &RecoveryStore{
		sessions:            b.Sessions,
		runs:                b.Runs,
		interrupts:          b.Interrupts,
		transcript:          b.Transcript,
		messages:            b.Conversations,
		goalRuns:            b.Goals,
		executorCheckpoints: b.ExecutorCheckpoints,
		modelInvocations:    b.ModelInvocations,
		toolInvocations:     b.ToolInvocations,
		childRunStarts:      b.ChildRunStarts,
		tx:                  b.Transactor,
	}
}

func (p *RecoveryStore) SessionByID(ctx context.Context, sessionID identity.SessionID) (session.Session, error) {
	return p.sessions.Get(ctx, sessionID)
}

func (p *RecoveryStore) ListNonTerminalRuns(ctx context.Context) ([]run.Run, error) {
	return p.runs.ListNonTerminalRuns(ctx)
}

func (p *RecoveryStore) ListPendingInterrupts(ctx context.Context) ([]runs.Pending, error) {
	return p.interrupts.ListOpen(ctx)
}

func (p *RecoveryStore) ListIndependentEndpoints(ctx context.Context) ([]modelref.EndpointObservation, error) {
	return p.modelInvocations.ListIndependentEndpoints(ctx)
}

func (p *RecoveryStore) ListOpenModelInvocations(ctx context.Context) ([]runs.OpenModelInvocation, error) {
	return p.modelInvocations.ListOpenModelInvocations(ctx)
}

func (p *RecoveryStore) ListOpenToolInvocations(ctx context.Context) ([]runs.OpenToolInvocation, error) {
	return p.toolInvocations.ListOpenToolInvocations(ctx)
}

func (p *RecoveryStore) ListTranscript(ctx context.Context, sessionID identity.SessionID) ([]transcript.Item, error) {
	return p.transcript.List(ctx, sessionID)
}

func (p *RecoveryStore) UnpublishedToolResults(ctx context.Context, sessionID identity.SessionID, runID identity.RunID) ([]corechat.ToolResult, error) {
	return p.runs.UnpublishedToolResults(ctx, sessionID, runID)
}

func (p *RecoveryStore) ReadMessages(ctx context.Context, sessionID identity.SessionID) ([]corechat.Message, error) {
	return p.messages.Read(ctx, sessionID)
}

func (p *RecoveryStore) LoadExecutorCheckpoint(
	ctx context.Context,
	rootMemberID string,
) (runs.ExecutorCheckpoint, error) {
	return p.executorCheckpoints.LoadCheckpoint(ctx, rootMemberID)
}

func (p *RecoveryStore) CommitRecovery(ctx context.Context, commit runs.RecoveryCommit) error {
	return p.tx(ctx, func(ctx context.Context) error { return p.applyRecovery(ctx, commit) })
}

func (p *RecoveryStore) applyRecovery(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, source := range commit.IndependentEndpoints() {
		if err := p.modelInvocations.ValidateIndependentEndpoint(ctx, source); err != nil {
			return fmt.Errorf("recovery: validate independent source: %w", err)
		}
	}
	if err := p.recoverModelInvocations(ctx, commit); err != nil {
		return err
	}
	if err := p.markToolInvocationsIncomplete(ctx, commit); err != nil {
		return err
	}
	if err := p.applyConversationTransitions(ctx, commit); err != nil {
		return err
	}
	if err := p.replaceTranscriptItems(ctx, commit); err != nil {
		return err
	}
	if err := p.recoverLostRuns(ctx, commit); err != nil {
		return err
	}
	if err := p.recordGoalRuns(ctx, commit); err != nil {
		return err
	}
	if err := p.deleteInterrupts(ctx, commit); err != nil {
		return err
	}
	if err := p.deleteExecutorCheckpoints(ctx, commit); err != nil {
		return err
	}
	return p.deleteChildRunStartReservations(ctx, commit)
}

func (p *RecoveryStore) recoverModelInvocations(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, invocation := range commit.ModelInvocations() {
		if err := p.modelInvocations.RecoverModelInvocation(ctx, invocation); err != nil {
			return fmt.Errorf("recovery: settle abandoned model invocation %q: %w", invocation.CallID, err)
		}
	}
	return nil
}

func (p *RecoveryStore) markToolInvocationsIncomplete(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, invocation := range commit.ToolInvocations() {
		if err := p.toolInvocations.RecordToolInvocation(ctx, invocation.SessionID, invocation.RunID, invocation.Transition()); err != nil {
			return fmt.Errorf("recovery: mark Tool invocation %q incomplete: %w", invocation.CallID, err)
		}
	}
	return nil
}

func (p *RecoveryStore) applyConversationTransitions(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, transition := range commit.ConversationTransitions() {
		count, err := p.messages.Count(ctx, transition.SessionID)
		if err != nil {
			return fmt.Errorf(
				"recovery: count conversation for root Run %q: %w",
				transition.RootRunID,
				err,
			)
		}
		if count != transition.ExpectedCount {
			return fmt.Errorf(
				"recovery: conversation for root Run %q moved from %d to %d messages",
				transition.RootRunID,
				transition.ExpectedCount,
				count,
			)
		}
		if err := p.messages.Write(ctx, transition.SessionID, transition.Messages...); err != nil {
			return fmt.Errorf(
				"recovery: close conversation for root Run %q: %w",
				transition.RootRunID,
				err,
			)
		}
	}
	return nil
}

func (p *RecoveryStore) replaceTranscriptItems(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, replacement := range commit.ItemReplacements() {
		if err := p.transcript.ReplaceItem(ctx, replacement); err != nil {
			return fmt.Errorf("recovery: replace transcript Item %q: %w", replacement.Expected().ID(), err)
		}
	}
	return nil
}

func (p *RecoveryStore) recoverLostRuns(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, replacement := range commit.LostRuns() {
		if err := p.runs.RecoverLost(ctx, replacement); err != nil {
			return fmt.Errorf("recovery: recover lost Run %q: %w", replacement.State().ID(), err)
		}
	}
	return nil
}

func (p *RecoveryStore) recordGoalRuns(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, charged := range commit.GoalRuns() {
		if err := p.goalRuns.CaptureGoalRun(ctx, charged); err != nil {
			return fmt.Errorf("recovery: record Goal Run for Run %q: %w", charged.ID(), err)
		}
	}
	return nil
}

func (p *RecoveryStore) deleteInterrupts(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, rootRunID := range commit.DeleteInterrupts() {
		if err := p.interrupts.Delete(ctx, rootRunID); err != nil {
			return fmt.Errorf("recovery: delete interrupt for root Run %q: %w", rootRunID, err)
		}
	}
	return nil
}

func (p *RecoveryStore) deleteExecutorCheckpoints(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, sessionID := range commit.DeleteCheckpointSessionIDs() {
		if err := p.executorCheckpoints.DeleteSessionCheckpoints(ctx, sessionID); err != nil {
			return fmt.Errorf("recovery: delete executor checkpoints for Session %q: %w", sessionID, err)
		}
	}
	return nil
}

func (p *RecoveryStore) deleteChildRunStartReservations(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, sessionID := range commit.RecoveredSessionIDs() {
		if err := p.childRunStarts.DeleteSession(ctx, sessionID); err != nil {
			return fmt.Errorf(
				"recovery: delete child Run start reservations for Session %q: %w",
				sessionID,
				err,
			)
		}
	}
	return nil
}

var _ runs.RecoveryStore = (*RecoveryStore)(nil)

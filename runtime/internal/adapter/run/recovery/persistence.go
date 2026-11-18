// Package recovery applies the boot-recovery write-set selected by the
// application. It coordinates persistence mechanics only; recovery policy and
// executor-checkpoint interpretation stay outside this adapter.
package recovery

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

type runStore interface {
	runs.ToolResultPublications
	ListNonTerminalRuns(ctx context.Context) ([]run.Run, error)
	RecoverLost(ctx context.Context, replacement run.Replacement) error
}

type sessionStore interface {
	Get(ctx context.Context, sessionID string) (session.Session, error)
}

type interruptStore interface {
	List(ctx context.Context, sessionID string) ([]runs.Pending, error)
	Delete(ctx context.Context, rootRunID string) error
}

type transcriptStore interface {
	List(ctx context.Context, sessionID string) ([]transcript.Item, error)
	ReplaceItem(ctx context.Context, replacement transcript.Replacement) error
}

type conversationStore interface {
	Read(ctx context.Context, sessionID string) ([]corechat.Message, error)
	Write(ctx context.Context, sessionID string, messages ...corechat.Message) error
	Count(ctx context.Context, sessionID string) (int, error)
}

type goalRunRecorder interface {
	CaptureGoalRun(ctx context.Context, value run.Run) error
}

type executorCheckpointStore interface {
	LoadCheckpoint(ctx context.Context, rootMemberID string) (runs.ExecutorCheckpoint, error)
	DeleteSessionCheckpoints(ctx context.Context, sessionID string) error
}

type modelInvocationStore interface {
	ListIndependentEndpoints(context.Context) ([]modelref.EndpointObservation, error)
	ValidateIndependentEndpoint(context.Context, modelref.EndpointObservation) error
	ListOpenModelInvocations(context.Context) ([]runs.OpenModelInvocation, error)
	RecoverModelInvocation(context.Context, runs.ModelInvocationRecovery) error
}

type toolInvocationStore interface {
	ListOpenToolInvocations(context.Context) ([]runs.OpenToolInvocation, error)
	RecordToolInvocation(ctx context.Context, sessionID, runID string, invocation runs.ToolInvocationCommit) error
}

// childRunStartReservations owns the process-scoped callback ledger behind
// child admission. Its rows are not recovery facts: callbacks die with the
// process, so reconciliation retires the claimed Session's abandoned ledger in
// the same transaction as the public Run repair.
type childRunStartReservations interface {
	DeleteSession(ctx context.Context, sessionID string) error
}

type Transactor func(ctx context.Context, fn func(context.Context) error) error

type Config struct {
	Sessions            sessionStore
	Runs                runStore
	Interrupts          interruptStore
	Transcript          transcriptStore
	Messages            conversationStore
	GoalRuns            goalRunRecorder
	ExecutorCheckpoints executorCheckpointStore
	ModelInvocations    modelInvocationStore
	ToolInvocations     toolInvocationStore
	ChildRunStarts      childRunStartReservations
	Tx                  Transactor
}

// Persistence implements the recovery use case's durable fact and atomic
// commit port.
type Persistence struct {
	sessions            sessionStore
	runs                runStore
	interrupts          interruptStore
	transcript          transcriptStore
	messages            conversationStore
	goalRuns            goalRunRecorder
	executorCheckpoints executorCheckpointStore
	modelInvocations    modelInvocationStore
	toolInvocations     toolInvocationStore
	childRunStarts      childRunStartReservations
	tx                  Transactor
}

func New(config Config) (*Persistence, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{"session store", config.Sessions}, {"run store", config.Runs},
		{"interrupt store", config.Interrupts}, {"transcript store", config.Transcript},
		{"conversation store", config.Messages}, {"goal run recorder", config.GoalRuns},
		{"executor checkpoint store", config.ExecutorCheckpoints},
		{"model invocation store", config.ModelInvocations}, {"tool invocation store", config.ToolInvocations},
		{"child run start reservation store", config.ChildRunStarts}, {"transactor", config.Tx},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("recovery: %s is required", required.name)
		}
	}
	return &Persistence{
		sessions:            config.Sessions,
		runs:                config.Runs,
		interrupts:          config.Interrupts,
		transcript:          config.Transcript,
		messages:            config.Messages,
		goalRuns:            config.GoalRuns,
		executorCheckpoints: config.ExecutorCheckpoints,
		modelInvocations:    config.ModelInvocations,
		toolInvocations:     config.ToolInvocations,
		childRunStarts:      config.ChildRunStarts,
		tx:                  config.Tx,
	}, nil
}

func (p *Persistence) SessionByID(ctx context.Context, sessionID string) (session.Session, error) {
	return p.sessions.Get(ctx, sessionID)
}

func (p *Persistence) ListNonTerminalRuns(ctx context.Context) ([]run.Run, error) {
	return p.runs.ListNonTerminalRuns(ctx)
}

func (p *Persistence) ListPendingInterrupts(ctx context.Context) ([]runs.Pending, error) {
	return p.interrupts.List(ctx, "")
}

func (p *Persistence) ListIndependentEndpoints(ctx context.Context) ([]modelref.EndpointObservation, error) {
	return p.modelInvocations.ListIndependentEndpoints(ctx)
}

func (p *Persistence) ListOpenModelInvocations(ctx context.Context) ([]runs.OpenModelInvocation, error) {
	return p.modelInvocations.ListOpenModelInvocations(ctx)
}

func (p *Persistence) ListOpenToolInvocations(ctx context.Context) ([]runs.OpenToolInvocation, error) {
	return p.toolInvocations.ListOpenToolInvocations(ctx)
}

func (p *Persistence) ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, error) {
	return p.transcript.List(ctx, sessionID)
}

func (p *Persistence) UnpublishedToolResults(ctx context.Context, sessionID, runID string) ([]corechat.ToolResult, error) {
	return p.runs.UnpublishedToolResults(ctx, sessionID, runID)
}

func (p *Persistence) ReadMessages(ctx context.Context, sessionID string) ([]corechat.Message, error) {
	return p.messages.Read(ctx, sessionID)
}

func (p *Persistence) LoadExecutorCheckpoint(
	ctx context.Context,
	rootMemberID string,
) (runs.ExecutorCheckpoint, error) {
	return p.executorCheckpoints.LoadCheckpoint(ctx, rootMemberID)
}

// CommitRecovery applies the application-produced write-set in one storage
// transaction. The adapter deliberately does not reinterpret ownership or
// resumability; it only preserves the ordering encoded in the plan.
func (p *Persistence) CommitRecovery(ctx context.Context, commit runs.RecoveryCommit) error {
	return p.tx(ctx, func(ctx context.Context) error { return p.applyRecovery(ctx, commit) })
}

func (p *Persistence) applyRecovery(ctx context.Context, commit runs.RecoveryCommit) error {
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

func (p *Persistence) recoverModelInvocations(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, invocation := range commit.ModelInvocations() {
		if err := p.modelInvocations.RecoverModelInvocation(ctx, invocation); err != nil {
			return fmt.Errorf("recovery: settle abandoned model invocation %q: %w", invocation.CallID, err)
		}
	}
	return nil
}

func (p *Persistence) markToolInvocationsIncomplete(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, invocation := range commit.ToolInvocations() {
		if err := p.toolInvocations.RecordToolInvocation(ctx, invocation.SessionID, invocation.RunID, invocation.Transition()); err != nil {
			return fmt.Errorf("recovery: mark Tool invocation %q incomplete: %w", invocation.CallID, err)
		}
	}
	return nil
}

func (p *Persistence) applyConversationTransitions(ctx context.Context, commit runs.RecoveryCommit) error {
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

func (p *Persistence) replaceTranscriptItems(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, replacement := range commit.ItemReplacements() {
		if err := p.transcript.ReplaceItem(ctx, replacement); err != nil {
			return fmt.Errorf("recovery: replace transcript Item %q: %w", replacement.Expected().ID(), err)
		}
	}
	return nil
}

func (p *Persistence) recoverLostRuns(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, replacement := range commit.LostRuns() {
		if err := p.runs.RecoverLost(ctx, replacement); err != nil {
			return fmt.Errorf("recovery: recover lost Run %q: %w", replacement.State().ID(), err)
		}
	}
	return nil
}

func (p *Persistence) recordGoalRuns(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, charged := range commit.GoalRuns() {
		if err := p.goalRuns.CaptureGoalRun(ctx, charged); err != nil {
			return fmt.Errorf("recovery: record Goal Run for Run %q: %w", charged.ID(), err)
		}
	}
	return nil
}

func (p *Persistence) deleteInterrupts(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, rootRunID := range commit.DeleteInterrupts() {
		if err := p.interrupts.Delete(ctx, rootRunID); err != nil {
			return fmt.Errorf("recovery: delete interrupt for root Run %q: %w", rootRunID, err)
		}
	}
	return nil
}

func (p *Persistence) deleteExecutorCheckpoints(ctx context.Context, commit runs.RecoveryCommit) error {
	for _, sessionID := range commit.DeleteCheckpointSessionIDs() {
		if err := p.executorCheckpoints.DeleteSessionCheckpoints(ctx, sessionID); err != nil {
			return fmt.Errorf("recovery: delete executor checkpoints for Session %q: %w", sessionID, err)
		}
	}
	return nil
}

func (p *Persistence) deleteChildRunStartReservations(ctx context.Context, commit runs.RecoveryCommit) error {
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

var _ runs.RecoveryStore = (*Persistence)(nil)

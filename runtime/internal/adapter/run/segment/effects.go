package segment

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

type SessionStore interface {
	Insert(ctx context.Context, value session.Session) error
	Save(ctx context.Context, replacement session.Replacement) error
}

type actorAuthority interface {
	Admit(context.Context, operation.Actor) error
}

type commandAcceptanceStore interface {
	Save(context.Context, runs.CommandAcceptance) error
	Lookup(context.Context, string) (runs.CommandAcceptanceRecord, error)
}

type goalRunRecorder interface {
	CaptureGoalRun(ctx context.Context, value run.Run) error
}

type InterruptStore interface {
	Open(ctx context.Context, p runs.Pending) error
	Consume(ctx context.Context, rootRunID identity.RunID) (runs.Pending, bool, error)
	Delete(ctx context.Context, rootRunID identity.RunID) error
}

type TranscriptStore interface {
	AppendItem(ctx context.Context, it transcript.Item) error
	SettleItem(ctx context.Context, settlement transcript.Replacement) error
}

type ItemReplacer interface {
	ReplaceItem(ctx context.Context, replacement transcript.Replacement) error
}

type rememberedRuleStore interface {
	Put(ctx context.Context, rule approval.Rule) error
}

type acceptedContinuationStore interface {
	Put(ctx context.Context, accepted runs.AcceptedContinuation) error
	PutSubtreeCancellation(ctx context.Context, accepted runs.AcceptedSubtreeCancellation) error
	Delete(ctx context.Context, rootRunID identity.RunID) error
}

type modelInvocationJournal interface {
	RecordModelInvocation(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, invocation runs.ModelInvocationCommit) error
}

type toolInvocationJournal interface {
	RecordToolInvocation(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, invocation runs.ToolInvocationCommit) error
}

type runWriter interface {
	Admit(ctx context.Context, draft run.Draft) error
	Resume(ctx context.Context, replacement run.Replacement) error
	RequireActiveSegment(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, segmentID identity.SegmentID) error
	Suspend(ctx context.Context, run run.Run, segmentID identity.SegmentID, commitID runs.CommitID) error
	Terminalize(ctx context.Context, replacement run.Replacement, winningCancellationKey string) error
	RecordRunCommit(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, segmentID identity.SegmentID, commitID runs.CommitID) error
	RecordWaitingRunCommit(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, commitID runs.CommitID) error
	TerminalizeEvent(ctx context.Context, run run.Run, segmentID identity.SegmentID, commitID runs.CommitID, winningCancellationKey string) error
}

type RunStore interface {
	ResultPublicationCommitted(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, segmentID identity.SegmentID, publicationID, digest string) (bool, error)
	RecordResultPublication(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, segmentID identity.SegmentID, publicationID, digest string, results []chat.ToolResult) error
	runs.ToolResultPublications
	runWriter
	Run(ctx context.Context, runID identity.RunID) (run.Run, bool, error)
	RunCommitCommitted(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, segmentID identity.SegmentID, commitID runs.CommitID) (bool, error)
}

type RunProgressWriter interface {
	UpdateProgress(ctx context.Context, progressed run.Run) error
}

type executorCheckpointStore interface {
	SaveCheckpoint(ctx context.Context, checkpoint runs.ExecutorCheckpoint) error
	LoadCheckpoint(ctx context.Context, rootMemberID string) (runs.ExecutorCheckpoint, error)
	DeleteCheckpoints(ctx context.Context, sessionID identity.SessionID, rootIDs []string) error
}

type childRunStartReservationStore interface {
	Reserve(ctx context.Context, record persistence.ChildRunStartReservationRecord) error
	Concluded(context.Context, persistence.ChildRunStartReservationRecord, runs.ChildRunStartOutcome) (bool, error)
	Conclude(
		ctx context.Context,
		record persistence.ChildRunStartReservationRecord,
		conclusion runs.ChildRunStartOutcome,
	) (changed bool, err error)
	DeleteSession(ctx context.Context, sessionID identity.SessionID) error
}

type Transactor func(ctx context.Context, fn func(context.Context) error) error

type BindingTransactor func(context.Context, string, runs.ExecutorCheckpoint, func(context.Context) error) error

type conversationStore interface {
	Read(ctx context.Context, sessionID identity.SessionID) ([]chat.Message, error)
	Write(ctx context.Context, sessionID identity.SessionID, messages ...chat.Message) error
	Count(ctx context.Context, sessionID identity.SessionID) (int, error)
}

type sessionContextInvalidator interface{ ForgetSessionContext(identity.SessionID) }

type Config struct {
	Compactions           runs.ConversationCompactionStore
	ContextState          sessionContextInvalidator
	ExecutionTrees        runs.ExecutionTreeStore
	Interrupts            InterruptStore
	Sessions              SessionStore
	ActorAuthority        actorAuthority
	CommandAcceptances    commandAcceptanceStore
	GoalRuns              goalRunRecorder
	Transcript            TranscriptStore
	ItemReplacer          ItemReplacer
	ModelInvocations      modelInvocationJournal
	ToolInvocations       toolInvocationJournal
	Conversation          conversationStore
	State                 RunStore
	RunProgress           RunProgressWriter
	ExecutorCheckpoints   executorCheckpointStore
	ChildRunStarts        childRunStartReservationStore
	Tx                    Transactor
	PluginBindings        BindingTransactor
	RememberedRules       rememberedRuleStore
	AcceptedContinuations acceptedContinuationStore
}

type Effects struct {
	compactions           runs.ConversationCompactionStore
	contextState          sessionContextInvalidator
	executionTrees        runs.ExecutionTreeStore
	interrupts            InterruptStore
	sessions              SessionStore
	actorAuthority        actorAuthority
	commandAcceptances    commandAcceptanceStore
	goalRuns              goalRunRecorder
	transcript            TranscriptStore
	itemReplacer          ItemReplacer
	modelInvocations      modelInvocationJournal
	toolInvocations       toolInvocationJournal
	conversation          conversationStore
	runState              RunStore
	runProgress           RunProgressWriter
	executorCheckpoints   executorCheckpointStore
	childRunStarts        childRunStartReservationStore
	tx                    Transactor
	pluginBindings        BindingTransactor
	rememberedRules       rememberedRuleStore
	acceptedContinuations acceptedContinuationStore
}

var (
	_ runs.OpeningCommitter                    = (*Effects)(nil)
	_ runs.ChildRunStartCommitter              = (*Effects)(nil)
	_ runs.EventCommitter                      = (*Effects)(nil)
	_ runs.TreeBarrierCommitter                = (*Effects)(nil)
	_ runs.WaitingCheckpointReader             = (*Effects)(nil)
	_ runs.WaitingSubtreeCancellationCommitter = (*Effects)(nil)
)

const instrumentationName = "github.com/Tangerg/flame/runtime/internal/adapter/run/segment"

func New(cfg Config) (*Effects, error) {
	required := []struct {
		name  string
		value any
	}{
		{"interrupt store", cfg.Interrupts},
		{"session store", cfg.Sessions},
		{"transcript store", cfg.Transcript},
		{"item replacer", cfg.ItemReplacer},
		{"remembered rule store", cfg.RememberedRules},
		{"accepted continuation store", cfg.AcceptedContinuations},
		{"model invocation journal", cfg.ModelInvocations},
		{"tool invocation journal", cfg.ToolInvocations},
		{"conversation store", cfg.Conversation},
		{"compaction store", cfg.Compactions},
		{"Session context invalidator", cfg.ContextState},
		{"run store", cfg.State},
		{"run progress writer", cfg.RunProgress},
		{"executor checkpoint store", cfg.ExecutorCheckpoints},
		{"execution tree store", cfg.ExecutionTrees},
		{"child run start store", cfg.ChildRunStarts},
		{"actor authority", cfg.ActorAuthority},
		{"command acceptance store", cfg.CommandAcceptances},
		{"goal run recorder", cfg.GoalRuns},
		{"transactor", cfg.Tx},
		{"plugin binding transactor", cfg.PluginBindings},
	}
	for _, required := range required {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("segment: %s is required", required.name)
		}
	}
	return &Effects{
		compactions: cfg.Compactions, contextState: cfg.ContextState,
		interrupts:            cfg.Interrupts,
		sessions:              cfg.Sessions,
		actorAuthority:        cfg.ActorAuthority,
		commandAcceptances:    cfg.CommandAcceptances,
		goalRuns:              cfg.GoalRuns,
		transcript:            cfg.Transcript,
		itemReplacer:          cfg.ItemReplacer,
		modelInvocations:      cfg.ModelInvocations,
		toolInvocations:       cfg.ToolInvocations,
		conversation:          cfg.Conversation,
		runState:              cfg.State,
		runProgress:           cfg.RunProgress,
		executorCheckpoints:   cfg.ExecutorCheckpoints,
		executionTrees:        cfg.ExecutionTrees,
		childRunStarts:        cfg.ChildRunStarts,
		tx:                    cfg.Tx,
		pluginBindings:        cfg.PluginBindings,
		rememberedRules:       cfg.RememberedRules,
		acceptedContinuations: cfg.AcceptedContinuations,
	}, nil
}

func (e *Effects) ReadWaitingCheckpoint(
	ctx context.Context,
	rootMemberID string,
) (runs.ExecutorCheckpoint, error) {
	checkpoint, err := e.executorCheckpoints.LoadCheckpoint(ctx, rootMemberID)
	if err != nil {
		return runs.ExecutorCheckpoint{}, fmt.Errorf("segment: load waiting executor checkpoint: %w", err)
	}
	return checkpoint.Clone(), nil
}

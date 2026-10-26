// Package segment is the driven adapter that executes the durable side
// effects of one streamed run segment. It implements the application's
// runs.Effects port: the run pump hands it a [runs.EventCommit] per event,
// which it applies ATOMICALLY — the open-interrupt record, transcript
// projections, and the run-state transition land in one transaction,
// so a crash never leaves a parked run with no admission mark or a terminal
// transcript with a still-running row. It also runs the non-durable live
// workspace nudge and terminal boundary maintenance (checkpoint snapshot,
// title). The fields only the runtime can resolve — an interrupt's executor member ID
// from live execution, a terminal Run's message watermark — it fills in itself.
package segment

import (
	"context"
	"fmt"
	"time"

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

// SessionStore is the exact persistence surface used inside an opening
// transaction. Application and Domain have already decided each aggregate.
type SessionStore interface {
	Insert(ctx context.Context, value session.Session) error
	Save(ctx context.Context, replacement session.Replacement) error
}

type actorAuthority interface {
	Admit(context.Context, operation.Actor) error
}

// CommandAcceptanceStore commits Run command receipts in the same transaction
// as their opening or input-admission head and confirms uncertain commit ACKs.
type commandAcceptanceStore interface {
	Save(context.Context, runs.CommandAcceptance) error
	Lookup(context.Context, string) (runs.CommandAcceptanceRecord, error)
}

// GoalRunRecorder applies a terminal goal-owned Run to its Session's Goal. It
// runs in the same transaction as terminalizing that Run.
type goalRunRecorder interface {
	RecordRun(ctx context.Context, value run.Run) error
}

// InterruptStore is the run-segment lifecycle view of the interrupt registry.
// It opens a new quiescent barrier and consumes the open hand-off when a
// continuation accepts its answers or the owning root terminalizes.
type InterruptStore interface {
	Open(ctx context.Context, p runs.Pending) error
	Consume(ctx context.Context, rootRunID string) (runs.Pending, bool, error)
	Delete(ctx context.Context, rootRunID string) error
}

// TranscriptStore is the run-segment append side of durable transcript
// persistence. Reading and destructive deletion belong to other use-cases.
type TranscriptStore interface {
	AppendItem(ctx context.Context, it transcript.Item) error
}

type ItemReplacer interface {
	ReplaceItem(ctx context.Context, replacement transcript.Replacement) error
}

// RememberedRuleStore persists a standing approval rule inside the acceptance
// transaction. The rule was already derived and validated against its frozen
// source authority by the acceptance owner; this store only writes it so it lands
// atomically with the answer, consumed open set, and Segment opening.
type rememberedRuleStore interface {
	Put(ctx context.Context, rule approval.Rule) error
}

// AcceptedContinuationStore writes the Run-owned recovery binding of an accepted
// continuation inside the acceptance transaction, so a crash before delivery
// completes leaves exactly one durable accepted-paused tree to revive. It carries
// only routing; the answer resolutions stay owned by the settled Items.
type acceptedContinuationStore interface {
	Put(ctx context.Context, accepted runs.AcceptedContinuation) error
	Delete(ctx context.Context, rootRunID string) error
}

type toolResultStore interface {
	Discard(ctx context.Context, sessionID string, ref transcript.ToolResultRef) error
}

// ModelInvocationJournal applies provider-attempt transitions. It is an
// operational evidence journal. A known inner response can accompany outer failure;
// it never creates executable Transcript continuation or advances Run accounting.
// The consumer-owned methods keep application enums out of storage.
type modelInvocationJournal interface {
	StartModelInvocation(
		ctx context.Context,
		sessionID, runID, segmentID, callID string,
		startedAt time.Time,
		purpose run.ModelCallPurpose,
	) error
	CompleteModelInvocation(
		ctx context.Context,
		sessionID, runID, segmentID, callID string,
		startedAt, finishedAt time.Time,
		firstOutputLatencyMillis *int64,
		usage *chat.Usage,
	) error
	FailModelInvocation(
		ctx context.Context,
		sessionID, runID, segmentID, callID string,
		startedAt, finishedAt time.Time,
		firstOutputLatencyMillis *int64,
		usage *chat.Usage, response *chat.Response,
	) error
	MarkModelInvocationUnknown(
		ctx context.Context,
		sessionID, runID, segmentID, callID string,
		startedAt, finishedAt time.Time,
	) error
}

// ToolInvocationJournal records pre-call and terminal attempt boundaries without
// using Transcript insertion order as an operational lock. Final semantic Tool
// Items are still committed through TranscriptStore in canonical model order.
type toolInvocationJournal interface {
	StartToolInvocation(
		ctx context.Context,
		sessionID, runID, segmentID, callID, itemID string,
		startedAt time.Time,
	) error
	CompleteToolInvocation(
		ctx context.Context,
		sessionID, runID, segmentID, callID, itemID string,
		startedAt, finishedAt time.Time,
	) error
	MarkToolInvocationIncomplete(
		ctx context.Context,
		sessionID, runID, segmentID, callID, itemID string,
		startedAt, finishedAt time.Time,
	) error
}

// RunWriter applies the run's lifecycle transitions inside the event commit:
// an opening admits or resumes it, a park suspends it, and a terminal
// ends it — each in the SAME transaction as the interrupt / item records it must
// stay consistent with. Suspend and Terminalize both take the whole Run because
// a state change and the accounting true at that moment are one fact: a park is
// a durable commit, so what the Run had spent by then is committed with it. The
// RequireActiveSegment is the admission fence for every EventCommit: it proves
// the complete write-set still belongs to the exact running Segment before any
// transcript, conversation, invocation, accounting, or lifecycle fact moves.
// RecordRunCommit records the Application-owned immutable write-set identity
// after a non-terminal command's complete projections in the same transaction.
// Suspend and TerminalizeEvent record it in their boundary transitions.
// The sqlite RunStore satisfies it.
type runWriter interface {
	Admit(ctx context.Context, draft run.Draft) error
	Resume(ctx context.Context, replacement run.Replacement) error
	RequireActiveSegment(ctx context.Context, sessionID, runID, segmentID string) error
	Suspend(ctx context.Context, run run.Run, segmentID string, commitID runs.CommitID) error
	Terminalize(ctx context.Context, replacement run.Replacement) error
	RecordRunCommit(ctx context.Context, sessionID, runID, segmentID string, commitID runs.CommitID) error
	RecordWaitingRunCommit(ctx context.Context, sessionID, runID string, commitID runs.CommitID) error
	TerminalizeEvent(ctx context.Context, run run.Run, segmentID string, commitID runs.CommitID) error
}

// RunStore combines lifecycle writes with the exact durable reads required to
// prove and reconstruct a result after an ambiguous command commit.
type RunStore interface {
	ResultPublicationCommitted(ctx context.Context, sessionID, runID, segmentID, publicationID, digest string) (bool, error)
	RecordResultPublication(ctx context.Context, sessionID, runID, segmentID, publicationID, digest string, results []chat.ToolResult) error
	runs.ToolResultPublications
	runWriter
	Run(ctx context.Context, runID string) (run.Run, bool, error)
	RunCommitCommitted(ctx context.Context, sessionID, runID, segmentID string, commitID runs.CommitID) (bool, error)
}

// RunProgressWriter writes the running Run the Application advanced at one
// model-response boundary, fenced to its active Segment. Keeping it separate
// from lifecycle writing lets consumers depend on the narrower behavior they
// actually exercise.
type RunProgressWriter interface {
	UpdateProgress(ctx context.Context, progressed run.Run) error
}

// ExecutorCheckpointStore persists and removes root-owned opaque executor
// checkpoints selected by the Run lifecycle. It never interprets the
// payload.
type executorCheckpointStore interface {
	SaveCheckpoint(ctx context.Context, checkpoint runs.ExecutorCheckpoint) error
	LoadCheckpoint(ctx context.Context, rootMemberID string) (runs.ExecutorCheckpoint, error)
	DeleteCheckpoints(ctx context.Context, sessionID string, rootIDs []string) error
}

// ChildRunStartReservationStore is the technical persistence mechanism behind
// the Application-owned child-start write-set. Its payload is opaque to SQLite;
// this adapter alone translates the application value.
type childRunStartReservationStore interface {
	Reserve(ctx context.Context, record persistence.ChildRunStartReservationRecord) error
	Concluded(context.Context, persistence.ChildRunStartReservationRecord, persistence.ChildRunStartConclusion) (bool, error)
	Conclude(
		ctx context.Context,
		record persistence.ChildRunStartReservationRecord,
		conclusion persistence.ChildRunStartConclusion,
	) (changed bool, err error)
	DeleteSession(ctx context.Context, sessionID string) error
}

// Transactor runs fn inside one storage transaction: every store call made by
// fn joins that transaction through the context. Durable commits reject a nil
// transactor rather than silently weakening atomicity.
type Transactor func(ctx context.Context, fn func(context.Context) error) error

// ConversationStore appends root execution context and resolves the watermark
// recorded by a terminal Run. Both operations join the event transaction.
type conversationStore interface {
	Read(ctx context.Context, sessionID string) ([]chat.Message, error)
	Write(ctx context.Context, sessionID string, messages ...chat.Message) error
	Count(ctx context.Context, sessionID string) (int, error)
}

type sessionContextInvalidator interface{ ForgetSessionContext(string) }

// Config bundles the Effects dependencies.
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
	ToolResults           toolResultStore
	ModelInvocations      modelInvocationJournal
	ToolInvocations       toolInvocationJournal
	Conversation          conversationStore
	State                 RunStore
	RunProgress           RunProgressWriter
	ExecutorCheckpoints   executorCheckpointStore
	ChildRunStarts        childRunStartReservationStore
	Tx                    Transactor
	RememberedRules       rememberedRuleStore
	AcceptedContinuations acceptedContinuationStore
}

// Effects coordinates run-segment side effects. It is stateless beyond its
// dependencies and safe to share.
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
	toolResults           toolResultStore
	modelInvocations      modelInvocationJournal
	toolInvocations       toolInvocationJournal
	conversation          conversationStore
	runState              RunStore
	runProgress           RunProgressWriter
	executorCheckpoints   executorCheckpointStore
	childRunStarts        childRunStartReservationStore
	tx                    Transactor
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

const runsegmentTracerName = "scope/flame/segment"

// New returns the durable Run-segment effects. Every dependency is required: a
// segment commit is one transaction, and a missing store would either fail it
// halfway or drop the schedule, Goal-accounting or Tool-result work it owns.
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
		{"tool result store", cfg.ToolResults},
		{"transactor", cfg.Tx},
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
		toolResults:           cfg.ToolResults,
		modelInvocations:      cfg.ModelInvocations,
		toolInvocations:       cfg.ToolInvocations,
		conversation:          cfg.Conversation,
		runState:              cfg.State,
		runProgress:           cfg.RunProgress,
		executorCheckpoints:   cfg.ExecutorCheckpoints,
		executionTrees:        cfg.ExecutionTrees,
		childRunStarts:        cfg.ChildRunStarts,
		tx:                    cfg.Tx,
		rememberedRules:       cfg.RememberedRules,
		acceptedContinuations: cfg.AcceptedContinuations,
	}, nil
}

// ReadWaitingCheckpoint returns the exact opaque recovery point selected by
// the Application waiting-tree use case.
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

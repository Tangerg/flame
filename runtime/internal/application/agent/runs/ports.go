package runs

import (
	"context"
	"iter"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	corechat "github.com/Tangerg/scope/core/chat"
)

type RootExecutionStarter interface {
	StageRoot(ctx context.Context, start RootExecutionStart) (ExecutorRef, error)
	BeginRoot(ctx context.Context, ref ExecutorRef) error
}

type ExecutionObserver interface {
	Observe(ctx context.Context, ref ExecutorRef) (iter.Seq[ExecutorEvent], error)
}

type ExecutionReleaser interface {
	Release(ctx context.Context, ref ExecutorRef) error
}

type RunningRootCancellationRequester interface {
	RequestRootCancellation(ctx context.Context, ref ExecutorRef, cancellation CancellationID) error
}

type ConversationReader interface {
	Read(ctx context.Context, sessionID identity.SessionID) ([]corechat.Message, error)
}

// Implementations return a self-contained snapshot; the executor never calls back
// into this port while running or restoring.
type WorkingContextComposer interface {
	ComposeWorkingContext(
		ctx context.Context,
		input WorkingContextInput,
	) ([]corechat.Message, error)
}

type modelAdmitter interface {
	AdmitSelection(ctx context.Context, selection flamemodel.Selection) error
}

type WorkingContextInput struct {
	SessionID    identity.SessionID
	CWD          string
	WorkspaceCWD string
	Project      identity.ProjectID
	PromptText   string
	Seed         []corechat.Message
}

type ExecutionContinuer interface {
	PrepareContinuation(
		ctx context.Context,
		continuation ExecutionContinuation,
		answers []InterruptAnswer,
		candidates []RememberedRuleCandidate,
	) ([]approval.RememberRequest, error)
	StageAcceptedContinuation(ctx context.Context, continuation ExecutionContinuation) (ExecutorRef, error)
	BeginContinuation(
		ctx context.Context,
		ref ExecutorRef,
		answers []InterruptAnswer,
		input *CommittedUserInput,
	) error
}

type RememberedRuleCandidate struct {
	Request            approval.RememberRequest
	CallID             string
	EffectiveArguments tool.Arguments
}

type approvalRulePreparer interface {
	PrepareRule(ctx context.Context, request approval.RememberRequest) (approval.Rule, error)
}

type acceptedContinuationStore interface {
	List(ctx context.Context) ([]AcceptedContinuation, error)
	Delete(ctx context.Context, rootRunID identity.RunID) error
}

type acceptedSubtreeCancellationStore interface {
	ListSubtreeCancellations(ctx context.Context) ([]AcceptedSubtreeCancellation, error)
	SubtreeCancellations(ctx context.Context, rootRunID identity.RunID) ([]AcceptedSubtreeCancellation, error)
	DeleteSubtreeCancellation(ctx context.Context, rootRunID, targetRunID identity.RunID) error
}

type AcceptedSubtreeCancellationRestorer interface {
	StageAcceptedSubtreeCancellation(context.Context, WaitingSubtreeCancellationRequest, WaitingSubtreeDisposition) (RecoveredSubtreeCancellation, error)
}

type RecoveredSubtreeCancellation interface {
	Settle(context.Context) error
}

type RunningExecutionSteerer interface {
	SubmitSteer(ctx context.Context, ref ExecutorRef, input PreparedSteer) (identity.ItemID, error)
}

type RunningSubtreeCanceler interface {
	CancelRunningSubtree(
		ctx context.Context,
		ref ExecutorRef,
		memberID string,
		cancellation CancellationID,
	) error
}

type WaitingSubtreeCancellationPreparer interface {
	PrepareWaitingSubtreeCancellation(
		ctx context.Context,
		request WaitingSubtreeCancellationRequest,
	) (PreparedWaitingSubtreeCancellation, error)
}

type WaitingSubtreeDisposition string

const (
	WaitingSubtreeStaysWaiting   WaitingSubtreeDisposition = "waiting"
	WaitingSubtreeResumesRunning WaitingSubtreeDisposition = "running"
)

func (w WaitingSubtreeDisposition) Valid() bool {
	return w == WaitingSubtreeStaysWaiting || w == WaitingSubtreeResumesRunning
}

type waitingSubtreeChange interface {
	Apply(disposition WaitingSubtreeDisposition) error
	Continue(ctx context.Context) error
	Discard() error
}

type PreparedWaitingSubtreeCancellation struct {
	canceledMemberIDs    []string
	pendingInterruptions []MemberInterruption
	checkpoint           ExecutorCheckpoint
	change               waitingSubtreeChange
}

type sessionReader interface {
	Get(ctx context.Context, id identity.SessionID) (session.Session, error)
}

type sessionCreator interface {
	PrepareOpening(
		ctx context.Context,
		id identity.SessionID,
		title, cwd string,
		project identity.ProjectID,
		selection flamemodel.Selection,
	) (session.Session, *session.Session, error)
}

type activeRunReader interface {
	ActiveRun(ctx context.Context, sessionID identity.SessionID) (run.Run, bool, error)
}

type pendingInterruptReader interface {
	LookupOpenInterrupt(ctx context.Context, runID identity.RunID) (Pending, bool, error)
}

type terminationCommitter interface {
	ApplyRunCancel(ctx context.Context, ref ExecutorRef, runID identity.RunID, reason string, finishedAt time.Time, receipt CancellationReceipt) (run.Run, error)
	ApplyRunLost(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, finishedAt time.Time) error
}

type Projection interface {
	ListRuns(context.Context, identity.SessionID) ([]run.Run, error)
	Run(ctx context.Context, runID identity.RunID) (run.Run, bool, error)
	Tree(ctx context.Context, runID identity.RunID) ([]run.Run, error)
}

type ItemProjection interface {
	Item(ctx context.Context, itemID identity.ItemID) (transcript.Item, bool, error)
}

type RootExecutionStart struct {
	SessionID                identity.SessionID
	RunID                    identity.RunID
	CWD                      string
	WorkspaceCWD             string
	Project                  identity.ProjectID
	Isolated                 bool
	ModelSelection           flamemodel.Selection
	Options                  *corechat.Options
	WorkingContext           []corechat.Message
	InterruptTypes           []run.InterruptType
	ChildRunAdmissionEnabled bool
	GoalIncarnationID        goalref.IncarnationID
}

type ExecutionMember struct {
	RunID           identity.RunID
	MemberID        string
	ParentRunID     identity.RunID
	SpawnedByItemID identity.ItemID
	ModelSelection  flamemodel.Selection
	DrainedTools    []DrainedTool
}

type ExecutionContinuation struct {
	SessionID         identity.SessionID
	ExecutorID        string
	RootRunID         identity.RunID
	Members           []ExecutionMember
	Checkpoint        ExecutorCheckpoint
	Capabilities      run.Capabilities
	GoalIncarnationID goalref.IncarnationID
	Workspace         string
	Project           identity.ProjectID
	Isolated          bool
}

type isolationProvider interface {
	Workspace(ctx context.Context, sessionID identity.SessionID, projectRoot string) (string, error)
}

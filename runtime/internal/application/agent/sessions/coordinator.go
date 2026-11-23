package sessions

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/keylock"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

type Store interface {
	List(ctx context.Context) ([]session.Session, error)
	ListPage(ctx context.Context, read session.CatalogRead) ([]session.Session, error)
	Get(ctx context.Context, id identity.SessionID) (session.Session, error)
	Insert(ctx context.Context, value session.Session) error
	Save(ctx context.Context, replacement session.Replacement) error
}

type modelAdmitter interface {
	AdmitSelection(ctx context.Context, selection flamemodel.Selection) error
}

type InterruptStore interface {
	List(ctx context.Context, sessionID identity.SessionID) ([]runs.Pending, error)
	Get(ctx context.Context, runID identity.RunID) (runs.Pending, bool, error)
}

type TranscriptStore interface {
	List(ctx context.Context, sessionID identity.SessionID) ([]transcript.Item, error)
}

type planBoundaries interface {
	Boundary(ctx context.Context, runID identity.RunID) (plan.Document, bool, error)
}

type RunStore interface {
	runs.ToolResultPublications
	runReader
	ListNonTerminalRuns(ctx context.Context) ([]run.Run, error)
}

type WriteSets interface {
	ApplyFork(ctx context.Context, plan ForkPlan) (session.Session, error)
	ApplyRollback(ctx context.Context, plan RollbackPlan) error
	ApplyRestore(ctx context.Context, plan RestorePlan) error
	ApplyDelete(ctx context.Context, plan DeletePlan) error
	ApplyTerminal(ctx context.Context, plan TerminalPlan, winningCancellationKey string) error
}

type SnapshotReader interface {
	ReadSnapshot(ctx context.Context, sessionID identity.SessionID) (Snapshot, error)
}

type sessionSnapshots interface {
	SnapshotReader
	ReadForkMaterial(ctx context.Context, sessionID identity.SessionID) (ForkMaterial, error)
}

type MaterialSnapshotReader interface {
	ReadMaterialSnapshot(ctx context.Context, sessionID identity.SessionID) (MaterialSnapshot, error)
}

type transientState interface {
	QuiesceSession(sessionID identity.SessionID) error
	QuiesceWorkspace(root string) error
	ForgetSession(sessionID identity.SessionID)
	ForgetSessionContext(sessionID identity.SessionID)
	ForgetWorkspace(root string)
}

type HistorySnapshot struct {
	Session     session.Session
	Messages    []chat.Message
	Items       []transcript.Item
	Runs        []run.Run
	ToolResults []transcript.ToolResultBlob
}

type Snapshot struct {
	HistorySnapshot
	Plan       []plan.Step
	PlanSource extension.ContributionSource
}

type WorkspaceCheckpoints interface {
	Restore(ctx context.Context, sessionID identity.SessionID, cwd string, runID identity.RunID) error
	DropSession(ctx context.Context, sessionID identity.SessionID) error
}

type sandboxDiscarder interface {
	Discard(sessionID identity.SessionID) error
}

// afterCommit runs exactly once after commit succeeds; settlement after a
// successful commit is recorded rather than returned so a committed mutation is
// never reported as failed.
type goalMutationGuard interface {
	WithSessionMutation(
		ctx context.Context,
		sessionIDs []identity.SessionID,
		commit func(context.Context) error,
		afterCommit func(context.Context) error,
	) error
}

type workspaceMutations interface {
	Record(ctx context.Context, m WorkspaceMutation) (bool, error)
	Complete(ctx context.Context, m WorkspaceMutation, write func(context.Context) ([]byte, error)) error
	ListPending(ctx context.Context) ([]WorkspaceMutation, error)
}

type resourceRetirements interface {
	Lookup(context.Context, identity.SessionID) (ResourceRetirement, bool, error)
	ListPending(context.Context) ([]ResourceRetirement, error)
	Complete(context.Context, ResourceRetirement) error
}

type executionReleaser interface {
	Release(ctx context.Context, ref runs.ExecutorRef) error
}

type Coordinator struct {
	sessions              Store
	interrupts            InterruptStore
	transcript            TranscriptStore
	runs                  RunStore
	plan                  PlanServices
	snapshots             sessionSnapshots
	materialSnapshots     MaterialSnapshotReader
	writes                WriteSets
	transientState        transientState
	executionReleaser     executionReleaser
	paths                 workspaceResolver
	models                modelAdmitter
	defaultModelSelection flamemodel.Selection
	checkpoints           WorkspaceCheckpoints
	sandbox               sandboxDiscarder
	goals                 goalMutationGuard
	mutations             workspaceMutations
	retirer               *ResourceRetirer
	admissions            Admissions
	titles                titleClaims
	// Notices are published post-commit: a signal for a rolled-back transaction would
	// send listeners to re-read unchanged state.
	invalidations    invalidation.Publish
	rollbackReceipts RollbackReceiptEncoder
	now              func() time.Time
	newID            func() identity.SessionID
	newRunID         func() identity.RunID
	newItemID        func() identity.ItemID
}

type Dependencies struct {
	Sessions              Store
	Interrupts            InterruptStore
	Transcript            TranscriptStore
	Runs                  RunStore
	Plan                  PlanServices
	Snapshots             sessionSnapshots
	MaterialSnapshots     MaterialSnapshotReader
	Writes                WriteSets
	TransientState        transientState
	ExecutionReleaser     executionReleaser
	Paths                 workspaceResolver
	Models                modelAdmitter
	DefaultModelSelection flamemodel.Selection
	Checkpoints           WorkspaceCheckpoints
	Sandbox               sandboxDiscarder
	Goals                 goalMutationGuard
	Mutations             workspaceMutations
	Retirer               *ResourceRetirer
	Admissions            Admissions
	Invalidations         invalidation.Publish
	RollbackReceipts      RollbackReceiptEncoder
	Now                   func() time.Time
	NewID                 func() identity.SessionID
	NewRunID              func() identity.RunID
	NewItemID             func() identity.ItemID
}

func New(deps Dependencies) (*Coordinator, error) {
	required := []struct {
		name  string
		value any
	}{
		{"session store", deps.Sessions},
		{"plan boundary reader", deps.Plan.Boundaries},
		{"plan replacements", deps.Plan.Replacements},
		{"goal mutation guard", deps.Goals},
		{"workspace mutations", deps.Mutations},
		{"session resource retirer", deps.Retirer},
		{"interrupt store", deps.Interrupts},
		{"transcript store", deps.Transcript},
		{"run store", deps.Runs},
		{"snapshot reader", deps.Snapshots},
		{"material snapshot reader", deps.MaterialSnapshots},
		{"write sets", deps.Writes},
		{"session transient state", deps.TransientState},
		{"execution releaser", deps.ExecutionReleaser},
		{"workspace resolver", deps.Paths},
		{"model admitter", deps.Models},
		{"admissions", deps.Admissions},
		{"workspace checkpoints", deps.Checkpoints},
		{"sandbox discarder", deps.Sandbox},
		{"clock", deps.Now},
		{"session id generator", deps.NewID},
		{"run id generator", deps.NewRunID},
		{"item id generator", deps.NewItemID},
		{"invalidations", deps.Invalidations},
		{"rollback receipt encoder", deps.RollbackReceipts},
	}
	for _, required := range required {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("sessions: %s is required", required.name)
		}
	}
	if err := deps.DefaultModelSelection.ValidateExact(); err != nil {
		return nil, fmt.Errorf("sessions: default model selection: %w", err)
	}
	return &Coordinator{
		sessions:              deps.Sessions,
		interrupts:            deps.Interrupts,
		transcript:            deps.Transcript,
		runs:                  deps.Runs,
		plan:                  deps.Plan,
		snapshots:             deps.Snapshots,
		materialSnapshots:     deps.MaterialSnapshots,
		writes:                deps.Writes,
		transientState:        deps.TransientState,
		executionReleaser:     deps.ExecutionReleaser,
		paths:                 deps.Paths,
		models:                deps.Models,
		defaultModelSelection: deps.DefaultModelSelection,
		checkpoints:           deps.Checkpoints,
		sandbox:               deps.Sandbox,
		goals:                 deps.Goals,
		mutations:             deps.Mutations,
		retirer:               deps.Retirer,
		admissions:            deps.Admissions,
		titles:                titleClaims{writes: keylock.NewSet()},
		invalidations:         deps.Invalidations,
		rollbackReceipts:      deps.RollbackReceipts,
		now:                   deps.Now,
		newID:                 deps.NewID,
		newRunID:              deps.NewRunID,
		newItemID:             deps.NewItemID,
	}, nil
}

func (c *Coordinator) ClaimWorkingTreeMutation(cwd string) (WorkingTreeAdmission, bool, error) {
	release, ok, err := c.admissions.AcquireWorkingTreeMutation(cwd)
	if err != nil || !ok {
		return WorkingTreeAdmission{}, false, err
	}
	return heldWorkingTreeAdmission(release), true, nil
}

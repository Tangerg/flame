package terminal

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/schedule"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

// Runtime is the terminal's complete consumer surface. Feature-specific
// capabilities remain separate so an unavailable negotiated feature is nil
// instead of a partially functioning Runtime method set.
type Runtime interface {
	ListSessions(context.Context, agent.SessionQuery) (agent.SessionPage, error)
	GetSession(context.Context, string) (agent.SessionSnapshot, error)
	CreateSession(context.Context, agent.CreateSession) (agent.Session, error)
	UpdateSession(context.Context, agent.UpdateSession) (agent.Session, error)
	ForkSession(context.Context, agent.ForkSession) (agent.Session, error)
	RollbackSession(context.Context, agent.RollbackSession) (agent.RollbackResult, error)
	DeleteSession(context.Context, agent.DeleteSession) error
	GetRun(context.Context, string) (agent.Run, error)
	ListRuns(context.Context, agent.RunQuery) (agent.RunPage, error)
	StartRun(context.Context, agent.StartRun) (agent.SegmentStream, error)
	ResumeRun(context.Context, agent.ResumeRun) (agent.SegmentStream, error)
	SubscribeRun(context.Context, agent.SubscribeRun) (agent.SegmentStream, error)
	SteerRun(context.Context, agent.SteerRun) error
	CancelRun(context.Context, agent.CancelRun) (agent.RunCancellation, error)
	ListModels(context.Context) ([]protocol.Model, error)
	GetApprovalMode(context.Context) (agent.ApprovalMode, error)
	SetApprovalMode(context.Context, agent.ApprovalMode) (agent.ApprovalMode, error)
	ListApprovalRules(context.Context, string) ([]agent.ApprovalRule, error)
	DeleteApprovalRule(context.Context, string) error
}

type Workspaces interface {
	Resolve(context.Context, workspace.ResolveRequest) (workspace.Workspace, error)
	List(context.Context) ([]workspace.Summary, error)
	Diff(context.Context, workspace.DiffRequest) (workspace.Diff, error)
	Head(context.Context, workspace.HeadRequest) (workspace.FileHead, error)
	Search(context.Context, workspace.SearchRequest) (workspace.SearchResult, error)
	Files(context.Context, workspace.FilesRequest) (workspace.FileListing, error)
	Read(context.Context, workspace.ReadRequest) (workspace.FileContent, error)
	Changes(context.Context, string) ([]workspace.Change, error)
}

type WorkspaceChanges interface {
	Changes(context.Context, string) ([]workspace.Change, error)
}

type Usage interface {
	SessionUsage(context.Context, string) (agent.SessionUsageReport, error)
	Summary(context.Context, agent.UsageSummaryPeriod) (agent.UsageSummary, error)
}

type Goals interface {
	GetGoal(context.Context, string) (agent.Goal, bool, error)
	StartGoal(context.Context, agent.StartGoal) (agent.Goal, error)
	UpdateGoal(context.Context, agent.UpdateGoal) (agent.Goal, error)
	ClearGoal(context.Context, string) error
	StopGoal(context.Context, string) (agent.Goal, error)
	ResumeGoal(context.Context, string) (agent.Goal, error)
}

type Skills interface {
	Discover(context.Context, string) ([]workspace.DiscoveredSkill, error)
	Managed(context.Context) ([]workspace.ManagedSkill, error)
	Proposals(context.Context, string) ([]workspace.SkillProposal, error)
	Archive(context.Context, string) error
	Restore(context.Context, string) error
	Approve(context.Context, workspace.SkillProposalReference) error
	Reject(context.Context, workspace.SkillProposalReference) error
}

type Schedules interface {
	Schedules(context.Context) ([]schedule.Schedule, error)
	Create(context.Context, schedule.Candidate) (schedule.Schedule, error)
	Update(context.Context, schedule.Patch) (schedule.Schedule, error)
	Delete(context.Context, string) error
	RunNow(context.Context, string) (schedule.RunHandle, error)
}

type AgentMemory interface {
	Items(context.Context, agent.MemoryTarget) ([]agent.MemoryItem, error)
	Review(context.Context, string, agent.MemoryReviewDecision) error
	Update(context.Context, agent.MemoryPatch) (agent.MemoryItem, error)
	Delete(context.Context, string) error
	Add(context.Context, agent.MemoryTarget, string) (agent.MemoryItem, error)
}

type Knowledge interface {
	Entries(context.Context, string) ([]workspace.KnowledgeEntry, error)
	Document(context.Context, workspace.KnowledgeTarget) (workspace.KnowledgeEntry, error)
	Save(context.Context, workspace.KnowledgeUpdate) (workspace.KnowledgeEntry, error)
}

type DiagnosticTools interface {
	Tools(context.Context) ([]workspace.DiagnosticToolDescriptor, error)
	Invoke(context.Context, workspace.DiagnosticToolInvocation) (workspace.DiagnosticToolResult, error)
}

type AuthoringContext interface {
	Documents(context.Context, string) ([]workspace.AuthoringDocument, error)
	Recipes(context.Context, string) ([]workspace.AuthoringRecipe, error)
}

type Hooks interface {
	Catalog(context.Context, string) (workspace.HookCatalog, error)
	SetProjectTrust(context.Context, string, bool) error
}

type Feedback interface {
	Record(context.Context, agent.FeedbackSignal) error
}

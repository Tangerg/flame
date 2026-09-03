package terminal

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/cli/internal/application/integration/models"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
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
	GetApprovalMode(context.Context) (protocol.ApprovalMode, error)
	SetApprovalMode(context.Context, protocol.ApprovalMode) (protocol.ApprovalMode, error)
	ListApprovalRules(context.Context, string) ([]protocol.ApprovalRule, error)
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

type ModelConfiguration interface {
	Roles(context.Context) (models.Roles, error)
	SetRole(context.Context, models.Role) (models.Role, error)
	Providers(context.Context) ([]models.Provider, error)
	UpdateProvider(context.Context, models.UpdateProvider) (models.Provider, error)
	TestProvider(context.Context, string) (models.TestResult, error)
}

type MCPManagement interface {
	Servers(context.Context) ([]mcp.Server, error)
	CreateServer(context.Context, mcp.Candidate) (mcp.Server, error)
	UpdateServer(context.Context, mcp.ServerUpdate) (mcp.Server, error)
	DeleteServer(context.Context, string) error
	TestServer(context.Context, mcp.Candidate) (mcp.TestResult, error)
	Tools(context.Context, string) ([]mcp.Tool, error)
	ReconnectServer(context.Context, string) error
	StartAuthorization(context.Context, string) (protocol.MCPAuthorizationAttempt, error)
	GetAuthorization(context.Context, mcp.AuthorizationReference) (protocol.MCPAuthorizationAttempt, error)
}

type Goals interface {
	GetGoal(context.Context, string) (protocol.Goal, bool, error)
	StartGoal(context.Context, protocol.StartGoalRequest) (protocol.Goal, error)
	UpdateGoal(context.Context, protocol.UpdateGoalRequest) (protocol.Goal, error)
	ClearGoal(context.Context, string) error
	StopGoal(context.Context, string) (protocol.Goal, error)
	ResumeGoal(context.Context, string) (protocol.Goal, error)
}

type Skills interface {
	Discover(context.Context, string) ([]protocol.Skill, error)
	Managed(context.Context) ([]protocol.ManagedSkill, error)
	Proposals(context.Context, string) ([]workspace.SkillProposal, error)
	Archive(context.Context, string) error
	Restore(context.Context, string) error
	Approve(context.Context, workspace.SkillProposalReference) error
	Reject(context.Context, workspace.SkillProposalReference) error
}

type Schedules interface {
	Schedules(context.Context) ([]protocol.Schedule, error)
	Create(context.Context, protocol.CreateScheduleRequest) (protocol.Schedule, error)
	Update(context.Context, protocol.UpdateScheduleRequest) (protocol.Schedule, error)
	Delete(context.Context, string) error
	RunNow(context.Context, string) (protocol.RunScheduleNowResponse, error)
}

type AgentMemory interface {
	Items(context.Context, agent.MemoryTarget) ([]protocol.AgentMemoryItem, error)
	Review(context.Context, string, protocol.AgentMemoryReviewDecision) error
	Update(context.Context, protocol.AgentMemoryUpdateRequest) (protocol.AgentMemoryItem, error)
	Delete(context.Context, string) error
	Add(context.Context, agent.MemoryTarget, string) (protocol.AgentMemoryItem, error)
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
	Documents(context.Context, string) ([]protocol.AgentDoc, error)
	Recipes(context.Context, string) ([]workspace.AuthoringRecipe, error)
}

type Hooks interface {
	Catalog(context.Context, string) (workspace.HookCatalog, error)
	SetProjectTrust(context.Context, string, bool) error
}

type Feedback interface {
	Record(context.Context, protocol.FeedbackRequest) error
}

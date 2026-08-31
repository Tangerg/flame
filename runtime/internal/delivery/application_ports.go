package delivery

import (
	"context"
	"io"
	"time"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/models"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/application/runs"
	"github.com/Tangerg/flame/runtime/internal/application/schedules"
	"github.com/Tangerg/flame/runtime/internal/application/sessions"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/knowledge"
	"github.com/Tangerg/flame/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/plan"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	toolsvc "github.com/Tangerg/flame/runtime/internal/domain/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/transcript"
)

// Every interface below is defined by Delivery — the consuming side. They keep
// the protocol boundary dependent on exactly the use cases it drives.

type sessionUseCases interface {
	CreateView(ctx context.Context, title, cwd string) (sessions.View, error)
	DeleteSession(ctx context.Context, sessionID string) error
	ForkView(ctx context.Context, spec sessions.ForkSpec) (sessions.View, error)
	ListViewPage(ctx context.Context, filter session.CatalogFilter, cursor string, limit pagination.RequestedLimit) (pagination.Page[sessions.View], error)
	ExportSession(ctx context.Context, sessionID string) (sessions.ExportResult, error)
	MaterialSnapshot(ctx context.Context, sessionID string) (sessions.MaterialSnapshot, error)
	RestorePortableSession(ctx context.Context, snapshot sessions.PortableSnapshot) (sessions.View, error)
	Rollback(ctx context.Context, spec sessions.RollbackSpec) (sessions.RollbackResult, error)
	UpdateView(ctx context.Context, id string, patch sessions.Patch) (sessions.View, error)
	View(ctx context.Context, id string) (sessions.View, error)
}

type mcpUseCases interface {
	CreateAuthorizationAttempt(ctx context.Context, name mcpserver.ServerName) (mcpapp.AuthorizationAttempt, error)
	CreateServer(ctx context.Context, input mcpapp.ServerInput) (mcpapp.Server, error)
	DeleteServer(ctx context.Context, name mcpserver.ServerName) error
	AuthorizationAttempt(ctx context.Context, id string) (mcpapp.AuthorizationAttempt, error)
	AuthorizationAttemptRetention() time.Duration
	Servers(ctx context.Context) ([]mcpapp.Server, error)
	Tools(ctx context.Context, server *mcpserver.ServerName) ([]mcpserver.AdvertisedTool, error)
	ReconnectServer(ctx context.Context, name mcpserver.ServerName) error
	TestServer(ctx context.Context, input mcpapp.ServerInput) (mcpapp.TestResult, error)
	UpdateServer(ctx context.Context, name mcpserver.ServerName, patch mcpapp.ServerPatch) (mcpapp.Server, error)
}

type approvalUseCases interface {
	ForgetRule(ctx context.Context, id string) error
	ListRules(ctx context.Context, sessionID string) ([]approval.Rule, error)
	DefaultMode(ctx context.Context) (approval.Mode, error)
	SetDefaultMode(ctx context.Context, mode approval.Mode) error
}

type modelUseCases interface {
	UpdateProvider(ctx context.Context, cmd models.UpdateProviderCommand) (models.ProviderSummary, error)
	EmbeddingRole() modelref.Selection
	ListModels(ctx context.Context, providerID string) ([]models.Model, error)
	ListProviders(ctx context.Context) ([]models.ProviderSummary, error)
	SetEmbeddingRole(ctx context.Context, providerID, model string) (modelref.Selection, error)
	SetUtilityRole(ctx context.Context, provider, model string) (modelref.Selection, error)
	TestProvider(ctx context.Context, id string) (models.ProviderTestOutcome, error)
	UtilityRole() modelref.Selection
}

type toolUseCases interface {
	Invoke(ctx context.Context, in workspaceapp.DiagnosticToolInvocation) (toolsvc.Result, error)
	List(ctx context.Context) ([]toolsvc.Tool, error)
}

type runUseCases interface {
	Cancel(ctx context.Context, cmd runs.CancelCommand) (runs.CancelResult, error)
	Resume(ctx context.Context, cmd runs.ResumeCommand) (runs.StartResult, error)
	Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error)
	Steer(ctx context.Context, cmd runs.SteerCommand) error
	Subscribe(ctx context.Context, req runs.SubscribeRequest) (runs.Subscription, error)
	// ReplayRetention is what discovery publishes. Reading it from the enforcer is
	// the point: a limit the client is told and a limit the runtime evicts by must
	// be one number, or discovery is describing a runtime that does not exist.
	ReplayRetention() runs.Retention
}

type queryUseCases interface {
	ListItemPage(ctx context.Context, scope sessions.ItemScope, order transcript.SequenceOrder, cursor string, limit pagination.RequestedLimit) (sessions.ItemPage, error)
	ListPendingInterruptPage(ctx context.Context, sessionID, rootRunID string, caller run.Capabilities, cursor string, limit pagination.RequestedLimit) (pagination.Page[runs.Pending], error)
	Run(ctx context.Context, runID string) (run.Run, bool, error)
	PlanState(ctx context.Context, sessionID string) (plan.Current, error)
	ListRunPage(ctx context.Context, filter sessions.RunPageFilter, cursor string, limit pagination.RequestedLimit) (pagination.Page[run.Run], error)
}

type usageUseCases interface {
	Session(ctx context.Context, sessionID string) (sessions.SessionUsageReport, error)
	Summary(ctx context.Context, period sessions.UsageSummaryPeriod) (sessions.UsageSummary, error)
}

type feedbackUseCases interface {
	Record(ctx context.Context, command sessions.FeedbackCommand) error
}

type scheduleManagementUseCases interface {
	Available() bool
	Create(ctx context.Context, cmd schedules.CreateCommand) (schedule.Schedule, error)
	Delete(ctx context.Context, id string) error
	ListPage(ctx context.Context, cursor string, limit pagination.RequestedLimit) (pagination.Page[schedule.Schedule], error)
	Update(ctx context.Context, cmd schedules.UpdateCommand) (schedule.Schedule, error)
}

type scheduleFiringUseCases interface {
	Available() bool
	RunNow(ctx context.Context, id string) (schedules.StartedRun, error)
}

type workspaceFileUseCases interface {
	Head(ctx context.Context, cwd, path string, lines workspaceapp.HeadLineLimit) (workspaceapp.FileHead, error)
	Grep(ctx context.Context, cwd string, input workspaceapp.GrepInput) (workspaceapp.GrepResult, error)
	List(ctx context.Context, input workspaceapp.FileListInput) (workspaceapp.FilePage, error)
	Read(ctx context.Context, cwd string, input workspaceapp.FileReadInput) (workspaceapp.FileReadResult, error)
}

type workspaceVCSUseCases interface {
	Diff(ctx context.Context, input workspaceapp.DiffInput) (workspaceapp.Diff, error)
	Changes(ctx context.Context, cwd string) ([]workspaceapp.FileChange, error)
}

type workspaceDiscoveryUseCases interface {
	AgentDocs(ctx context.Context, cwd string) ([]workspaceapp.AgentDoc, error)
	Workspaces(ctx context.Context) ([]workspaceapp.Summary, error)
	Recipes(ctx context.Context, cwd string) ([]workspaceapp.Recipe, error)
	Resolve(path string) (workspaceapp.Resolved, error)
}

type workspaceKnowledgeUseCases interface {
	Available() bool
	Entries(ctx context.Context, cwd string) ([]knowledge.Entry, error)
	Read(ctx context.Context, scope knowledge.Scope, cwd string) (knowledge.Entry, error)
	Update(ctx context.Context, scope knowledge.Scope, cwd, expectedRevision, content string) (knowledge.Entry, error)
}

type workspaceSkillUseCases interface {
	Archive(ctx context.Context, name string) error
	Managed(ctx context.Context) ([]skills.Entry, error)
	Proposals(ctx context.Context, cwd string) ([]skills.ProposalReview, error)
	List(ctx context.Context, cwd string) ([]workspaceapp.SkillSummary, error)
	ApproveProposal(ctx context.Context, cwd string, ref skills.ProposalRef) error
	RejectProposal(ctx context.Context, cwd string, ref skills.ProposalRef) error
	Restore(ctx context.Context, name string) error
}

type workspaceHookUseCases interface {
	Inspect(ctx context.Context, cwd string) (workspaceapp.HookInspection, error)
	SetProjectTrust(ctx context.Context, projectRoot string, trusted bool) error
}

type workspaceWatchUseCases interface {
	Available() bool
	Watch(cwds []string, notify func()) (io.Closer, error)
}

type workspaceAuthoredWatchUseCases interface {
	Watch(
		cwds []string,
		resources []workspaceapp.AuthoredResource,
		notify func(workspaceapp.AuthoredResource),
	) (workspaceapp.AuthoredObservation, error)
}

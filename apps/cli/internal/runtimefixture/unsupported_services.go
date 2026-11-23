package runtimefixture

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/apps/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/apps/cli/internal/application/integration/models"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// The fixture Runtime serves Sessions and Runs only. Every other Runtime
// service refuses, and no change-feed topic is supported.
var errUnsupported = errors.ErrUnsupported

func (*Runtime) Changes(context.Context, string) (_ []workspace.Change, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Diff(context.Context, workspace.DiffRequest) (_ workspace.Diff, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Files(context.Context, workspace.FilesRequest) (_ workspace.FileListing, err error) {
	err = errUnsupported
	return
}

func (*Runtime) List(context.Context) (_ []workspace.Summary, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Read(context.Context, workspace.ReadRequest) (_ workspace.FileContent, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Resolve(context.Context, workspace.ResolveRequest) (_ workspace.Workspace, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Search(context.Context, workspace.SearchRequest) (_ workspace.SearchResult, err error) {
	err = errUnsupported
	return
}

func (*Runtime) SessionUsage(context.Context, string) (_ conversation.SessionUsageReport, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Summary(context.Context, conversation.UsageSummaryPeriod) (_ conversation.UsageSummary, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Providers(context.Context) (_ []models.Provider, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Roles(context.Context) (_ models.Roles, err error) {
	err = errUnsupported
	return
}

func (*Runtime) SetRole(context.Context, models.Role) (_ models.Role, err error) {
	err = errUnsupported
	return
}

func (*Runtime) TestProvider(context.Context, string) (_ protocol.ProviderTestOutcome, err error) {
	err = errUnsupported
	return
}

func (*Runtime) UpdateProvider(context.Context, models.UpdateProvider) (_ models.Provider, err error) {
	err = errUnsupported
	return
}

func (*Runtime) ClearGoal(context.Context, string) (err error) {
	err = errUnsupported
	return
}

func (*Runtime) GetGoal(context.Context, string) (_ protocol.Goal, _ bool, err error) {
	err = errUnsupported
	return
}

func (*Runtime) ResumeGoal(context.Context, string) (_ protocol.Goal, err error) {
	err = errUnsupported
	return
}

func (*Runtime) StartGoal(context.Context, protocol.StartGoalRequest) (_ protocol.Goal, err error) {
	err = errUnsupported
	return
}

func (*Runtime) StopGoal(context.Context, string) (_ protocol.Goal, err error) {
	err = errUnsupported
	return
}

func (*Runtime) UpdateGoal(context.Context, protocol.UpdateGoalRequest) (_ protocol.Goal, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Approve(context.Context, workspace.SkillProposalReference) (err error) {
	err = errUnsupported
	return
}

func (*Runtime) Archive(context.Context, string) (err error) {
	err = errUnsupported
	return
}

func (*Runtime) Discover(context.Context, string) (_ protocol.SkillDiscovery, err error) {
	err = errUnsupported
	return
}

func (*Runtime) InspectSkill(context.Context, string, string) (_ protocol.SkillDetail, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Managed(context.Context) (_ []protocol.ManagedSkill, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Proposals(context.Context, string) (_ []workspace.SkillProposal, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Reject(context.Context, workspace.SkillProposalReference) (err error) {
	err = errUnsupported
	return
}

func (*Runtime) Restore(context.Context, string) (err error) {
	err = errUnsupported
	return
}

func (*Runtime) CreateServer(context.Context, authoring.CommandID, mcp.Candidate) (_ protocol.MCPServer, err error) {
	err = errUnsupported
	return
}

func (*Runtime) DeleteServer(context.Context, protocol.MCPServerID) (err error) {
	err = errUnsupported
	return
}

func (*Runtime) GetAuthorization(context.Context, mcp.AuthorizationReference) (_ protocol.MCPAuthorizationAttempt, err error) {
	err = errUnsupported
	return
}

func (*Runtime) ReconnectServer(context.Context, protocol.MCPServerID) (err error) {
	err = errUnsupported
	return
}

func (*Runtime) Servers(context.Context) (_ []protocol.MCPServer, err error) {
	err = errUnsupported
	return
}

func (*Runtime) SetToolExposure(context.Context, protocol.SetMCPToolExposureRequest) (err error) {
	err = errUnsupported
	return
}

func (*Runtime) StartAuthorization(context.Context, protocol.MCPServerID) (_ protocol.MCPAuthorizationAttempt, err error) {
	err = errUnsupported
	return
}

func (*Runtime) TestServer(context.Context, mcp.Candidate) (_ protocol.MCPTestOutcome, err error) {
	err = errUnsupported
	return
}

func (*Runtime) ToolExposure(context.Context, protocol.MCPServerID) (_ protocol.MCPToolExposure, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Tools(context.Context, *protocol.MCPServerID) (_ []protocol.MCPTool, err error) {
	err = errUnsupported
	return
}

func (*Runtime) UpdateServer(context.Context, authoring.CommandID, mcp.ServerUpdate) (_ protocol.MCPServer, err error) {
	err = errUnsupported
	return
}

func (*Runtime) CallPlugin(context.Context, protocol.CallPluginRequest, authoring.CommandID) (_ *protocol.CallPluginResponse, err error) {
	err = errUnsupported
	return
}

func (*Runtime) ListPlugins(context.Context) (_ []protocol.PluginInstallation, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Documents(context.Context, string) (_ []protocol.AgentDoc, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Catalog(context.Context, string) (_ workspace.HookCatalog, err error) {
	err = errUnsupported
	return
}

func (*Runtime) SetProjectTrust(context.Context, string, bool) (err error) {
	err = errUnsupported
	return
}

func (*Runtime) Record(context.Context, protocol.FeedbackRequest) (err error) {
	err = errUnsupported
	return
}

func (*Runtime) Subscribe(context.Context, changefeed.Subscription) (_ changefeed.EventStream, err error) {
	err = errUnsupported
	return
}

func (*Runtime) Supports(protocol.RuntimeTopic) (_ bool) {
	return
}

func (*Runtime) ExportSession(context.Context, session.ExportRequest) (_ session.Document, err error) {
	err = errUnsupported
	return
}

func (*Runtime) ImportSession(context.Context, session.ImportRequest) (_ conversation.Session, err error) {
	err = errUnsupported
	return
}

// Diagnostic and MCP tool listings share a method name, so diagnostics
// refuse through their own receiver.
type UnsupportedDiagnostics struct{}

func (UnsupportedDiagnostics) Tools(context.Context) ([]workspace.DiagnosticToolDescriptor, error) {
	return nil, errUnsupported
}

func (UnsupportedDiagnostics) Invoke(context.Context, workspace.DiagnosticToolInvocation) (workspace.DiagnosticToolResult, error) {
	return workspace.DiagnosticToolResult{}, errUnsupported
}

package terminal

import (
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
	"github.com/Tangerg/flame/apps/cli/internal/application/integration/models"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	commandCategoryApplication = "Application"
	commandCategoryTranscript  = "Transcript"
	commandCategorySessions    = "Sessions"
	commandCategoryComposer    = "Composer"
	commandCategoryRuntime     = "Runtime"
	commandCategoryContext     = "Context"
	commandCategoryConnections = "Connections"
	commandCategoryWorkspace   = "Workspace"
	commandCategoryExtensions  = "Extensions"
)

func builtinCommands() []localCommand {
	return slices.Concat(
		commandGroup(commandCategoryApplication,
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "quit", Title: "leave flame", Aliases: []string{"exit"}}, Run: func(a *app, _ string) error { a.loop.Quit(); return nil }},
		),
		commandGroup(commandCategoryTranscript,
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "help", Title: "show commands available in this session"}, Run: func(a *app, _ string) error { a.showCommandPalette(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "shortcuts", Title: "show all keyboard shortcuts"}, Run: func(a *app, _ string) error { a.ShowShortcuts(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "clear", Title: "release the live transcript"}, Available: availableWithoutActiveRun, Run: func(a *app, _ string) error { a.Clear(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "find", Title: "find text in the live transcript", Arguments: extensions.RequiredArguments}, Run: func(a *app, query string) error { a.search.find(query); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "next", Title: "step to the next search match"}, Run: func(a *app, _ string) error { a.search.step(1); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "previous", Title: "step to the previous search match", Aliases: []string{"prev"}}, Run: func(a *app, _ string) error { a.search.step(-1); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "queue", Title: "manage follow-ups waiting behind the current run"}, Run: func(a *app, _ string) error { a.queueEditor.ShowQueue(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "details", Title: "expand or collapse tool output and diffs"}, Run: func(a *app, _ string) error { a.ToggleToolDetails(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "view", Title: "open the selected transcript entry in the full reader"}, Available: availableWithReadableSelection, Run: func(a *app, _ string) error { a.OpenReader(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "copy-last", Title: "copy the latest durable assistant response"}, Run: func(a *app, _ string) error { return a.copyLastAssistant() }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "export", Title: "export a runtime-native session document", Arguments: extensions.RequiredArguments}, Available: availableWithSessionTransfer, Run: func(a *app, argument string) error { return a.lifecycle.exportSession(argument) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "feedback", Title: "rate the latest assistant response", Arguments: extensions.RequiredArguments}, Available: availableWithFeedback, Run: func(a *app, argument string) error { return a.screens.feedback.RecordFeedback(argument) }},
		),
		commandGroup(commandCategorySessions,
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "sessions", Title: "search and switch sessions", Aliases: []string{"resume"}}, Available: availableWithoutActiveRun, Run: func(a *app, _ string) error { a.sessions.ShowSessions(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "timeline", Title: "browse runs and live subagents in the current session"}, Run: func(a *app, _ string) error { a.ShowTimeline(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "new", Title: "start a new session"}, Available: availableWithoutActiveRun, Run: func(a *app, _ string) error { a.lifecycle.NewSession(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "workspace", Title: "start a session in a recent or specified workspace", Arguments: extensions.OptionalArguments}, Available: availableWithoutActiveRun, Run: func(a *app, path string) error { return a.lifecycle.chooseWorkspace(path) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "relocate", Title: "move the current session to another workspace", Arguments: extensions.RequiredArguments}, Available: availableForRelocation, Run: func(a *app, path string) error { return a.lifecycle.RelocateSession(path) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "rename", Title: "rename the current session", Arguments: extensions.RequiredArguments}, Available: availableWithoutActiveRun, Run: func(a *app, title string) error { a.lifecycle.RenameSession(title); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "fork", Title: "fork the complete current session", Arguments: extensions.OptionalArguments}, Available: availableWithoutActiveRun, Run: func(a *app, title string) error { a.lifecycle.ForkSession(title); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "rollback", Title: "rewind history, files, or both to a run boundary", Arguments: extensions.RequiredArguments}, Available: availableForRollback, Run: func(a *app, argument string) error { return a.lifecycle.prepareSessionRollback(argument) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "import", Title: "import and open a runtime-native JSON session artifact", Arguments: extensions.RequiredArguments}, Available: availableWithSessionTransfer, Run: func(a *app, path string) error { return a.lifecycle.prepareSessionImport(path) }},
		),
		commandGroup(commandCategoryComposer,
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "attach", Title: "attach a local file to the next prompt", Arguments: extensions.RequiredArguments}, Run: func(a *app, path string) error { return a.composer.addAttachment(path) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "detach", Title: "remove an attachment by name, number, or all", Arguments: extensions.RequiredArguments}, Run: func(a *app, value string) error { return a.composer.removeAttachment(value) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "attachments", Title: "show files attached to the next prompt", Aliases: []string{"files"}}, Run: func(a *app, _ string) error { a.composer.showAttachments(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "stash", Title: "stash the current prompt and clear the composer"}, Available: availableWithDraft, Run: func(a *app, _ string) error { return a.composer.stashPrompt() }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "stashes", Title: "list saved prompt stashes"}, Run: func(a *app, _ string) error { a.composer.showPromptStashes(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "stash-apply", Title: "apply a prompt stash by id or unique prefix", Arguments: extensions.RequiredArguments}, Available: availableWithEmptyDraft, Run: func(a *app, id string) error { return a.composer.applyPromptStash(id) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "stash-delete", Title: "delete a prompt stash by id or unique prefix", Arguments: extensions.RequiredArguments}, Run: func(a *app, id string) error { return a.composer.deletePromptStash(id) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "editor", Title: "edit the current prompt in the configured external editor"}, Available: availableWithoutActiveRun, Run: func(a *app, _ string) error { return a.composer.editExternally() }},
		),
		commandGroup(commandCategoryRuntime,
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "tools", Title: "inspect direct read-only diagnostic tools"}, Available: availableWithDiagnosticTools, Run: func(a *app, _ string) error { a.screens.diagnosticTools.ShowDiagnosticTools(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "tool-invoke", Title: "invoke a direct read-only diagnostic tool", Arguments: extensions.RequiredArguments}, Available: availableWithDiagnosticTools, Run: func(a *app, argument string) error { return a.screens.diagnosticTools.InvokeDiagnosticTool(argument) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "model", Title: "choose the current session model"}, Run: func(a *app, _ string) error { a.screens.options.ChooseModel(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "models", Title: "inspect runtime model capabilities and pricing"}, Run: func(a *app, _ string) error { a.screens.options.ShowModels(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "usage", Title: "inspect session and runtime usage", Arguments: extensions.OptionalArguments}, Available: availableWithUsage, Run: func(a *app, days string) error { return a.screens.usage.ShowUsage(days) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "roles", Title: "inspect utility and embedding model roles"}, Available: availableWithModelConfiguration, Run: func(a *app, _ string) error { a.screens.modelConfig.ShowModelRoles(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "utility", Title: "set the utility model role", Arguments: extensions.RequiredArguments}, Available: availableWithModelConfiguration, Run: func(a *app, target string) error {
				return a.screens.modelConfig.SetModelRole(models.UtilityRole, target)
			}},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "embedding", Title: "set the embedding model role", Arguments: extensions.RequiredArguments}, Available: availableWithModelConfiguration, Run: func(a *app, target string) error {
				return a.screens.modelConfig.SetModelRole(models.EmbeddingRole, target)
			}},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "providers", Title: "inspect configured model providers"}, Available: availableWithModelConfiguration, Run: func(a *app, _ string) error { a.screens.modelConfig.ShowProviders(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "provider-test", Title: "test a configured provider", Arguments: extensions.RequiredArguments}, Available: availableWithModelConfiguration, Run: func(a *app, provider string) error { return a.screens.modelConfig.TestConfiguredProvider(provider) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "provider-config", Title: "configure provider endpoint and credentials", Arguments: extensions.RequiredArguments}, Available: availableWithModelConfiguration, Run: func(a *app, provider string) error { return a.screens.modelConfig.ConfigureProvider(provider) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "approval", Title: "choose the runtime approval mode", Aliases: []string{"permissions", "permission"}}, Run: func(a *app, _ string) error { a.screens.approvals.ChooseApprovalMode(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "status", Title: "show runtime and run policy"}, Run: func(a *app, _ string) error { a.screens.options.ShowRuntimeStatus(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "rules", Title: "show remembered approval rules"}, Run: func(a *app, _ string) error { a.screens.approvals.ShowApprovalRules(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "rule-delete", Title: "forget a remembered approval rule", Arguments: extensions.RequiredArguments}, Run: func(a *app, id string) error { return a.screens.approvals.PrepareDeleteApprovalRule(id) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "steer", Title: "inject an instruction into the observed run segment", Arguments: extensions.RequiredArguments}, Available: availableWithRunningSegment, Run: func(a *app, instruction string) error { return a.steering.steerRun(instruction) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "goal", Title: "inspect the current autonomous session goal"}, Available: availableWithGoals, Run: func(a *app, _ string) error { a.screens.goals.ShowGoal(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "goal-start", Title: "start autonomous pursuit for this session", Arguments: extensions.RequiredArguments}, Available: availableForGoalStart, Run: func(a *app, objective string) error { return a.screens.goals.StartGoal(objective) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "goal-update", Title: "revise the current autonomous objective", Arguments: extensions.RequiredArguments}, Available: availableWithGoals, Run: func(a *app, objective string) error { return a.screens.goals.UpdateGoal(objective) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "goal-clear", Title: "clear autonomous pursuit for this session"}, Available: availableWithGoals, Run: func(a *app, _ string) error { return a.screens.goals.ClearGoal() }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "goal-stop", Title: "pause autonomous pursuit for this session"}, Available: availableWithGoals, Run: func(a *app, _ string) error { return a.screens.goals.StopGoal() }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "goal-resume", Title: "resume autonomous pursuit for this session"}, Available: availableWithGoals, Run: func(a *app, _ string) error { return a.screens.goals.ResumeGoal() }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "hooks", Title: "audit lifecycle hooks and project trust"}, Available: availableWithHooks, Run: func(a *app, _ string) error { a.screens.hooks.ShowHooks(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "hooks-trust", Title: "trust reviewed project lifecycle hooks"}, Available: availableWithHooks, Run: func(a *app, _ string) error { return a.screens.hooks.PrepareHookTrust(true) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "hooks-revoke", Title: "revoke project lifecycle hook trust"}, Available: availableWithHooks, Run: func(a *app, _ string) error { return a.screens.hooks.PrepareHookTrust(false) }},
		),
		commandGroup(commandCategoryContext,
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "agent-docs", Title: "inspect applicable AGENTS.md documents"}, Available: availableWithAuthoringContext, Run: func(a *app, _ string) error { a.screens.authoringContext.ShowAgentDocuments(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "skills", Title: "inspect available skills or one named skill", Arguments: extensions.OptionalArguments}, Available: availableWithSkills, Run: func(a *app, name string) error { a.screens.skills.ShowDiscoveredSkills(name); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "skill-library", Title: "inspect active and archived managed skills"}, Available: availableWithSkills, Run: func(a *app, _ string) error { a.screens.skills.ShowManagedSkills(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "skill-proposals", Title: "review pending immutable Skill proposals"}, Available: availableWithSkills, Run: func(a *app, _ string) error { a.screens.skills.ShowSkillProposals(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "skill-archive", Title: "archive one managed Skill", Arguments: extensions.RequiredArguments}, Available: availableWithSkills, Run: func(a *app, name string) error { return a.screens.skills.ArchiveSkill(name) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "skill-restore", Title: "restore one archived managed Skill", Arguments: extensions.RequiredArguments}, Available: availableWithSkills, Run: func(a *app, name string) error { return a.screens.skills.RestoreSkill(name) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "skill-approve", Title: "approve an exact pending Skill proposal", Arguments: extensions.RequiredArguments}, Available: availableWithSkills, Run: func(a *app, identity string) error {
				return a.screens.skills.PrepareSkillProposalDecision(identity, true)
			}},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "skill-reject", Title: "reject an exact pending Skill proposal", Arguments: extensions.RequiredArguments}, Available: availableWithSkills, Run: func(a *app, identity string) error {
				return a.screens.skills.PrepareSkillProposalDecision(identity, false)
			}},
		),
		commandGroup(commandCategoryConnections,
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "mcp-tool", Title: "set one MCP tool's exposure or standing decision", Arguments: extensions.RequiredArguments}, Available: availableWithMCP, Run: func(a *app, args string) error { return a.screens.mcp.ConfigureMCPTool(args) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "mcp", Title: "inspect configured MCP servers and live state"}, Available: availableWithMCP, Run: func(a *app, _ string) error { a.screens.mcp.ShowMCPServers(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "mcp-tools", Title: "inspect MCP tools, optionally for one server", Arguments: extensions.OptionalArguments}, Available: availableWithMCP, Run: func(a *app, server string) error { a.screens.mcp.ShowMCPTools(server); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "mcp-create", Title: "configure a new MCP server"}, Available: availableWithMCP, Run: func(a *app, _ string) error { return a.screens.mcp.OpenMCPCreateForm() }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "mcp-edit", Title: "update an MCP server", Arguments: extensions.RequiredArguments}, Available: availableWithMCP, Run: func(a *app, server string) error { return a.screens.mcp.EditMCPServer(server) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "mcp-probe", Title: "test an unpersisted MCP candidate"}, Available: availableWithMCP, Run: func(a *app, _ string) error { return a.screens.mcp.OpenMCPProbeForm() }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "mcp-delete", Title: "delete an MCP server", Arguments: extensions.RequiredArguments}, Available: availableWithMCP, Run: func(a *app, server string) error { return a.screens.mcp.PrepareDeleteMCPServer(server) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "mcp-reconnect", Title: "request an asynchronous MCP reconnect", Arguments: extensions.RequiredArguments}, Available: availableWithMCP, Run: func(a *app, server string) error { return a.screens.mcp.ReconnectMCPServer(server) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "mcp-auth", Title: "start and observe MCP browser authorization", Arguments: extensions.RequiredArguments}, Available: availableWithMCP, Run: func(a *app, server string) error { return a.screens.mcp.AuthorizeMCPServer(server) }},
		),
		commandGroup(commandCategoryWorkspace,
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "workspaces", Title: "inspect runtime-known workspaces"}, Available: availableWithWorkspaceService, Run: func(a *app, _ string) error { a.screens.inspection.ShowWorkspaces(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "changes", Title: "inspect authoritative workspace changes"}, Available: availableWithGitWorkspaceService, Run: func(a *app, _ string) error { a.screens.inspection.ShowWorkspaceChanges(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "diff", Title: "inspect workspace changes; supports --base, --rows, and --limit", Arguments: extensions.OptionalArguments}, Available: availableWithGitWorkspaceService, Run: func(a *app, argument string) error { return a.screens.inspection.ShowWorkspaceDiff(argument) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "preview", Title: "preview a file; supports --lines", Arguments: extensions.RequiredArguments}, Available: availableWithWorkspaceService, Run: func(a *app, argument string) error { return a.screens.inspection.PreviewWorkspaceFile(argument) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "grep", Title: "search files; supports --path and --limit", Arguments: extensions.RequiredArguments}, Available: availableWithWorkspaceService, Run: func(a *app, argument string) error { return a.screens.inspection.SearchWorkspace(argument) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "browse", Title: "browse files; supports --recursive, --ignored, and --glob", Arguments: extensions.OptionalArguments}, Available: availableWithWorkspaceService, Run: func(a *app, argument string) error { return a.screens.inspection.BrowseWorkspace(argument) }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "read", Title: "read a file; supports --start, --end, and --max-bytes", Arguments: extensions.RequiredArguments}, Available: availableWithWorkspaceService, Run: func(a *app, argument string) error { return a.screens.inspection.ReadWorkspaceFile(argument) }},
		),
		commandGroup(commandCategoryExtensions,
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "plugins", Title: "inspect Runtime-installed plugins and release state"}, Available: availableWithPlugins, Run: func(a *app, _ string) error { a.screens.plugins.ShowPlugins(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "extensions", Title: "show local command extensions and lifecycle state"}, Run: func(a *app, _ string) error { a.extensions.ShowExtensions(); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "reload", Title: "reload a local command extension and its dependents", Arguments: extensions.RequiredArguments}, Run: func(a *app, id string) error { a.extensions.ReloadPlugin(id); return nil }},
			localCommand{Descriptor: extensions.CommandDescriptor{Name: "unload", Title: "unload a local command extension and its dependents", Arguments: extensions.RequiredArguments}, Run: func(a *app, id string) error { a.extensions.UnloadPlugin(id); return nil }},
		),
	)
}

func availableWithWorkspaceService(a *app) extensions.CommandAvailability {
	if a.workspaces == nil {
		return extensions.CommandUnavailable("this runtime composition has no workspace service")
	}
	return extensions.CommandAvailable()
}

func availableWithGitWorkspaceService(a *app) extensions.CommandAvailability {
	if unavailable := availableWithWorkspaceService(a); !unavailable.Enabled() {
		return unavailable
	}
	return availableWithRuntimeFeature(a, protocol.FeatureGit)
}

func availableWithSessionTransfer(a *app) extensions.CommandAvailability {
	if unavailable := availableWithoutActiveRun(a); !unavailable.Enabled() {
		return unavailable
	}
	if a.lifecycle.transfers == nil {
		return extensions.CommandUnavailable("this runtime composition has no session transfer service")
	}
	return availableWithRuntimeFeature(a, protocol.FeatureSessionExport)
}

func availableWithUsage(a *app) extensions.CommandAvailability {
	if a.screens.usage == nil {
		return extensions.CommandUnavailable("this runtime composition has no usage service")
	}
	return extensions.CommandAvailable()
}

func availableWithModelConfiguration(a *app) extensions.CommandAvailability {
	if a.screens.modelConfig == nil {
		return extensions.CommandUnavailable("this runtime composition has no model configuration service")
	}
	return extensions.CommandAvailable()
}

func availableWithGoals(a *app) extensions.CommandAvailability {
	if a.screens.goals == nil {
		return extensions.CommandUnavailable("this runtime composition has no goal service")
	}
	return availableWithRuntimeFeature(a, protocol.FeatureGoals)
}

func availableWithSkills(a *app) extensions.CommandAvailability {
	if a.screens.skills == nil {
		return extensions.CommandUnavailable("this runtime composition has no skill service")
	}
	return availableWithRuntimeFeature(a, protocol.FeatureSkills)
}

func availableWithMCP(a *app) extensions.CommandAvailability {
	if a.screens.mcp == nil {
		return extensions.CommandUnavailable("this runtime composition has no MCP service")
	}
	return availableWithRuntimeFeature(a, protocol.FeatureMCP)
}

func availableWithPlugins(a *app) extensions.CommandAvailability {
	if a.screens.plugins == nil {
		return extensions.CommandUnavailable("this runtime composition has no plugin catalog")
	}
	return availableWithRuntimeFeature(a, protocol.FeaturePlugins)
}

func availableWithDiagnosticTools(a *app) extensions.CommandAvailability {
	if a.screens.diagnosticTools == nil {
		return extensions.CommandUnavailable("this runtime composition has no diagnostic tool service")
	}
	return extensions.CommandAvailable()
}

func availableWithAuthoringContext(a *app) extensions.CommandAvailability {
	if a.screens.authoringContext == nil {
		return extensions.CommandUnavailable("this runtime composition has no authoring context service")
	}
	return extensions.CommandAvailable()
}

func availableWithHooks(a *app) extensions.CommandAvailability {
	if a.screens.hooks == nil {
		return extensions.CommandUnavailable("this runtime composition has no hook service")
	}
	return extensions.CommandAvailable()
}

func availableWithFeedback(a *app) extensions.CommandAvailability {
	if a.screens.feedback == nil {
		return extensions.CommandUnavailable("this runtime composition has no feedback service")
	}
	return extensions.CommandAvailable()
}

func availableForGoalStart(a *app) extensions.CommandAvailability {
	if unavailable := availableWithoutActiveRun(a); !unavailable.Enabled() {
		return unavailable
	}
	return availableWithGoals(a)
}

func availableForRelocation(a *app) extensions.CommandAvailability {
	if unavailable := availableWithoutActiveRun(a); !unavailable.Enabled() {
		return unavailable
	}
	return availableWithRuntimeFeature(a, protocol.FeatureRelocate)
}

func availableForRollback(a *app) extensions.CommandAvailability {
	if unavailable := availableWithoutActiveRun(a); !unavailable.Enabled() {
		return unavailable
	}
	_, present, err := a.commandDraft()
	if err != nil {
		return extensions.CommandUnavailable(err.Error())
	}
	if present {
		return extensions.CommandUnavailable("stash or detach the current draft before rolling back")
	}
	return extensions.CommandAvailable()
}

func availableWithRunningSegment(a *app) extensions.CommandAvailability {
	if a.runs.conversation().Phase() != conversation.Running || a.runs.conversation().RunID() == "" || a.runs.conversation().SegmentID() == "" {
		return extensions.CommandUnavailable("no observed run segment is executing")
	}
	return extensions.CommandAvailable()
}

func commandGroup(category string, commands ...localCommand) []localCommand {
	for index := range commands {
		commands[index].Descriptor.Category = category
	}
	return commands
}

func availableWithoutActiveRun(a *app) extensions.CommandAvailability {
	if a.runs.blocksAdmission() {
		return extensions.CommandUnavailable("an active run owns this session")
	}
	return extensions.CommandAvailable()
}

func availableWithReadableSelection(a *app) extensions.CommandAvailability {
	if _, readable := a.transcript.readerTargetForSelected(); !readable {
		return extensions.CommandUnavailable("select a readable transcript entry first")
	}
	return extensions.CommandAvailable()
}

func availableWithDraft(a *app) extensions.CommandAvailability {
	_, present, err := a.commandDraft()
	if err != nil {
		return extensions.CommandUnavailable(err.Error())
	}
	if !present {
		return extensions.CommandUnavailable("the composer is empty")
	}
	return extensions.CommandAvailable()
}

func availableWithEmptyDraft(a *app) extensions.CommandAvailability {
	_, present, err := a.commandDraft()
	if err != nil {
		return extensions.CommandUnavailable(err.Error())
	}
	if present {
		return extensions.CommandUnavailable("stash or clear the current draft first")
	}
	return extensions.CommandAvailable()
}

func (a *app) Clear() {
	if a.runs.observing() {
		a.status.doing = "the active run owns the transcript"
		return
	}
	a.runs.clearPresentation()
	a.transcript.Reset()
	a.activity.Reset()
	a.status.Reset()
	a.status.note("cleared")
	a.header.SetUsage(a.runs.conversation().Usage())
}

func (a *app) ShowShortcuts() {
	a.shortcuts.show()
	a.status.note("keyboard shortcuts")
}

func (a *app) ToggleToolDetails() {
	a.transcript.ToggleDetails()
	a.message(a.transcript.DetailsLabel())
}

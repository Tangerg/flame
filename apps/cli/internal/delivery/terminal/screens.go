package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/kit"
)

type screenHost struct {
	tasks        taskRunner
	reader       *readerSurface
	status       *statusView
	message      func(string)
	confirm      func(title, question, action, content string, run func())
	session      *sessionState
	stack        *workspaceScene
	theme        kit.Theme
	glyphs       kit.Glyphs
	clipboard    program.Clipboard
	replayPolicy mutation.ReplayPolicy
}

type screens struct {
	options          *runOptionScreens
	feedback         *feedbackScreens
	approvals        *approvalScreens
	inspection       *workspaceScreens
	goals            *goalScreens
	modelConfig      *modelConfigScreens
	usage            *usageScreens
	mcp              *mcpScreens
	skills           *skillScreens
	hooks            *hookScreens
	diagnosticTools  *diagnosticScreens
	authoringContext *agentDocumentScreens
	plugins          *pluginScreens
}

func (a *app) screenHost(replayPolicy mutation.ReplayPolicy) screenHost {
	return screenHost{
		tasks: a.tasks, reader: a.reader, status: a.status, message: a.message,
		confirm: a.confirm.ask, session: &a.lifecycle.state,
		stack: &a.stack, theme: a.transcript.theme, glyphs: a.transcript.glyphs,
		clipboard: a.loop.Clipboard(), replayPolicy: replayPolicy,
	}
}

func (a *app) buildScreens(cfg appConfig) {
	host := a.screenHost(cfg.replayPolicy)
	a.screens.options = &runOptionScreens{
		screenHost: host, service: a.runtime, profile: a.runtimeProfile, options: &a.options, lifecycle: a.lifecycle,
		observing: a.runs.observing, displayOptions: a.displayOptions, presentOptions: a.presentOptions,
		appendNote: a.appendNote,
	}
	a.screens.options.buildModelPicker(host.theme, host.glyphs)
	a.screens.feedback = &feedbackScreens{screenHost: host, service: cfg.feedback, runs: a.runs}
	a.screens.approvals = newApprovalScreens(host, a.runtime, a.runs.observing)
	a.screens.inspection = &workspaceScreens{screenHost: host, service: a.workspaces}
	a.screens.goals = &goalScreens{screenHost: host, service: cfg.goals, options: &a.options, header: a.header}
	a.screens.modelConfig = &modelConfigScreens{screenHost: host, service: cfg.modelConfig}
	a.screens.usage = &usageScreens{screenHost: host, service: cfg.usage}
	a.screens.mcp = &mcpScreens{screenHost: host, service: cfg.mcp, setApprovalRule: a.screens.approvals.setApprovalRule}
	a.screens.skills = &skillScreens{screenHost: host, service: cfg.skills}
	a.screens.hooks = &hookScreens{screenHost: host, service: cfg.hooks}
	a.screens.diagnosticTools = &diagnosticScreens{screenHost: host, service: cfg.diagnosticTools}
	a.screens.plugins = &pluginScreens{screenHost: host, service: cfg.plugins, workbench: a.workbench, viewsChanged: func() {
		a.registerCommands()
		a.completion.refreshCompletion()
	}}
	a.screens.authoringContext = &agentDocumentScreens{screenHost: host, service: cfg.authoringContext}
}

func (a *app) loadPluginViews(affected bool) {
	if affected && a.runtimeSupports(protocol.FeaturePlugins) {
		a.screens.plugins.loadPluginViews()
	}
}

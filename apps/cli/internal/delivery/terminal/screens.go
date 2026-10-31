package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/program"
)

// screenHost is what a management screen uses of the terminal: background
// work, the reader, notices, confirmation, modal forms and the session it
// presents. A screen owns its Runtime port and its own state; it reaches the
// rest of the terminal only through these.
type screenHost struct {
	tasks        taskRunner
	reader       *readerSurface
	status       *statusView
	message      func(string)
	confirm      func(title, question, action, content string, run func())
	session      *sessionState
	stack        *headless.Stack
	theme        kit.Theme
	glyphs       kit.Glyphs
	clipboard    program.Clipboard
	replayPolicy mutation.ReplayPolicy
}

// screens are the management surfaces the terminal composes. A screen whose
// Runtime port is absent is nil, which command availability reports.
type screens struct {
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

func (a *app) screenHost() screenHost {
	return screenHost{
		tasks: a.tasks, reader: a.reader, status: a.status, message: a.message,
		confirm: a.confirmActionWithContent, session: &a.session,
		stack: &a.stack, theme: a.transcript.theme, glyphs: a.transcript.glyphs,
		clipboard: a.loop.Clipboard(), replayPolicy: a.replayPolicy,
	}
}

func (a *app) buildScreens(cfg appConfig) {
	host := a.screenHost()
	a.screens.approvals = newApprovalScreens(host, a.runtime, a.runs.observing)
	a.screens.inspection = &workspaceScreens{screenHost: host, service: a.workspaces}
	if cfg.goals != nil {
		a.screens.goals = &goalScreens{screenHost: host, service: cfg.goals, options: &a.options, header: a.header}
	}
	if cfg.modelConfig != nil {
		a.screens.modelConfig = &modelConfigScreens{screenHost: host, service: cfg.modelConfig}
	}
	if cfg.usage != nil {
		a.screens.usage = &usageScreens{screenHost: host, service: cfg.usage}
	}
	if cfg.mcp != nil {
		a.screens.mcp = &mcpScreens{screenHost: host, service: cfg.mcp, setApprovalRule: a.screens.approvals.setApprovalRule}
	}
	if cfg.skills != nil {
		a.screens.skills = &skillScreens{screenHost: host, service: cfg.skills}
	}
	if cfg.hooks != nil {
		a.screens.hooks = &hookScreens{screenHost: host, service: cfg.hooks}
	}
	if cfg.diagnosticTools != nil {
		a.screens.diagnosticTools = &diagnosticScreens{screenHost: host, service: cfg.diagnosticTools}
	}
	if cfg.plugins != nil {
		a.screens.plugins = &pluginScreens{screenHost: host, service: cfg.plugins, workbench: a.workbench, viewsChanged: func() {
			a.registerCommands()
			// A command typed while the views loaded completes once they arrive.
			a.refreshCompletion()
		}}
	}
	if cfg.authoringContext != nil {
		a.screens.authoringContext = &agentDocumentScreens{screenHost: host, service: cfg.authoringContext}
	}
}

// loadPluginViews reads the installed terminal views when installations change.
func (a *app) loadPluginViews(affected bool) {
	if affected && a.screens.plugins != nil && a.runtimeSupports(protocol.FeaturePlugins) {
		a.screens.plugins.loadPluginViews()
	}
}

package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
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
	if cfg.mcp != nil {
		a.mcp = &mcpScreens{screenHost: host, service: cfg.mcp, setApprovalRule: a.setApprovalRule}
	}
}

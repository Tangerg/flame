package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

// sessionRoot is what the Session lifecycle asks of the terminal root, which
// owns the view tree and the modals: present a Session and its transcript,
// close what belonged to a retired Session, and follow the new workspace.
type sessionRoot interface {
	message(label string)
	presentSession(session conversation.Session)
	displayOptions() authoring.RunOptions
	currentTranscript() *transcriptView
	newTranscript() *transcriptView
	presentTranscript(transcript *transcriptView) *transcriptView
	enterSessionHealth()
	dismissModals()
	dismissCompletion()
	cancelPluginCommands(pluginIDs ...string)
	followRuntimeChanges()
	requireRuntimeFeature(feature string) error
}

// sessionLifecycle owns which Session the terminal presents: creating,
// switching, forking, renaming, relocating, importing and rolling back
// Sessions, refreshing the presented one after Runtime changes, and installing
// a projection with its draft. It is the only writer of sessionState.
type sessionLifecycle struct {
	state sessionState
	root  sessionRoot

	runtime        Runtime
	workspaces     Workspaces
	transfers      session.TransferService
	workbench      *workbench.Store
	queue          *workbench.Queue
	replayPolicy   mutation.ReplayPolicy
	localDirectory string
	tasks          taskRunner
	report         func(workbenchConcern, error)

	status   *statusView
	prompt   *promptView
	header   *sessionHeader
	activity *activityView

	runs             *runs
	composer         *promptComposer
	prompts          *interruptPrompts
	steering         *steerDesk
	reader           *readerSurface
	confirm          *confirmPrompt
	sessions         *sessionBrowser
	workspaceChoices *pickerDialog[workspaceChoice]
}

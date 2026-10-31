package terminal

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/attachment"
	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/sessionartifact"
	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/highlight"
)

const (
	sendPrompt                     = keymap.Action(settings.ActionSend)
	insertNewline                  = keymap.Action(settings.ActionNewline)
	cancelRun                      = keymap.Action(settings.ActionCancelRun)
	quitApp                        = keymap.Action(settings.ActionQuit)
	commandPalette                 = keymap.Action(settings.ActionCommandPalette)
	showShortcuts                  = keymap.Action(settings.ActionShortcuts)
	showSessions                   = keymap.Action(settings.ActionSessions)
	showTimeline                   = keymap.Action(settings.ActionTimeline)
	searchTranscript               = keymap.Action(settings.ActionSearch)
	manageQueue                    = keymap.Action(settings.ActionManageQueue)
	chooseModel                    = keymap.Action(settings.ActionChooseModel)
	toggleDetails                  = keymap.Action(settings.ActionToggleDetails)
	openReader       keymap.Action = "open-reader"
	historyPrevious                = keymap.Action(settings.ActionHistoryPrevious)
	historyNext                    = keymap.Action(settings.ActionHistoryNext)
	nextMatch                      = keymap.Action(settings.ActionNextMatch)
	previousMatch                  = keymap.Action(settings.ActionPreviousMatch)
	scrollPageUp                   = keymap.Action(settings.ActionScrollPageUp)
	scrollPageDown                 = keymap.Action(settings.ActionScrollPageDown)
	scrollTop                      = keymap.Action(settings.ActionScrollTop)
	scrollBottom                   = keymap.Action(settings.ActionScrollBottom)
	editPrompt                     = keymap.Action(settings.ActionExternalEditor)
	queueFollowUp    keymap.Action = "queue-follow-up"
	queueOrSendNext  keymap.Action = "queue-or-send-next"
)

type app struct {
	ctx            context.Context
	loop           *program.Runtime
	runtime        Runtime
	workspaces     Workspaces
	changes        changefeed.Source
	transfers      session.TransferService
	feedback       Feedback
	runtimeProfile RuntimeProfile
	replayPolicy   mutation.ReplayPolicy
	artifacts      sessionartifact.Store

	pluginHost       *extensions.Host
	pluginIssues     []extensions.SourceIssue
	operations       *operationOwner
	tasks            taskRunner
	runs             *runs
	steering         *steerDesk
	dialogs          dialogState
	reader           *readerSurface
	prompts          *interruptPrompts
	confirm          *confirmPrompt
	sessions         *sessionBrowser
	lifecycle        *sessionLifecycle
	screens          screens
	timeline         *timelineDialog
	palette          *pickerDialog[commandPaletteItem]
	search           *searchPrompt
	shortcuts        *shortcutGuide
	queueEditor      *queueEditor
	models           *pickerDialog[protocol.Model]
	workspaceChoices *pickerDialog[workspaceChoice]

	transcript *transcriptView
	brand      *brandBanner
	header     *sessionHeader
	activity   *activityView
	queueView  *queueView
	status     *statusView
	settings   settings.Config

	options        authoring.RunOptions
	composer       *promptComposer
	prompt         *promptView
	commands       commandCatalog
	completion     headless.Completion
	completionGate completionGate
	shell          *shellView
	stack          headless.Stack
	queue          *workbench.Queue
	workbench      *workbench.Store
	editor         *draftEditor
	localDirectory string
	detachOnExit   bool

	workbenchHealth   workbenchHealth
	commandOperations commandOperationRegistry
	confirmation      pressConfirmation
	keys              keyRouter
	attention         attentionCenter

	closed         bool
	closeErr       error
	syntax         highlight.Renderer
	prepareDiagram diagramPreparation
}

type appConfig struct {
	context          context.Context
	runtime          Runtime
	workspaces       Workspaces
	changes          changefeed.Source
	transfers        session.TransferService
	usage            Usage
	modelConfig      ModelConfiguration
	goals            Goals
	skills           Skills
	mcp              MCPManagement
	plugins          Plugins
	diagnosticTools  DiagnosticTools
	authoringContext AuthoringContext
	hooks            Hooks
	feedback         Feedback
	runtimeProfile   RuntimeProfile
	replayPolicy     mutation.ReplayPolicy
	clientVersion    string
	snapshot         conversation.SessionSnapshot

	pluginHost      *extensions.Host
	pluginIssues    []extensions.SourceIssue
	attachments     *attachment.Resolver
	initialDraft    authoring.Message
	recoveredSteers []runworkflow.SteerResult
	settings        settings.Config

	options        authoring.RunOptions
	keyBindings    keyBindings
	queue          *workbench.Queue
	workbench      *workbench.Store
	editor         *draftEditor
	localDirectory string
	detachOnExit   bool
}

type terminalAppearance struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	syntax highlight.Renderer
}

func newTerminalAppearance(loop *program.Runtime) terminalAppearance {
	ground := loop.Environment().Ground()
	style := highlight.Style("github-dark")
	if !ground.BG.Default() && !ground.BG.RGB().Dark() {
		style = "github"
	}
	return terminalAppearance{
		theme: kit.Suited(ground), glyphs: kit.GlyphsFor(loop.Environment().Locale()), syntax: highlight.New(style),
	}
}

func newApp(loop *program.Runtime, cfg appConfig) *app {
	cfg.keyBindings.setResolver(loop.After)
	appearance := newTerminalAppearance(loop)
	transcript := newTranscriptView(appearance.theme, appearance.glyphs, loop.Environment().Locale(), loop.Environment().Wheel(), appearance.syntax, cfg.settings.UI.TranscriptRetain, cfg.settings.UI.ToolDetails, loop.Clipboard())
	brand := newBrandBanner(
		appearance.theme,
		appearance.glyphs,
		cfg.clientVersion,
		cfg.snapshot.Session,
		displayRunOptions(cfg.options, cfg.snapshot.Session),
	)
	transcript.SetEntrance(brand)
	a := &app{
		ctx: cfg.context, loop: loop, runtime: cfg.runtime, workspaces: cfg.workspaces,
		runtimeProfile: cfg.runtimeProfile, replayPolicy: cfg.replayPolicy,
		changes: cfg.changes, transfers: cfg.transfers,
		feedback: cfg.feedback,

		pluginHost: cfg.pluginHost, pluginIssues: cfg.pluginIssues,
		operations:        newOperationOwner(cfg.context),
		transcript:        transcript,
		brand:             brand,
		header:            newSessionHeader(appearance.theme, appearance.glyphs, cfg.snapshot.Session),
		activity:          newActivityView(appearance.theme, appearance.glyphs),
		queueView:         newQueueView(appearance.theme, appearance.glyphs),
		status:            newStatusView(appearance.theme, appearance.glyphs),
		queue:             cfg.queue,
		workbench:         cfg.workbench,
		editor:            cfg.editor,
		localDirectory:    cfg.localDirectory,
		detachOnExit:      cfg.detachOnExit,
		settings:          cfg.settings.Clone(),
		options:           cfg.options,
		syntax:            appearance.syntax,
		commandOperations: newCommandOperationRegistry(),
		commands:          newCommandCatalog(),
		attention:         newAttentionCenter(),
		keys:              keyRouter{application: cfg.keyBindings.application, global: cfg.keyBindings.global},
	}
	a.lifecycle = &sessionLifecycle{state: sessionState{current: cfg.snapshot.Session, context: newSessionContextLease()}}
	a.runs = &runs{
		execution: executionState{conversation: conversation.New()},
		runtime:   cfg.runtime, workbench: cfg.workbench, queue: cfg.queue, replayPolicy: cfg.replayPolicy,
		session: &a.lifecycle.state, every: loop.Every, setProgress: loop.Session().SetProgress,
		report: a.reportWorkbenchIssue, link: a, transcript: func() *transcriptView { return a.transcript },
		status: a.status, header: a.header, activity: a.activity,
	}
	a.tasks = taskRunner{operations: a.operations, dispatch: loop.Dispatcher(), settled: func() { a.runs.drainQueue() }}
	a.runs.tasks = a.tasks
	a.composer = &promptComposer{
		attachments: cfg.attachments, workbench: cfg.workbench, session: &a.lifecycle.state,
		ctx: cfg.context, after: loop.After, closed: func() bool { return a.closed },
		notify: a.message, report: a.reportWorkbenchIssue, validate: a.validateMessageCapabilities,
		appendNote: func(label, body string) {
			a.transcript.Append(&kit.Entry{Theme: a.transcript.theme, Label: label, Body: body})
		},
		cleared: a.confirmation.Reset,
	}
	a.composer.drafts.store = newDraftPersistence(cfg.workbench, func(result draftPersistenceResult) {
		loop.Dispatcher().Post(func() {
			if a.closed || !a.composer.drafts.store.Current(result.revision) {
				return
			}
			a.reportWorkbenchIssue(workbenchDraft, result.err)
		})
	})
	a.transcript.images = newTerminalImagePresenter(loop.Images())
	diagramTheme := "dark"
	ground := loop.Environment().Ground()
	if !ground.BG.Default() && !ground.BG.RGB().Dark() {
		diagramTheme = "default"
	}
	a.prepareDiagram = prepareMermaid(diagramTheme)
	a.configureMarkdown(a.transcript)
	a.configureComposer(appearance, cfg.keyBindings.editor, cfg.initialDraft)
	a.configureCompletion(appearance)
	a.registerCommands()
	a.buildInterface(appearance, cfg.keyBindings.editor)
	a.runs.prompt, a.runs.prompts, a.runs.queueEditor, a.runs.composer = a.prompt, a.prompts, a.queueEditor, a.composer
	a.steering = &steerDesk{
		runtime: a.runtime, workbench: a.workbench, replayPolicy: a.replayPolicy,
		composer: a.composer, session: &a.lifecycle.state, runs: a.runs, tasks: a.tasks,
		notify: a.message, report: a.reportWorkbenchIssue, drainQueue: a.runs.drainQueue, readSession: func(ctx context.Context, id string) (conversation.SessionSnapshot, error) {
			return a.lifecycle.readSessionAfterMutation(ctx, id)
		},
	}
	a.runs.steering = a.steering
	*a.lifecycle = sessionLifecycle{
		state: a.lifecycle.state, root: a, runtime: a.runtime, workspaces: a.workspaces, transfers: a.transfers, artifacts: a.artifacts,
		workbench: a.workbench, queue: a.queue, replayPolicy: a.replayPolicy, localDirectory: a.localDirectory, tasks: a.tasks,
		report: a.reportWorkbenchIssue, status: a.status, prompt: a.prompt, header: a.header, activity: a.activity,
		runs: a.runs, composer: a.composer, prompts: a.prompts, steering: a.steering, reader: a.reader, confirm: a.confirm,
		sessions: a.sessions, workspaceChoices: a.workspaceChoices,
	}
	a.runs.sessions = a.lifecycle
	a.buildScreens(cfg)
	for _, result := range cfg.recoveredSteers {
		a.steering.receipts.accept(result)
	}
	a.lifecycle.restore(cfg.snapshot)
	a.runs.restoreSessionOutbox()
	_ = a.composer.persistDraft()
	a.followRuntimeChanges()
	a.loadPluginViews(true)
	return a
}

func (a *app) configureComposer(appearance terminalAppearance, keys *keymap.Map, initial authoring.Message) {
	a.composer.Theme = appearance.theme
	a.composer.Prompt = appearance.glyphs.Marker + " "
	a.composer.MaxRows = 6
	a.composer.Editor().Placeholder = "Ask flame to inspect, explain, or change something"
	a.composer.Editor().Keys = keys
	a.composer.Editor().Clipboard = a.loop.Clipboard()
	if a.workbench != nil {
		a.composer.history.Load(a.workbench.History())
	}
	a.composer.restoreComposer(initial)
}

func (a *app) configureCompletion(appearance terminalAppearance) {
	completionKeys := headless.DefaultCompletionKeys()
	completionKeys.Bind(headless.Accept, input.Chord{Code: input.Enter})
	a.completion = headless.Completion{
		Look: appearance.theme.Look(appearance.glyphs), Keys: completionKeys,
		Accept: func(candidate headless.Candidate, token headless.Token) {
			// Acceptance closes both halves of completion: Oolong has already
			// dismissed the list, while the application must retire its async
			// producer so an older file lookup cannot reopen the accepted token.
			a.operations.Cancel(completionOperation)
			if token.Trigger.Prefix == "@" {
				a.acceptAttachmentCompletion(candidate.Text, token)
				return
			}
			a.completionGate.Reset()
			a.composer.Editor().Replace(token.Start, token.End, candidate.Text)
			a.composer.scheduleDraftPersistence()
		},
	}
}

func (a *app) buildInterface(appearance terminalAppearance, editorKeys *keymap.Map) {
	theme, glyphs := appearance.theme, appearance.glyphs
	a.prompt = newPromptView(theme, glyphs, editorKeys, &a.composer.Composer, a.displayOptions())
	a.shell = newShellView(a.header, a.transcript, a.activity, a.queueView, a.status, a.prompt)
	a.wireTranscript(a.transcript)
	a.shell.Focus(true)
	a.stack.SetBase(a.shell)
	a.queueEditor = newQueueEditor(queueEditorConfig{
		queue: a.queue, session: &a.lifecycle.state, run: a.runs, view: a.queueView, prompt: a.prompt,
		stack: &a.stack, theme: theme, glyphs: glyphs, keys: editorKeys, clip: a.loop.Clipboard(),
	})
	a.prompts = newInterruptPrompts(interruptPromptsConfig{
		run: a.runs, editContext: a.openContextEditor, remember: func() settings.RememberPreference { return a.settings.Approval.Remember },
		stack: &a.stack, theme: theme, glyphs: glyphs, syntax: a.syntax,
		wheel: a.loop.Environment().Wheel(), clipboard: a.loop.Clipboard(),
	})
	a.buildSessionPicker(theme, glyphs)
	a.buildWorkspacePicker(theme, glyphs)
	a.buildTimeline(theme, glyphs)
	a.buildModelPicker(theme, glyphs)
	a.palette = newPickerDialog(pickerDialogConfig[commandPaletteItem]{
		stack: &a.stack, theme: theme, glyphs: glyphs, title: "Commands", search: "search commands",
		where: layout.Placement{Width: 82, Height: 18},
		label: func(item commandPaletteItem) string { return "/" + item.command.Name },
		detail: func(item commandPaletteItem) string {
			if !item.availability.Enabled() {
				return item.category + " · unavailable: " + item.availability.Reason()
			}
			return item.category + " · " + item.command.Title
		},
		choose: a.chooseCommand,
	})
	a.shortcuts = newShortcutGuide(&a.stack, theme, glyphs, a.loop.Environment().Wheel(), editorKeys, a.transcript.Keys())
	a.search = newSearchPrompt(&a.stack, theme, glyphs, a.Find)
	a.confirm = &confirmPrompt{stack: &a.stack, theme: theme, glyphs: glyphs, wheel: a.loop.Environment().Wheel(), session: &a.lifecycle.state}
	a.reader = newReaderSurface(readerSurfaceConfig{
		theme: theme, glyphs: glyphs, syntax: a.syntax, stack: &a.stack, loop: a.loop,
		tasks: a.tasks, status: a.status, message: a.message,
	})
	a.listenForSearch()
	a.setWindowTitle()
}

func (a *app) wireTranscript(transcript *transcriptView) {
	a.prompt.SetTranscriptKeys(transcript.Keys())
	transcript.OnFocusChange(a.prompt.SetTranscriptFocused)
	transcript.OnSelection(a.prompt.SetTranscriptSelection)
	transcript.OnCopy(func(string) {
		if !a.runs.conversation().Busy() && !a.runs.following() {
			a.status.note("copied selected transcript text")
		}
	})
}

func (a *app) buildSessionPicker(theme kit.Theme, glyphs kit.Glyphs) {
	a.sessions = newSessionBrowser(sessionBrowserConfig{browser: sessionBrowser{
		runtime: a.runtime, workbench: a.workbench, queue: a.queue, replayPolicy: a.replayPolicy,
		session: &a.lifecycle.state, tasks: a.tasks, status: a.status, stack: &a.stack, theme: theme, glyphs: glyphs,
		clipboard: a.loop.Clipboard(), notify: a.message, observing: a.runs.observing, adoptCurrent: func(session conversation.Session) { a.lifecycle.setActiveSession(session) },
		catalogChanged: func() { a.lifecycle.applySessionInvalidation(true, false) },
	}, switchTo: func(id string) { a.lifecycle.switchSession(id) }})
}

func (a *app) Draw(frame headless.Frame) {
	a.stack.Draw(frame)
	if a.stack.Depth() == 0 && a.completion.Open() {
		a.drawCompletion(frame)
	}
}

func (a *app) Close(ctx context.Context) error {
	if a.closed {
		return a.closeErr
	}
	closeErr := a.runs.settleOpeningForShutdown()
	a.closed = true
	shutdown, err := a.runs.planShutdown(a.detachOnExit, closeErr == nil)
	closeErr = errors.Join(closeErr, err)
	a.runs.dropStream()
	a.operations.Cancel(completionOperation)
	a.cancelPluginCommands()
	a.operations.Close()
	if a.screens.mcp != nil && a.screens.mcp.form != nil {
		a.screens.mcp.closeMCPForm(a.screens.mcp.form)
	}
	a.composer.drafts.cancel()
	// Flush is a serialization barrier: any autosave already in the filesystem
	// finishes first, then the last visible composer state is written last.
	closeErr = errors.Join(closeErr, a.composer.persistDraft())
	closeErr = errors.Join(closeErr, a.composer.drafts.store.Close())
	closeErr = errors.Join(closeErr, a.runs.finishShutdown(ctx, shutdown))
	a.transcript.Close()
	closeErr = errors.Join(closeErr, a.transcript.releaseErr)
	if a.reader != nil {
		a.reader.shutdown()
	}
	a.runs.stopClock()
	a.closeErr = closeErr
	return closeErr
}

func (a *app) submit() {
	message, err := a.composer.message()
	if err != nil {
		a.message(err.Error())
		return
	}
	if message.IsEmpty() {
		a.sendNextQueuedIfBusy()
		return
	}
	if name, arg, command := parseSlashCommand(strings.TrimSpace(message.Text)); command {
		a.runCommand(name, arg, func() {
			a.composer.restoreComposer(authoring.Message{Attachments: message.Attachments})
			a.operations.Cancel(completionOperation)
			a.completion.Dismiss()
		})
		return
	}
	a.dispatchPrompt(message)
}

// dispatchPrompt owns the single path from an authored message to either the
// active run or its durable follow-up queue. Callers such as prompt submission
// cannot bypass session-change exclusion, prompt history, or composer cleanup.
func (a *app) dispatchPrompt(message authoring.Message) {
	if err := a.validateMessageCapabilities(message); err != nil {
		a.message(err.Error())
		return
	}
	if a.operations.Active(sessionChangeOperation) {
		a.message("wait for the current session change to finish")
		return
	}
	commandID := mutation.NewCommandID()
	if _, err := a.queue.EnqueueCommand(commandID, a.lifecycle.state.current.ID, message, a.options); err != nil {
		a.reportWorkbenchIssue(workbenchRunOutbox, err)
		a.message("prompt submission blocked: " + err.Error())
		return
	}
	a.reportWorkbenchIssue(workbenchRunOutbox, nil)
	if a.runs.runAdmissionBlocked() {
		a.runs.enqueueDeferredPrompt()
		return
	}
	a.sessions.dialog.Dismiss()
	a.dismissModelPicker()
	a.composer.resetComposer()
	a.operations.Cancel(completionOperation)
	a.completion.Dismiss()
	if !a.runs.drainQueue() {
		a.runs.syncQueue()
	}
}

func (a *app) sendNextQueuedIfBusy() {
	if !a.runs.blocksAdmission() {
		return
	}
	state := a.queue.State(a.lifecycle.state.current.ID)
	if len(state.Entries) == 0 {
		return
	}
	index := 0
	if _, dispatching := state.DispatchingID(); dispatching {
		index = 1
	}
	if index >= len(state.Entries) || state.Entries[index].Held {
		return
	}
	if err := a.queueEditor.sendQueuedNow(state.Entries[index].ID); err != nil {
		a.message(err.Error())
	}
}

func (a *app) message(label string) {
	if a.runs.conversation().Phase() == conversation.Running {
		a.status.active(label)
		return
	}
	a.status.note(label)
}

func (a *app) terminalClosed() bool { return a.closed }

func (a *app) dismissCompletion() { a.completion.Dismiss() }

func (a *app) currentTranscript() *transcriptView { return a.transcript }

// presentTranscript shows transcript in place of the current one, which it
// returns for the caller to close once nothing refers to it.
func (a *app) presentTranscript(transcript *transcriptView) *transcriptView {
	previous := a.transcript
	a.transcript = transcript
	a.wireTranscript(transcript)
	a.shell.SetTranscript(transcript)
	return previous
}

// enterSessionHealth resets workbench health for a newly presented Session.
func (a *app) enterSessionHealth() {
	a.workbenchHealth.enterSession()
	a.status.setProblem(a.workbenchHealth.problem())
}

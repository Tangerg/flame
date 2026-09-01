package terminal

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/highlight"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/attachment"
	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/sessionartifact"
	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/cli/internal/application/agent/promptqueue"
	"github.com/Tangerg/flame/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/application/extensions"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/application/settings"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

const (
	sendPrompt       keymap.Action = settings.ActionSend
	insertNewline    keymap.Action = settings.ActionNewline
	cancelRun        keymap.Action = settings.ActionCancelRun
	quitApp          keymap.Action = settings.ActionQuit
	commandPalette   keymap.Action = settings.ActionCommandPalette
	showShortcuts    keymap.Action = settings.ActionShortcuts
	showSessions     keymap.Action = settings.ActionSessions
	showTimeline     keymap.Action = settings.ActionTimeline
	searchTranscript keymap.Action = settings.ActionSearch
	manageQueue      keymap.Action = settings.ActionManageQueue
	chooseModel      keymap.Action = settings.ActionChooseModel
	toggleDetails    keymap.Action = settings.ActionToggleDetails
	openReader       keymap.Action = "open reader"
	historyPrevious  keymap.Action = settings.ActionHistoryPrevious
	historyNext      keymap.Action = settings.ActionHistoryNext
	nextMatch        keymap.Action = settings.ActionNextMatch
	previousMatch    keymap.Action = settings.ActionPreviousMatch
	scrollPageUp     keymap.Action = settings.ActionScrollPageUp
	scrollPageDown   keymap.Action = settings.ActionScrollPageDown
	scrollTop        keymap.Action = settings.ActionScrollTop
	scrollBottom     keymap.Action = settings.ActionScrollBottom
	editPrompt       keymap.Action = settings.ActionExternalEditor
	queueFollowUp    keymap.Action = "queue-follow-up"
	queueOrSendNext  keymap.Action = "queue-or-send-next"
)

type app struct {
	ctx              context.Context
	loop             *program.Runtime
	runtime          Runtime
	workspaces       Workspaces
	changes          changefeed.Source
	transfers        session.TransferService
	usage            Usage
	modelConfig      ModelConfiguration
	goals            Goals
	skills           Skills
	mcp              MCPManagement
	schedules        Schedules
	agentMemory      AgentMemory
	knowledge        Knowledge
	diagnosticTools  DiagnosticTools
	authoringContext AuthoringContext
	hooks            Hooks
	feedback         Feedback
	runtimeProfile   *runtimebinding.Profile
	artifacts        sessionartifact.Store
	registry         *extensions.Registry
	pluginHost       *extensions.Host
	pluginIssues     []extensions.SourceIssue
	operations       *operationOwner
	session          sessionState
	execution        executionState
	dialogs          dialogState

	transcript      *transcriptView
	brand           *brandBanner
	header          *sessionHeader
	activity        *activityView
	queueView       *queueView
	queueDrawer     *queueDrawer
	status          *statusView
	settings        settings.Config
	reconnectPolicy retry.ReconnectPolicy
	options         agent.RunOptions
	composer        kit.Composer
	prompt          *promptView
	commands        commandCatalog
	completion      headless.Completion
	completionGate  completionGate
	shell           *shellView
	stack           headless.Stack
	queue           *promptqueue.Queue
	workbench       *workbench.Store
	drafts          *draftPersistence
	draftState      draftObservation
	stopDraftSave   func()
	editor          promptEditor

	attachments        *attachment.Resolver
	attachmentElements map[uint64]agent.Attachment
	history            promptHistory
	workbenchHealth    workbenchHealth
	commandOperations  commandOperationRegistry
	confirmation       pressConfirmation
	applicationKeys    *keymap.Map
	globalKeys         *keymap.Map
	applicationMatcher keymap.Matcher
	globalMatcher      keymap.Matcher
	attention          attentionCenter

	closed   bool
	closeErr error
	syntax   highlight.Renderer
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
	schedules        Schedules
	agentMemory      AgentMemory
	knowledge        Knowledge
	diagnosticTools  DiagnosticTools
	authoringContext AuthoringContext
	hooks            Hooks
	feedback         Feedback
	runtimeProfile   *runtimebinding.Profile
	clientVersion    string
	snapshot         agent.SessionSnapshot
	registry         *extensions.Registry
	pluginHost       *extensions.Host
	pluginIssues     []extensions.SourceIssue
	attachments      *attachment.Resolver
	initialDraft     agent.Message
	settings         settings.Config
	reconnectPolicy  retry.ReconnectPolicy
	options          agent.RunOptions
	keyBindings      keyBindings
	queue            *promptqueue.Queue
	workbench        *workbench.Store
	editor           promptEditor
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
	transcript := newTranscriptView(appearance.theme, appearance.glyphs, loop.Environment().Wheel(), appearance.syntax, cfg.settings.UI.TranscriptRetain, cfg.settings.UI.ToolDetails, loop.Clipboard())
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
		runtimeProfile: cfg.runtimeProfile,
		changes:        cfg.changes, transfers: cfg.transfers, usage: cfg.usage, modelConfig: cfg.modelConfig,
		goals: cfg.goals, skills: cfg.skills, mcp: cfg.mcp, schedules: cfg.schedules,
		agentMemory: cfg.agentMemory, knowledge: cfg.knowledge,
		diagnosticTools:  cfg.diagnosticTools,
		authoringContext: cfg.authoringContext, hooks: cfg.hooks, feedback: cfg.feedback,
		session:    sessionState{current: cfg.snapshot.Session, context: newSessionContextLease()},
		registry:   cfg.registry,
		pluginHost: cfg.pluginHost, pluginIssues: cfg.pluginIssues,
		execution:          executionState{conversation: agent.NewConversation()},
		operations:         newOperationOwner(cfg.context),
		transcript:         transcript,
		brand:              brand,
		header:             newSessionHeader(appearance.theme, appearance.glyphs, cfg.snapshot.Session),
		activity:           newActivityView(appearance.theme, appearance.glyphs),
		queueView:          newQueueView(appearance.theme, appearance.glyphs),
		status:             newStatusView(appearance.theme, appearance.glyphs),
		queue:              cfg.queue,
		workbench:          cfg.workbench,
		editor:             cfg.editor,
		settings:           cfg.settings.Clone(),
		reconnectPolicy:    cfg.reconnectPolicy,
		options:            cfg.options,
		syntax:             appearance.syntax,
		attachments:        cfg.attachments,
		attachmentElements: make(map[uint64]agent.Attachment),
		commandOperations:  newCommandOperationRegistry(),
		commands:           newCommandCatalog(),
		attention:          newAttentionCenter(),
		applicationKeys:    cfg.keyBindings.application,
		globalKeys:         cfg.keyBindings.global,
	}
	a.drafts = newDraftPersistence(cfg.workbench, func(result draftPersistenceResult) {
		loop.Dispatcher().Post(func() {
			if a.closed || !a.drafts.Current(result.revision) {
				return
			}
			a.reportWorkbenchIssue(workbenchDraft, result.err)
		})
	})
	a.transcript.images = newTerminalImagePresenter(loop.Images())
	a.configureComposer(appearance, cfg.keyBindings.editor, cfg.initialDraft)
	a.configureCompletion(appearance)
	a.registerCommands()
	a.buildInterface(appearance, cfg.keyBindings.editor)
	a.restore(cfg.snapshot)
	a.restoreSessionOutbox()
	_ = a.persistDraft()
	a.followRuntimeChanges()
	return a
}

func (a *app) configureComposer(appearance terminalAppearance, keys *keymap.Map, initial agent.Message) {
	a.composer = kit.Composer{
		Theme: appearance.theme, Prompt: appearance.glyphs.Marker + " ",
		MaxRows: 6,
	}
	a.composer.Editor().Placeholder = "Ask flame to inspect, explain, or change something"
	a.composer.Editor().Keys = keys
	a.composer.Editor().Clipboard = a.loop.Clipboard()
	if a.workbench != nil {
		a.history.Load(a.workbench.History())
	}
	a.restoreComposer(initial)
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
			a.scheduleDraftPersistence()
		},
	}
}

func (a *app) buildInterface(appearance terminalAppearance, editorKeys *keymap.Map) {
	theme, glyphs := appearance.theme, appearance.glyphs
	a.prompt = newPromptView(theme, glyphs, editorKeys, &a.composer, a.displayOptions())
	a.shell = newShellView(a.header, a.transcript, a.activity, a.queueView, a.status, a.prompt)
	a.wireTranscript(a.transcript)
	a.shell.Focus(true)
	a.stack.SetBase(a.shell)
	a.buildQueueDrawer(theme, glyphs, editorKeys)
	a.buildApprovalDialog(theme, glyphs)
	a.buildSessionPicker(theme, glyphs)
	a.buildWorkspacePicker(theme, glyphs)
	a.buildTimeline(theme, glyphs)
	a.buildRuntimePickers(theme, glyphs)
	a.buildCommandPalette(theme, glyphs)
	a.buildShortcutDialog(theme, glyphs, editorKeys)
	a.buildSearchDialog(theme, glyphs)
	a.buildReader(theme, glyphs)
	a.listenForSearch()
	a.setWindowTitle()
}

func (a *app) wireTranscript(transcript *transcriptView) {
	a.prompt.SetTranscriptKeys(transcript.Keys())
	transcript.OnFocusChange(a.prompt.SetTranscriptFocused)
	transcript.OnSelection(a.prompt.SetTranscriptSelection)
	transcript.OnCopy(func(string) {
		if !a.execution.conversation.Busy() && !a.execution.following {
			a.status.note("copied selected transcript text")
		}
	})
}

func (a *app) buildSessionPicker(theme kit.Theme, glyphs kit.Glyphs) {
	a.dialogs.sessionCenter = newSessionCenterPane(theme, glyphs, func(session agent.Session) {
		a.dialogs.sessionDialog.Dismiss()
		a.switchSession(session.ID)
	})
	a.dialogs.sessionCenter.loadMore = a.loadMoreSessions
	a.dialogs.sessionCenter.toggleFavorite = a.toggleSessionFavorite
	a.dialogs.sessionCenter.rename = a.openSessionRename
	a.dialogs.sessionCenter.delete = a.openSessionDelete
	a.dialogs.sessionDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Sessions · Center", Body: a.dialogs.sessionCenter,
		Where: layout.Placement{Width: 96, Height: 24},
	})
	a.dialogs.sessionCenter.picker.cancel = func() {
		a.operations.Cancel(pickerCatalogOperation)
		a.dialogs.sessionDialog.Dismiss()
	}
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
	var closeErr error
	if !a.execution.blocksAdmission() && a.execution.openingRunID != "" {
		if err := a.attemptQueuedDispatchSettlement(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("settle acknowledged run start: %w", err))
		} else {
			a.execution.openingRunID = ""
		}
	}
	a.closed = true
	var (
		target           agent.CancelRun
		openingCommandID agent.CommandID
		cancelRuntime    bool
		cancelReplay     commandreplay.Guard
	)
	if a.execution.pendingCancel != nil {
		target, openingCommandID, cancelRuntime = a.execution.pendingCancel.request, a.execution.pendingCancel.openingCommandID, true
		cancelReplay = a.execution.pendingCancel.replay
	} else {
		target, cancelRuntime = a.activeCancellation()
		openingCommandID = a.openingCommandForRun(target.RunID)
		cancelReplay = commandReplayGuard(a.runtimeProfile)
	}
	var (
		pendingStart  workbench.PendingRun
		cancelOpening bool
	)
	if !cancelRuntime && closeErr == nil {
		var err error
		pendingStart, cancelOpening, err = a.stageOpeningCancellation()
		closeErr = errors.Join(closeErr, err)
	}
	a.dropStream()
	a.operations.Cancel(completionOperation)
	a.cancelPluginCommands()
	a.operations.Close()
	a.cancelScheduledDraftSave()
	// Flush is a serialization barrier: any autosave already in the filesystem
	// finishes first, then the last visible composer state is written last.
	closeErr = errors.Join(closeErr, a.persistDraft(), a.drafts.Close())
	if cancelRuntime {
		if err := a.cancelRuntimeNow(ctx, target, cancelReplay); err == nil {
			closeErr = errors.Join(closeErr, a.retireCanceledRuntimeOwnership(target.RunID, openingCommandID))
		} else {
			closeErr = errors.Join(closeErr, err)
		}
	} else if cancelOpening {
		closeErr = errors.Join(closeErr, a.cancelOpeningRunNow(ctx, pendingStart))
	}
	a.transcript.Close()
	if a.dialogs.reader != nil {
		a.dialogs.reader.Shutdown()
	}
	if a.execution.stopClock != nil {
		a.execution.stopClock()
		a.execution.stopClock = nil
	}
	a.closeErr = closeErr
	return closeErr
}

func (a *app) submit() {
	message, err := a.composerMessage()
	if err != nil {
		a.message(err.Error())
		return
	}
	if message.Text == "" && len(message.Attachments) == 0 {
		a.sendNextQueuedIfBusy()
		return
	}
	if name, arg, command := parseSlashCommand(message.Text); command {
		// A command acts on the staged composer context. Clear its command text but
		// put attachment elements back so /attachments and /detach can inspect or
		// mutate them without accidentally sending a user turn.
		a.restoreComposer(agent.Message{Attachments: message.Attachments})
		a.operations.Cancel(completionOperation)
		a.completion.Dismiss()
		a.runCommand(name, arg)
		return
	}
	a.dispatchPrompt(message)
}

// dispatchPrompt owns the single path from an authored message to either the
// active run or its durable follow-up queue. Callers such as recipe expansion
// cannot bypass session-change exclusion, prompt history, or composer cleanup.
func (a *app) dispatchPrompt(message agent.Message) {
	if err := a.validateMessageCapabilities(message); err != nil {
		a.message(err.Error())
		return
	}
	if a.operations.Active(sessionChangeOperation) {
		a.message("wait for the current session change to finish")
		return
	}
	commandID, err := agent.NewCommandID()
	if err != nil {
		a.message("prompt submission blocked: " + err.Error())
		return
	}
	if commitPromptSubmissionErr := a.commitPromptSubmission(commandID, message); commitPromptSubmissionErr != nil {
		a.reportWorkbenchIssue(workbenchRunOutbox, commitPromptSubmissionErr)
		a.message("prompt submission blocked: " + commitPromptSubmissionErr.Error())
		return
	}
	a.reportWorkbenchIssue(workbenchRunOutbox, nil)
	if a.execution.blocksAdmission() || a.operations.BlocksRunAdmission() {
		a.enqueueDeferredPrompt(commandID, message)
		return
	}
	_, err = a.queue.EnqueueCommand(commandID, a.session.current.ID, message, a.options)
	if err != nil {
		a.message(err.Error())
		return
	}
	a.operations.Cancel(pickerCatalogOperation)
	a.dialogs.sessionDialog.Dismiss()
	a.dialogs.modelDialog.Dismiss()
	a.resetComposer()
	a.operations.Cancel(completionOperation)
	a.completion.Dismiss()
	if !a.drainQueue() {
		a.syncQueue()
	}
}

// commitPromptSubmission is the durable ownership boundary between the composer
// and a runtime run or follow-up queue. Once submission starts, a restart must
// not resurrect the same prompt as an unsent draft.
func (a *app) commitPromptSubmission(commandID agent.CommandID, message agent.Message) error {
	if err := message.Validate(); err != nil {
		return err
	}
	if a.workbench != nil {
		pending := workbench.PendingRun{
			State: workbench.PendingRunQueued, Replay: commandreplay.UnprotectedGuard(),
			CancelReplay: commandreplay.UnprotectedGuard(),
			Command: agent.StartRun{
				CommandID: commandID, SessionID: a.session.current.ID, Message: message.Clone(), Options: a.options.Clone(),
			},
		}
		if err := a.workbench.StagePendingRun(pending); err != nil {
			return fmt.Errorf("save pending run: %w", err)
		}
	}
	return nil
}

func (a *app) sendNextQueuedIfBusy() {
	if !a.execution.blocksAdmission() {
		return
	}
	state := a.queue.State(a.session.current.ID)
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
	if err := a.sendQueuedNow(state.Entries[index].ID); err != nil {
		a.message(err.Error())
	}
}

func (a *app) message(label string) {
	if a.execution.conversation.Phase() == agent.ConversationRunning {
		a.status.active(label)
		return
	}
	a.status.note(label)
}

package terminal

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/attachment"
	runworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	flameauthoring "github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

// Every Runtime service is required; the negotiated RuntimeProfile alone
// decides which of them the Runtime supports.
type Config struct {
	Runtime          Runtime
	RuntimeProfile   RuntimeProfile
	Workspaces       Workspaces
	Changes          changefeed.Source
	Transfers        session.TransferService
	Usage            Usage
	ModelConfig      ModelConfiguration
	Goals            Goals
	Skills           Skills
	MCP              MCPManagement
	Plugins          Plugins
	DiagnosticTools  DiagnosticTools
	AuthoringContext AuthoringContext
	Hooks            Hooks
	Feedback         Feedback
	ClientVersion    string
	SessionID        string
	Workspace        string
	InitialPrompt    string
	PluginSources    []extensions.Source
	Host             program.Host
	Settings         settings.Config
	OpenWorkbench    func() (*workbench.Store, error)
	LocalDirectory   string
	DetachOnExit     bool
}

func (c Config) validate() error {
	for _, required := range []struct {
		name  string
		value any
	}{
		{"Runtime", c.Runtime}, {"RuntimeProfile", c.RuntimeProfile}, {"Workspaces", c.Workspaces},
		{"Changes", c.Changes}, {"Transfers", c.Transfers}, {"Usage", c.Usage}, {"ModelConfig", c.ModelConfig},
		{"Goals", c.Goals}, {"Skills", c.Skills}, {"MCP", c.MCP}, {"Plugins", c.Plugins},
		{"DiagnosticTools", c.DiagnosticTools}, {"AuthoringContext", c.AuthoringContext},
		{"Hooks", c.Hooks}, {"Feedback", c.Feedback},
	} {
		if required.value == nil {
			return fmt.Errorf("terminal: %s is required", required.name)
		}
	}
	if c.OpenWorkbench == nil {
		return errors.New("terminal: OpenWorkbench is required")
	}
	return nil
}

func Run(ctx context.Context, cfg Config) (runErr error) {
	if err := cfg.validate(); err != nil {
		return err
	}
	prepared, err := prepareSession(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, prepared.workbench.Close()) }()

	extensionHost := extensions.NewHost()
	defer func() { runErr = errors.Join(runErr, extensionHost.Close()) }()
	discovered, err := extensions.Discover(ctx, cfg.PluginSources...)
	if err != nil {
		return err
	}
	results, err := extensionHost.Activate(discovered.Plugins)
	if err != nil {
		return err
	}

	var active *app
	prompts, err := workbench.NewQueue(prepared.workbench)
	if err != nil {
		return err
	}
	programConfig := program.ScreenConfig{
		Build: func(loop *program.Runtime) (scene.Element, error) {
			started, err := startApp(ctx, loop, cfg, prepared, extensionHost, discovered, results, prompts)
			if err != nil {
				return scene.Element{}, err
			}
			active = started
			return active.Element(), nil
		},
		Config: program.Config{Terminal: term.Features{Probe: true, Mouse: prepared.settings.UI.Mouse, Focus: true, Keyboard: term.KeyboardCompatible}, Host: cfg.Host},
	}
	if err := programConfig.Validate(); err != nil {
		return err
	}
	if programConfig.Host == nil {
		host, err := term.Open(term.Config{Features: programConfig.Terminal, AltScreen: true})
		if errors.Is(err, term.ErrNotTerminal) {
			return terminalUnavailableError{cause: err}
		}
		if err != nil {
			return err
		}
		// Images must be released before the terminal drains and closes its writer.
		defer func() { runErr = errors.Join(runErr, host.Close()) }()
		programConfig.Host = program.TerminalHost(host)
	}
	screen, err := program.NewScreen(programConfig)
	if err != nil {
		return err
	}
	err = screen.Run(ctx)
	if active != nil {
		err = errors.Join(err, active.Close(ctx))
	}
	if errors.Is(err, term.ErrNotTerminal) {
		return terminalUnavailableError{cause: err}
	}
	return err
}

// startApp builds the terminal app and reports what startup recovered: an
// interrupted rollback, refused steers, unloaded plugins and unconfirmed
// installed commands.
func startApp(
	ctx context.Context,
	loop *program.Runtime,
	cfg Config,
	prepared preparedSession,
	extensionHost *extensions.Host,
	discovered extensions.Discovery,
	results []extensions.LifecycleResult,
	prompts *workbench.Queue,
) (*app, error) {
	active, err := newApp(loop, appConfig{
		context: ctx, runtime: cfg.Runtime, runtimeProfile: prepared.runtimeProfile, replayPolicy: prepared.replayPolicy,
		workspaces: cfg.Workspaces, changes: cfg.Changes, transfers: cfg.Transfers,
		usage: cfg.Usage, modelConfig: cfg.ModelConfig, goals: cfg.Goals, skills: cfg.Skills,
		mcp: cfg.MCP, plugins: cfg.Plugins,
		diagnosticTools: cfg.DiagnosticTools, authoringContext: cfg.AuthoringContext,
		hooks: cfg.Hooks, feedback: cfg.Feedback,
		snapshot: prepared.opened, clientVersion: cfg.ClientVersion,
		pluginHost: extensionHost, pluginIssues: discovered.Issues,
		attachments: prepared.attachments,
		settings:    prepared.settings,
		options:     prepared.options, keyBindings: prepared.keyBindings, queue: prompts,
		workbench: prepared.workbench, initialDraft: prepared.draft, editor: prepared.editor,
		recoveredSteers: prepared.recoveryIssues.steers.Accepted,
		localDirectory:  cfg.LocalDirectory, detachOnExit: cfg.DetachOnExit,
	})
	if err != nil {
		return nil, err
	}
	var rollbackNotice string
	if prepared.rollbackRecovery != nil {
		rollbackNotice = rollbackRecoveryNotice(*prepared.rollbackRecovery)
	}
	active.announceStartup(
		rollbackNotice,
		active.steering.refusedSteersNotice(prepared.recoveryIssues.steers.Refused),
		unloadedPluginsNotice(results, discovered.Issues),
		pendingInstalledCommandsNotice(prepared.workbench),
	)
	active.reportWorkbenchIssue(workbenchSteerOutbox, prepared.recoveryIssues.steer)
	if err := loop.Dispatcher().Post(func() { active.shell.FocusPrompt() }); err != nil {
		return nil, err
	}
	return active, nil
}

func pendingInstalledCommandsNotice(store *workbench.Store) string {
	commands := store.PendingInstalledCommands()
	if len(commands) == 0 {
		return ""
	}
	intents := make([]string, 0, len(commands))
	for _, command := range commands {
		intents = append(intents, command.Intent)
	}
	return "unconfirmed plugin commands · " + strings.Join(intents, ", ")
}

type terminalUnavailableError struct{ cause error }

func (e terminalUnavailableError) Error() string      { return e.cause.Error() }
func (e terminalUnavailableError) Unwrap() error      { return e.cause }
func (terminalUnavailableError) TerminalUnavailable() {}

type preparedSession struct {
	opened         conversation.SessionSnapshot
	runtimeProfile RuntimeProfile
	replayPolicy   mutation.ReplayPolicy
	attachments    *attachment.Resolver
	keyBindings    keyBindings
	settings       settings.Config

	options          flameauthoring.RunOptions
	workbench        *workbench.Store
	draft            flameauthoring.Message
	editor           *draftEditor
	rollbackRecovery *workbench.SessionRollbackRecovery
	recoveryIssues   sessionCommandRecovery
}

type sessionCommandRecovery struct {
	steer  error
	steers runworkflow.SteerRecovery
}

func prepareSession(ctx context.Context, cfg Config) (preparedSession, error) {
	configured, bindings, err := validatedSessionConfig(cfg)
	if err != nil {
		return preparedSession{}, err
	}
	replayPolicy, err := mutation.NewReplayPolicy(cfg.RuntimeProfile.IdempotencyLimits(), time.Now)
	if err != nil {
		return preparedSession{}, fmt.Errorf("session command replay policy: %w", err)
	}
	authoring, err := cfg.OpenWorkbench()
	if err != nil {
		return preparedSession{}, fmt.Errorf("open CLI workbench: %w", err)
	}
	if authoring == nil {
		return preparedSession{}, errors.New("session: workbench factory returned no authoring store")
	}
	recovery, err := recoverSessionCommands(ctx, cfg.Runtime, authoring, replayPolicy)
	if err != nil {
		return preparedSession{}, errors.Join(err, authoring.Close())
	}
	prepared, err := openPreparedSession(ctx, cfg, configured, bindings, authoring)
	prepared.replayPolicy = replayPolicy
	if err != nil {
		return preparedSession{}, errors.Join(err, authoring.Close())
	}
	prepared.recoveryIssues = recovery
	return prepared, nil
}

func validatedSessionConfig(cfg Config) (settings.Config, keyBindings, error) {
	if cfg.LocalDirectory != "" && !filepath.IsAbs(cfg.LocalDirectory) {
		return settings.Config{}, keyBindings{}, errors.New("session local directory is not absolute")
	}
	configured := cfg.Settings.Clone()
	if err := configured.Validate(); err != nil {
		return settings.Config{}, keyBindings{}, fmt.Errorf("session settings: %w", err)
	}
	bindings, err := configuredKeyBindings(configured)
	if err != nil {
		return settings.Config{}, keyBindings{}, err
	}
	return configured, bindings, nil
}

func recoverSessionCommands(
	ctx context.Context,
	runtime Runtime,
	authoring *workbench.Store,
	replayPolicy mutation.ReplayPolicy,
) (sessionCommandRecovery, error) {
	recovery := sessionCommandRecovery{}
	if err := session.RecoverDeletions(
		ctx, runtime, authoring, replayPolicy, runtimeRecoveryBackoff,
	); err != nil {
		return sessionCommandRecovery{}, fmt.Errorf("recover session deletions: %w", err)
	}
	steers, err := runworkflow.RecoverSteers(
		ctx, runtime, authoring, replayPolicy, runtimeRecoveryBackoff,
	)
	recovery.steers = steers
	if err != nil {
		if !errors.Is(err, runworkflow.ErrSteerReplayUnavailable) {
			return sessionCommandRecovery{}, fmt.Errorf("recover steer commands: %w", err)
		}
		recovery.steer = fmt.Errorf("recover steer commands: %w", err)
	}
	if err := session.RecoverRollbacks(
		ctx, runtime, authoring, replayPolicy, runtimeRecoveryBackoff,
	); err != nil {
		return sessionCommandRecovery{}, fmt.Errorf("recover session rollbacks: %w", err)
	}
	return recovery, nil
}

func openPreparedSession(
	ctx context.Context,
	cfg Config,
	configured settings.Config,
	bindings keyBindings,
	authoring *workbench.Store,
) (preparedSession, error) {
	options, err := configured.RunOptions()
	if err != nil {
		return preparedSession{}, fmt.Errorf("session run options: %w", err)
	}
	opened, err := session.Open(ctx, cfg.Runtime, cfg.SessionID, cfg.Workspace)
	if err != nil {
		return preparedSession{}, err
	}
	if activateSessionStateErr := authoring.ActivateSessionState(opened.Session.ID); activateSessionStateErr != nil {
		return preparedSession{}, fmt.Errorf("activate session authoring state: %w", activateSessionStateErr)
	}
	attachments, err := attachment.New(authoringDirectory(cfg.LocalDirectory, opened.Session.Workspace.Path))
	if err != nil {
		return preparedSession{}, fmt.Errorf("session attachments: %w", err)
	}
	if rememberWorkspaceErr := authoring.RememberWorkspace(opened.Session.Workspace.Path); rememberWorkspaceErr != nil {
		return preparedSession{}, fmt.Errorf("remember workspace: %w", rememberWorkspaceErr)
	}
	editor, err := configuredDraftEditor()
	if err != nil {
		return preparedSession{}, err
	}
	// Activate last: no later preparation step may fail after the one-time rollback
	// report becomes unreachable.
	activation, err := authoring.ActivateSessionDraft(
		opened.Session.ID, flameauthoring.Message{Text: cfg.InitialPrompt},
	)
	if err != nil {
		return preparedSession{}, fmt.Errorf("activate session draft: %w", err)
	}
	return preparedSession{
		opened: opened, runtimeProfile: cfg.RuntimeProfile, attachments: attachments, keyBindings: bindings,
		settings: configured, options: options,
		workbench: authoring, draft: activation.Draft, editor: editor,
		rollbackRecovery: activation.Rollback,
	}, nil
}

func authoringDirectory(localDirectory, workspace string) string {
	if localDirectory != "" {
		return localDirectory
	}
	return workspace
}

func (a *app) announceStartup(notices ...string) {
	notices = slices.DeleteFunc(notices, func(notice string) bool { return notice == "" })
	switch len(notices) {
	case 0:
	case 1:
		a.message(notices[0])
	default:
		a.transcript.Append(&kit.Entry{Theme: a.transcript.theme, Label: "startup", Body: kit.NewParagraph(strings.Join(notices, "\n"), a.transcript.theme.Text)})
	}
}

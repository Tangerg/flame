// Package terminal is the interactive adapter for the Flame runtime. It owns
// oolong state and translates user intent into the runtime port; neither the
// domain model nor the Runtime binding adapter imports this package.
package terminal

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/attachment"
	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/application/extensions"
	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/application/settings"
	"github.com/Tangerg/flame/cli/internal/application/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

// Config describes one terminal application instance.
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
	Schedules        Schedules
	AgentMemory      AgentMemory
	DiagnosticTools  DiagnosticTools
	AuthoringContext AuthoringContext
	Hooks            Hooks
	Feedback         Feedback
	ClientVersion    string
	SessionID        string
	Workspace        string
	InitialPrompt    string
	Plugins          []extensions.Plugin
	PluginSources    []extensions.Source
	Host             program.Host
	Settings         *settings.Config
	OpenWorkbench    func() (*workbench.Store, error)
	// LocalDirectory fixes client-side authoring to this absolute directory.
	// Runtime workspace references then remain opaque to this client.
	// When empty, an embedded Runtime's Session workspace is also local.
	LocalDirectory string
	// DetachOnExit leaves shared Runtime execution alive when the terminal exits.
	// Explicit cancellation still owns its acknowledgement and settlement.
	DetachOnExit bool
}

// Run opens and owns the terminal interface until the user leaves.
func Run(ctx context.Context, cfg Config) (runErr error) {
	prepared, err := prepareSession(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, prepared.workbench.Close()) }()

	registry := new(extensions.Registry)
	extensionHost, err := extensions.NewHost(registry)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, extensionHost.Close()) }()
	sources := make([]extensions.Source, 0, 1+len(cfg.PluginSources))
	sources = append(sources, extensions.StaticSource{
		Name: "terminal", Plugins: append([]extensions.Plugin{builtinPlugin()}, cfg.Plugins...),
	})
	sources = append(sources, cfg.PluginSources...)
	discovered, err := extensions.Discover(ctx, sources...)
	if err != nil {
		return err
	}
	results, err := extensionHost.Activate(discovered.Plugins)
	if err != nil {
		return err
	}
	if requireLoadedPluginErr := requireLoadedPlugin(results, "terminal.core"); requireLoadedPluginErr != nil {
		return requireLoadedPluginErr
	}

	var active *app
	prompts, err := workbench.NewQueue(prepared.workbench)
	if err != nil {
		return err
	}
	programConfig := program.Config{
		Root: func(loop *program.Runtime) program.Component {
			active = newApp(loop, appConfig{
				context: ctx, runtime: cfg.Runtime, runtimeProfile: prepared.runtimeProfile, replayPolicy: prepared.replayPolicy,
				workspaces: cfg.Workspaces, changes: cfg.Changes, transfers: cfg.Transfers,
				usage: cfg.Usage, modelConfig: cfg.ModelConfig, goals: cfg.Goals, skills: cfg.Skills,
				mcp: cfg.MCP, schedules: cfg.Schedules, agentMemory: cfg.AgentMemory,
				diagnosticTools: cfg.DiagnosticTools, authoringContext: cfg.AuthoringContext,
				hooks: cfg.Hooks, feedback: cfg.Feedback,
				snapshot: prepared.opened, clientVersion: cfg.ClientVersion,
				registry: registry, pluginHost: extensionHost, pluginIssues: discovered.Issues,
				attachments: prepared.attachments,
				settings:    prepared.settings,
				options:     prepared.options, keyBindings: prepared.keyBindings, queue: prompts,
				workbench: prepared.workbench, initialDraft: prepared.draft, editor: prepared.editor,
				recoveredSteers: prepared.recoveryIssues.steers.Accepted,
				localDirectory:  cfg.LocalDirectory, detachOnExit: cfg.DetachOnExit,
			})
			var rollbackNotice string
			if prepared.rollbackRecovery != nil {
				rollbackNotice = rollbackRecoveryNotice(*prepared.rollbackRecovery)
			}
			active.announceStartup(
				rollbackNotice,
				active.refusedSteersNotice(prepared.recoveryIssues.steers.Refused),
				unloadedPluginsNotice(results, discovered.Issues),
			)
			active.reportWorkbenchIssue(workbenchSteerOutbox, prepared.recoveryIssues.steer)
			return headless.NewRoot(active)
		},
		Terminal: term.Features{Probe: true, Mouse: prepared.settings.UI.Mouse, Focus: true, Keyboard: term.KeyboardCompatible},
		Host:     cfg.Host,
	}
	if err := programConfig.Validate(); err != nil {
		return err
	}
	if programConfig.Host == nil {
		host, err := term.Open(programConfig.TerminalConfig())
		if errors.Is(err, term.ErrNotTerminal) {
			return terminalUnavailableError{cause: err}
		}
		if err != nil {
			return err
		}
		// Images must be released before the terminal drains and closes its writer.
		defer func() { runErr = errors.Join(runErr, host.Close()) }()
		programConfig.Host = localTerminalHost{host}
	}
	err = program.Run(ctx, programConfig)
	if active != nil {
		err = errors.Join(err, active.Close(ctx))
	}
	if errors.Is(err, term.ErrNotTerminal) {
		return terminalUnavailableError{cause: err}
	}
	return err
}

type localTerminalHost struct{ *term.Terminal }

func (h localTerminalHost) Writer() program.FrameWriter { return h.Terminal.Writer() }
func (h localTerminalHost) Input() program.EventSource  { return localTerminalInput(h) }

type localTerminalInput struct{ *term.Terminal }

func (i localTerminalInput) Err() error { return i.InputErr() }

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

	options          prompt.RunOptions
	workbench        *workbench.Store
	draft            prompt.Message
	editor           *draftEditor
	rollbackRecovery *workbench.SessionRollbackRecovery
	recoveryIssues   sessionCommandRecovery
}

type sessionCommandRecovery struct {
	steer  error
	steers runworkflow.SteerRecovery
}

func prepareSession(ctx context.Context, cfg Config) (preparedSession, error) {
	if cfg.Runtime == nil {
		return preparedSession{}, errors.New("session: agent runtime is required")
	}
	profile, configured, bindings, err := validatedSessionConfig(cfg)
	if err != nil {
		return preparedSession{}, err
	}
	replayPolicy, err := mutation.PolicyFromProfile(profile, time.Now)
	if err != nil {
		return preparedSession{}, fmt.Errorf("session command replay policy: %w", err)
	}
	if cfg.OpenWorkbench == nil {
		return preparedSession{}, errors.New("session: workbench factory is required")
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
	prepared, err := openPreparedSession(ctx, cfg, profile, configured, bindings, authoring)
	prepared.replayPolicy = replayPolicy
	if err != nil {
		return preparedSession{}, errors.Join(err, authoring.Close())
	}
	prepared.recoveryIssues = recovery
	return prepared, nil
}

func validatedSessionConfig(cfg Config) (RuntimeProfile, settings.Config, keyBindings, error) {
	if cfg.LocalDirectory != "" && !filepath.IsAbs(cfg.LocalDirectory) {
		return nil, settings.Config{}, keyBindings{}, errors.New("session local directory is not absolute")
	}
	profile := cfg.RuntimeProfile
	if profile == nil {
		return nil, settings.Config{}, keyBindings{}, errors.New("session runtime profile is required")
	}
	if err := profile.Validate(); err != nil {
		return nil, settings.Config{}, keyBindings{}, fmt.Errorf("session runtime profile: %w", err)
	}
	configured := settings.Default()
	if cfg.Settings != nil {
		configured = cfg.Settings.Clone()
	}
	if err := configured.Validate(); err != nil {
		return nil, settings.Config{}, keyBindings{}, fmt.Errorf("session settings: %w", err)
	}
	bindings, err := configuredKeyBindings(configured)
	if err != nil {
		return nil, settings.Config{}, keyBindings{}, err
	}
	return profile, configured, bindings, nil
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
	profile RuntimeProfile,
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
	// Activate last: it commits the existing draft, argv input, and confirmed
	// rollback opening as one Session authoring transition. No later preparation
	// step may fail after the one-time rollback report becomes unreachable.
	activation, err := authoring.ActivateSessionDraft(
		opened.Session.ID,
		prompt.Message{Text: cfg.InitialPrompt},
	)
	if err != nil {
		return preparedSession{}, fmt.Errorf("activate session draft: %w", err)
	}
	return preparedSession{
		opened: opened, runtimeProfile: profile, attachments: attachments, keyBindings: bindings,
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

func requireLoadedPlugin(results []extensions.LifecycleResult, id string) error {
	for _, result := range results {
		if result.PluginID != id {
			continue
		}
		if result.Phase == extensions.PluginLoaded {
			return nil
		}
		if result.Err != nil {
			return fmt.Errorf("session: required plugin %q is %s: %w", id, result.Phase, result.Err)
		}
		return fmt.Errorf("session: required plugin %q is %s", id, result.Phase)
	}
	return fmt.Errorf("session: required plugin %q was not discovered", id)
}

// announceStartup reports what recovery and plugin loading found. The status
// row holds one message, so several notices go to the transcript together
// instead of each replacing the one before it.
func (a *app) announceStartup(notices ...string) {
	notices = slices.DeleteFunc(notices, func(notice string) bool { return notice == "" })
	switch len(notices) {
	case 0:
	case 1:
		a.message(notices[0])
	default:
		a.transcript.Append(&kit.Entry{Theme: a.transcript.theme, Label: "startup", Body: strings.Join(notices, "\n")})
	}
}

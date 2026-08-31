// Package terminal is the interactive adapter for the Flame runtime. It owns
// oolong state and translates user intent into the runtime port; neither the
// domain model nor the Runtime binding adapter imports this package.
package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/attachment"
	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	"github.com/Tangerg/flame/cli/internal/application/agent/promptqueue"
	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/application/extensions"
	"github.com/Tangerg/flame/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/cli/internal/application/integration/models"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/application/settings"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
	"github.com/Tangerg/flame/cli/internal/domain/schedule"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

// Config describes one terminal application instance.
type Config struct {
	Runtime          agent.Runtime
	RuntimeProfile   *runtimebinding.Profile
	Workspaces       workspace.Service
	Changes          changefeed.Source
	Transfers        session.TransferService
	Usage            agent.UsageService
	ModelConfig      models.Service
	Goals            agent.GoalService
	Skills           workspace.SkillService
	MCP              mcp.Service
	Schedules        schedule.Service
	AgentMemory      agent.MemoryService
	Knowledge        workspace.KnowledgeService
	DiagnosticTools  workspace.DiagnosticToolService
	AuthoringContext workspace.AuthoringContextService
	Hooks            workspace.HookService
	Feedback         agent.FeedbackService
	ClientVersion    string
	SessionID        string
	Workspace        string
	InitialPrompt    string
	Plugins          []extensions.Plugin
	PluginSources    []extensions.Source
	Host             program.Host
	Settings         *settings.Config
	StateDirectory   string
}

// Run opens and owns the terminal interface until the user leaves.
func Run(ctx context.Context, cfg Config) (runErr error) {
	prepared, err := prepareSession(ctx, cfg)
	if err != nil {
		return err
	}

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
	queue := promptqueue.New()
	err = program.Run(ctx, program.Config{
		Root: func(loop *program.Runtime) program.Component {
			active = newApp(loop, appConfig{
				context: ctx, runtime: cfg.Runtime, runtimeProfile: prepared.runtimeProfile,
				workspaces: cfg.Workspaces, changes: cfg.Changes, transfers: cfg.Transfers,
				usage: cfg.Usage, modelConfig: cfg.ModelConfig, goals: cfg.Goals, skills: cfg.Skills,
				mcp: cfg.MCP, schedules: cfg.Schedules, agentMemory: cfg.AgentMemory, knowledge: cfg.Knowledge,
				diagnosticTools: cfg.DiagnosticTools, authoringContext: cfg.AuthoringContext,
				hooks: cfg.Hooks, feedback: cfg.Feedback,
				snapshot: prepared.opened, clientVersion: cfg.ClientVersion,
				registry: registry, pluginHost: extensionHost, pluginIssues: discovered.Issues,
				attachments: prepared.attachments,
				settings:    prepared.settings, reconnectPolicy: prepared.reconnectPolicy,
				options: prepared.options, keyBindings: prepared.keyBindings, queue: queue,
				workbench: prepared.workbench, initialDraft: prepared.draft, editor: prepared.editor,
			})
			if prepared.rollbackRecovery != nil {
				active.reportSessionRollbackRecovery(*prepared.rollbackRecovery)
			}
			return headless.NewRoot(active)
		},
		Terminal: term.Features{Probe: true, Mouse: prepared.settings.UI.Mouse, Focus: true, Keyboard: term.KeyboardCompatible},
		Host:     cfg.Host,
	})
	if active != nil {
		err = errors.Join(err, active.Close(ctx))
	}
	return err
}

type preparedSession struct {
	opened           agent.SessionSnapshot
	runtimeProfile   *runtimebinding.Profile
	attachments      *attachment.Resolver
	keyBindings      keyBindings
	settings         settings.Config
	reconnectPolicy  retry.ReconnectPolicy
	options          agent.RunOptions
	workbench        *workbench.Store
	draft            agent.Message
	editor           *draftEditor
	rollbackRecovery *workbench.SessionRollbackRecovery
}

func prepareSession(ctx context.Context, cfg Config) (preparedSession, error) {
	if cfg.Runtime == nil {
		return preparedSession{}, errors.New("session: agent runtime is required")
	}
	profile, configured, bindings, err := validatedSessionConfig(cfg)
	if err != nil {
		return preparedSession{}, err
	}
	reconnectPolicy, err := retry.NewReconnectPolicy(configured.UI.ReconnectAttempts)
	if err != nil {
		return preparedSession{}, fmt.Errorf("session reconnect policy: %w", err)
	}
	authoring, err := openSessionWorkbench(cfg.StateDirectory)
	if err != nil {
		return preparedSession{}, fmt.Errorf("open CLI workbench: %w", err)
	}
	if err := recoverSessionCommands(ctx, cfg.Runtime, authoring, profile); err != nil {
		return preparedSession{}, err
	}
	return openPreparedSession(ctx, cfg, profile, configured, reconnectPolicy, bindings, authoring)
}

func openSessionWorkbench(directory string) (*workbench.Store, error) {
	if strings.TrimSpace(directory) == "" {
		return workbench.OpenMemory(workbench.Config{})
	}
	return workbench.OpenDirectory(directory, workbench.Config{})
}

func validatedSessionConfig(cfg Config) (*runtimebinding.Profile, settings.Config, keyBindings, error) {
	var profile *runtimebinding.Profile
	if cfg.RuntimeProfile != nil {
		cloned := cfg.RuntimeProfile.Clone()
		if err := cloned.Validate(); err != nil {
			return nil, settings.Config{}, keyBindings{}, fmt.Errorf("session runtime profile: %w", err)
		}
		profile = &cloned
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
	runtime agent.Runtime,
	authoring *workbench.Store,
	profile *runtimebinding.Profile,
) error {
	if err := session.RecoverDeletions(
		ctx, runtime, authoring, commandReplayPolicy(profile), runtimeRecoveryBackoff,
	); err != nil {
		return fmt.Errorf("recover session deletions: %w", err)
	}
	if err := runworkflow.RecoverSteers(
		ctx, runtime, authoring, commandReplayPolicy(profile), runtimeRecoveryBackoff,
	); err != nil {
		return fmt.Errorf("recover steer commands: %w", err)
	}
	if err := session.RecoverRollbacks(
		ctx, runtime, authoring, commandReplayPolicy(profile), runtimeRecoveryBackoff,
	); err != nil {
		return fmt.Errorf("recover session rollbacks: %w", err)
	}
	return nil
}

func openPreparedSession(
	ctx context.Context,
	cfg Config,
	profile *runtimebinding.Profile,
	configured settings.Config,
	reconnectPolicy retry.ReconnectPolicy,
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
	recovery, recovered, err := authoring.ConsumeConfirmedSessionRollback(opened.Session.ID)
	if err != nil {
		return preparedSession{}, fmt.Errorf("recover session rollback input: %w", err)
	}
	attachments, err := attachment.New(opened.Session.Workspace.Path)
	if err != nil {
		return preparedSession{}, fmt.Errorf("session attachments: %w", err)
	}
	if rememberWorkspaceErr := authoring.RememberWorkspace(opened.Session.Workspace.Path); rememberWorkspaceErr != nil {
		return preparedSession{}, fmt.Errorf("remember workspace: %w", rememberWorkspaceErr)
	}
	draft, _, err := authoring.Draft(opened.Session.ID)
	if err != nil {
		return preparedSession{}, fmt.Errorf("load session draft: %w", err)
	}
	if cfg.InitialPrompt != "" {
		draft = agent.Message{Text: cfg.InitialPrompt}
	}
	editor, err := configuredDraftEditor()
	if err != nil {
		return preparedSession{}, err
	}
	return preparedSession{
		opened: opened, runtimeProfile: profile, attachments: attachments, keyBindings: bindings,
		settings: configured, reconnectPolicy: reconnectPolicy, options: options,
		workbench: authoring, draft: draft, editor: editor,
		rollbackRecovery: optionalRollbackRecovery(recovery, recovered),
	}, nil
}

func commandReplayPolicy(profile *runtimebinding.Profile) commandreplay.Policy {
	policy, err := runtimebinding.CommandReplayPolicy(profile)
	if err != nil {
		return commandreplay.Policy{}
	}
	return policy
}

func commandReplayGuard(profile *runtimebinding.Profile) commandreplay.Guard {
	guard, err := commandReplayPolicy(profile).NewGuard()
	if err != nil {
		return commandreplay.Guard{}
	}
	return guard
}

func commandReplaySafe(guard commandreplay.Guard, profile *runtimebinding.Profile) bool {
	return commandReplaySafeAt(guard, profile, time.Now().UTC())
}

func commandReplaySafeAt(
	guard commandreplay.Guard,
	profile *runtimebinding.Profile,
	now time.Time,
) bool {
	policy, err := runtimebinding.CommandReplayPolicyWithClock(profile, func() time.Time { return now })
	if err != nil {
		return false
	}
	return policy.Replayable(guard)
}

func commandReplayStoreMatches(
	guard commandreplay.Guard,
	profile *runtimebinding.Profile,
) bool {
	return commandReplayPolicy(profile).SameStore(guard)
}

func commandReplayAdmission(
	guard commandreplay.Guard,
	profile *runtimebinding.Profile,
) mutation.Admission {
	return mutation.FreshDynamicReplayAdmission(func() commandreplay.Policy {
		return commandReplayPolicy(profile)
	}, guard)
}

func optionalRollbackRecovery(
	recovery workbench.SessionRollbackRecovery,
	present bool,
) *workbench.SessionRollbackRecovery {
	if !present {
		return nil
	}
	return &recovery
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

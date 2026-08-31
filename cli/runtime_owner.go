package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/delivery/cmd"
	"github.com/Tangerg/flame/cli/internal/application/extensions"
	"github.com/Tangerg/flame/cli/internal/adapter/runtimeadapter"
	"github.com/Tangerg/flame/cli/internal/adapter/runtimeprofile"
	"github.com/Tangerg/flame/cli/internal/delivery/sideload"
	"github.com/Tangerg/flame/cli/internal/delivery/terminal"
)

func newRuntimeOwnerAt(flameHome string) (*runtimeadapter.Owner, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve runtime home: %w", err)
	}
	runtimeDirectory := filepath.Join(filepath.Clean(flameHome), "runtime")
	configDirectories, err := runtimeConfigDirectories(runtimeDirectory)
	if err != nil {
		return nil, err
	}
	return runtimeadapter.NewOwner(runtimeadapter.Config{
		DataDirectory: runtimeDirectory, UserHomePath: userHome,
		ConfigDirectories: configDirectories, ClientVersion: cmd.Version(),
	}), nil
}

func runtimeDependencies(owner *runtimeadapter.Owner, stateDirectory string) cmd.Dependencies {
	return cmd.Dependencies{
		OpenRuntime: func(ctx context.Context) (agent.Runtime, *runtimeprofile.Profile, error) {
			connection, err := owner.Connection(ctx)
			if err != nil {
				return nil, nil, err
			}
			profile := connection.Profile()
			return connection, &profile, nil
		},
		StartTerminal: func(ctx context.Context, request cmd.TerminalRequest) error {
			connection, err := owner.Connection(ctx)
			if err != nil {
				return err
			}
			return startTerminal(ctx, connection, request)
		},
		StateDirectory: stateDirectory,
	}
}

func startTerminal(ctx context.Context, connection *runtimeadapter.Connection, request cmd.TerminalRequest) error {
	profile := connection.Profile()
	configured := request.Settings.Clone()
	cfg := terminal.Config{
		Runtime: connection, RuntimeProfile: &profile,
		Workspaces: connection, Changes: connection, Usage: connection, ModelConfig: connection,
		DiagnosticTools:  connection.DiagnosticToolService(),
		AuthoringContext: connection.AuthoringContextService(), Hooks: connection.HookService(),
		Feedback: connection.FeedbackService(), AgentMemory: connection.AgentMemoryService(),
		Knowledge:     connection.KnowledgeService(),
		ClientVersion: cmd.Version(), SessionID: request.SessionID, Workspace: request.Workspace,
		InitialPrompt: request.InitialPrompt, Settings: &configured,
		PluginSources:  []extensions.Source{sideload.New(configured.Plugins.Directories)},
		StateDirectory: request.StateDirectory,
	}
	if profile.Supports(runtimeprofile.FeatureGoals) {
		cfg.Goals = connection
	}
	if profile.Supports(runtimeprofile.FeatureSkills) {
		cfg.Skills = connection
	}
	if profile.Supports(runtimeprofile.FeatureMCP) {
		cfg.MCP = connection
	}
	if profile.Supports(runtimeprofile.FeatureSchedules) {
		cfg.Schedules = connection
	}
	if profile.Supports(runtimeprofile.FeatureSessionExport) {
		cfg.Transfers = connection
	}
	return terminal.Run(ctx, cfg)
}

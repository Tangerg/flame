package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/cli/internal/application/extensions"
	"github.com/Tangerg/flame/cli/internal/delivery/cmd"
	"github.com/Tangerg/flame/cli/internal/delivery/terminal"
	"github.com/Tangerg/flame/cli/internal/delivery/terminal/sideload"
	"github.com/Tangerg/flame/runtime/protocol"
)

func newRuntimeOwnerAt(flameHome string) (*runtimebinding.Owner, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve runtime home: %w", err)
	}
	return runtimebinding.NewOwner(runtimebinding.Config{
		ProductRoot: flameHome, UserHomePath: userHome,
		ConfigDirectories: runtimeConfigDirectories(), ClientVersion: cmd.Version(),
		RemoteToken: os.Getenv("FLAME_RUNTIME_TOKEN"),
	}), nil
}

func runtimeDependencies(owner *runtimebinding.Owner, stateDirectory string) cmd.Dependencies {
	return cmd.Dependencies{
		OpenRuntime: func(ctx context.Context, endpoint string) (cmd.Runtime, *runtimebinding.Profile, error) {
			connection, err := owner.Connection(ctx, endpoint)
			if err != nil {
				return nil, nil, err
			}
			profile := connection.Profile()
			return connection, &profile, nil
		},
		StartTerminal: func(ctx context.Context, request cmd.TerminalRequest) error {
			connection, err := owner.Connection(ctx, request.Settings.Runtime.Endpoint)
			if err != nil {
				return err
			}
			return startTerminal(ctx, connection, request)
		},
		StateDirectory: stateDirectory,
	}
}

func startTerminal(ctx context.Context, connection *runtimebinding.Connection, request cmd.TerminalRequest) error {
	profile := connection.Profile()
	configured := request.Settings.Clone()
	cfg := terminal.Config{
		Runtime: connection, RuntimeProfile: &profile,
		Workspaces: connection, Changes: connection, Usage: connection, ModelConfig: connection,
		DiagnosticTools:  connection.DiagnosticTools(),
		AuthoringContext: connection.AuthoringContext(), Hooks: connection.Hooks(),
		Feedback:      connection.Feedback(),
		ClientVersion: cmd.Version(), SessionID: request.SessionID, Workspace: request.Workspace,
		InitialPrompt: request.InitialPrompt, Settings: &configured,
		PluginSources:  []extensions.Source{sideload.New(configured.Plugins.Directories)},
		StateDirectory: request.StateDirectory,
	}
	if configured.Runtime.Endpoint != "" {
		cfg.LocalDirectory = request.LocalDirectory
		cfg.DetachOnExit = true
	}
	if profile.Supports(protocol.FeatureGoals) {
		cfg.Goals = connection
	}
	if profile.Supports(protocol.FeatureSkills) {
		cfg.Skills = connection
	}
	if profile.Supports(protocol.FeatureMCP) {
		cfg.MCP = connection
	}
	if profile.Supports(protocol.FeatureSchedules) {
		cfg.Schedules = connection
	}
	if profile.Supports(protocol.FeatureSessionExport) {
		cfg.Transfers = connection
	}
	if profile.Supports(protocol.FeatureAgentMemory) {
		cfg.AgentMemory = connection.AgentMemory()
	}
	return terminal.Run(ctx, cfg)
}

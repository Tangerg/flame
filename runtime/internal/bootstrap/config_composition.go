package bootstrap

import (
	"path/filepath"
	"slices"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/codeintel"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

// ComposeConfig translates process settings and already-opened adapters into
// the construction input consumed by assembly.
func composeConfig(cfg config.Settings, stores *persistence.Bundle, resolver chatResolver, providers models.ProviderRegistry, buildID string) assemblyConfig {
	return assemblyConfig{
		stores:                   stores,
		resources:                []terminalResource{stores},
		buildID:                  buildID,
		chatResolver:             resolver,
		pricing:                  modeladapter.Pricing(),
		skillsUserDir:            filepath.Join(stores.DataDirectory, "skills"),
		a2aAgents:                toolsetA2AAgents(cfg.A2AAgents),
		lspServers:               codeintelServers(cfg.LSPServers),
		sandboxShell:             cfg.SandboxShell,
		sandboxReadOnlyPaths:     cfg.SandboxReadOnlyPaths,
		sandboxDir:               filepath.Join(stores.DataDirectory, "sandbox"),
		providerRegistry:         providers,
		provider:                 cfg.Provider,
		model:                    cfg.Model,
		checkpointDir:            filepath.Join(stores.DataDirectory, "checkpoints"),
		toolResultOffloadEnabled: cfg.ToolResultOffload.Enabled,
		toolResultThreshold:      cfg.ToolResultOffload.Threshold,
		approvalMode:             approval.ModeBalanced,
	}
}

func toolsetA2AAgents(in []config.A2AAgent) []toolset.A2AAgentConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]toolset.A2AAgentConfig, len(in))
	for i, agent := range in {
		out[i] = toolset.A2AAgentConfig{
			Name:              agent.Name,
			CardURL:           agent.CardURL,
			AllowedRPCOrigins: slices.Clone(agent.AllowedRPCOrigins),
		}
	}
	return out
}

func codeintelServers(in []config.LSPServer) []codeintel.ServerSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]codeintel.ServerSpec, len(in))
	for i, server := range in {
		out[i] = codeintel.ServerSpec{
			Name:        server.Name,
			Command:     server.Command,
			Args:        slices.Clone(server.Args),
			LanguageID:  server.LanguageID,
			Extensions:  slices.Clone(server.Extensions),
			RootMarkers: slices.Clone(server.RootMarkers),
		}
	}
	return out
}

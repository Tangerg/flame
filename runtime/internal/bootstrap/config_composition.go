package bootstrap

import (
	"path/filepath"
	"slices"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/codeintel"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

// ComposeConfig translates process settings and already-opened adapters into
// the construction input consumed by assembly.
func ComposeConfig(cfg config.Settings, stores *persistence.Bundle, resolver ChatResolver, providers models.ProviderRegistry, hooks HookResolver, buildID string) Config {
	return Config{
		Stores:                   stores,
		Resources:                []TerminalResource{stores},
		BuildID:                  buildID,
		ChatResolver:             resolver,
		Pricing:                  modeladapter.Pricing(),
		SkillsUserDir:            filepath.Join(stores.DataDirectory, "skills"),
		Online:                   toolset.OnlineConfig(cfg.Online),
		A2AAgents:                toolsetA2AAgents(cfg.A2AAgents),
		LSPServers:               codeintelServers(cfg.LSPServers),
		SandboxShell:             cfg.SandboxShell,
		SandboxReadOnlyPaths:     cfg.SandboxReadOnlyPaths,
		SandboxDir:               filepath.Join(stores.DataDirectory, "sandbox"),
		ProviderRegistry:         providers,
		Provider:                 cfg.Provider,
		Model:                    cfg.Model,
		HooksResolver:            hooks,
		RecipesGlobalDir:         filepath.Join(stores.DataDirectory, "recipes"),
		CheckpointDir:            filepath.Join(stores.DataDirectory, "checkpoints"),
		ToolResultOffloadEnabled: cfg.ToolResultOffload.Enabled,
		ToolResultThreshold:      cfg.ToolResultOffload.Threshold,
		ApprovalMode:             approval.ModeBalanced,
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

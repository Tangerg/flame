package bootstrap

import (
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

// ComposeConfig translates process settings and already-opened adapters into
// the construction input consumed by assembly.
func composeConfig(cfg config.Settings, stores *persistence.Bundle, adapters modelProviderAdapters, providers models.ProviderRegistry, buildID string) assemblyConfig {
	return assemblyConfig{
		stores:                   stores,
		resources:                []terminalResource{stores},
		buildID:                  buildID,
		modelProviders:           adapters,
		skillsUserDir:            filepath.Join(stores.DataDirectory, "skills"),
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

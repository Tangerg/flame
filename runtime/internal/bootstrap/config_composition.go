package bootstrap

import (
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

func composeConfig(cfg config.Settings, stores *persistence.Bundle, providers models.ProviderRegistry, buildID string) assemblyConfig {
	return assemblyConfig{
		stores:                     stores,
		resources:                  []terminalResource{stores},
		buildID:                    buildID,
		skillsUserDir:              filepath.Join(stores.DataDirectory, "skills"),
		sandboxShell:               cfg.SandboxShell,
		sandboxReadOnlyPaths:       cfg.SandboxReadOnlyPaths,
		sandboxDir:                 filepath.Join(stores.DataDirectory, "sandbox"),
		providerRegistry:           providers,
		defaultSelection:           cfg.DefaultSelection,
		checkpointDir:              filepath.Join(stores.DataDirectory, "checkpoints"),
		toolResultOffloadThreshold: cfg.ToolResultOffloadThreshold,
		approvalMode:               approval.ModeBalanced,
	}
}

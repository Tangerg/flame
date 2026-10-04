package execution

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

type InstallationCheckpoints interface {
	PendingCheckpointPayloads(context.Context) ([][]byte, error)
}

// ChangeInstallation serializes release changes with executable admission. It
// reads existing owned manifests and durable checkpoints, never a plugin refcount.
func (i *InteractionExecutor) ChangeInstallation(ctx context.Context, id string, admission plugins.ChangeAdmission, change func() error) error {
	if admission != plugins.RequireQuiescent && admission != plugins.AllowInUse {
		return plugin.ErrInvalid
	}
	i.installationAdmission.Lock()
	defer i.installationAdmission.Unlock()
	if admission == plugins.RequireQuiescent {
		for _, session := range i.sessions.snapshot() {
			session.state.mu.Lock()
			deployments := session.state.deployments
			session.state.mu.Unlock()
			if deployments == nil {
				continue
			}
			for _, manifest := range deployments.manifests {
				for _, dependency := range manifest.Installations {
					if dependency.InstallationID == id {
						return plugin.ErrInUse
					}
				}
				for _, executable := range append(manifest.Clone().Visible, manifest.Deferred...) {
					ref, err := toolset.Identify(executable)
					if err != nil {
						return err
					}
					if ref.Server().Installation() == id {
						return plugin.ErrInUse
					}
				}
			}
		}
		if i.config.InstallationCheckpoints == nil {
			return errors.New("execution: installation checkpoint reader is required")
		}
		payloads, err := i.config.InstallationCheckpoints.PendingCheckpointPayloads(ctx)
		if err != nil {
			return err
		}
		for _, payload := range payloads {
			// This read projects only declared dependencies. Restoration owns
			// validation of the execution tree and unrelated continuation data.
			var state interactionCheckpointDependenciesWire
			if err := json.Unmarshal(payload, &state); err != nil {
				return fmt.Errorf("execution: checkpoint dependencies: %w", err)
			}
			if err := state.Validate(); err != nil {
				return err
			}
			for _, dependency := range state.Installations {
				if dependency.InstallationID == id {
					return plugin.ErrInUse
				}
			}
			for _, binding := range state.ToolBindings {
				ref, err := parseToolBinding(binding)
				if err != nil {
					return err
				}
				if ref.Server().Installation() == id {
					return plugin.ErrInUse
				}
			}
		}
	}
	return change()
}

func parseToolBinding(binding toolConfigurationIdentity) (tool.Ref, error) {
	ref, err := tool.ParseRef(binding.Reference)
	if err != nil {
		return tool.Ref{}, err
	}
	return ref, ref.ValidateFingerprint(binding.SourceFingerprint)
}

func installationDependencies(dependencies []promptsource.InstallationDependency) []installationDependencyWire {
	result := make([]installationDependencyWire, 0, len(dependencies))
	for _, dependency := range dependencies {
		result = append(result, installationDependencyWire{InstallationID: dependency.InstallationID, Digest: dependency.Digest})
	}
	return result
}

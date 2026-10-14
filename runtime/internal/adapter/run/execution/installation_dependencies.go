package execution

import (
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

type InstallationCheckpoints interface {
	PendingCheckpointDependencies(context.Context) ([]plugin.Dependency, error)
}

type InstallationCommands interface {
	PendingCommandDependencies(context.Context) ([]plugin.Dependency, error)
}

// ChangeInstallation applies one installation change at the admission
// serialization point. Quiescence is derived from the dependency projections of
// owned sessions, pending checkpoints and unresolved commands, never from a plugin refcount; change
// receives that same held set, so whatever it reclaims is decided against the
// state that admission serializes. change commits durably and withdraws the
// superseded live tools before it returns, so a session assembled after this
// call observes only the committed state. The installation owner reports an
// execution change only after committing, including when withdrawal then fails.
// Quiescence controls existing dependencies; every execution change invalidates
// unpublished assemblies that depended on the previous state.
func (i *InteractionExecutor) ChangeInstallation(ctx context.Context, id resourceid.InstallationID, admission plugins.ChangeAdmission, change func(held []plugin.Dependency) (executionChanged bool, err error)) error {
	if admission != plugins.RequireQuiescent && admission != plugins.AllowInUse {
		return plugin.ErrInvalid
	}
	return i.installations.Change(ctx, id, func() (bool, error) {
		held, err := i.heldDependencies(ctx)
		if err != nil {
			return false, err
		}
		if admission == plugins.RequireQuiescent && slices.ContainsFunc(held, func(dependency plugin.Dependency) bool { return dependency.InstallationID == id }) {
			return false, plugin.ErrInUse
		}
		return change(held)
	})
}

// UnderAdmission runs inspect at the admission serialization point with the
// held releases, without changing any installation.
func (i *InteractionExecutor) UnderAdmission(ctx context.Context, inspect func(held []plugin.Dependency) error) error {
	return i.installations.Under(ctx, func() error {
		held, err := i.heldDependencies(ctx)
		if err != nil {
			return err
		}
		return inspect(held)
	})
}

// heldDependencies is the canonical union of every owned session's dependency
// projection, pending checkpoint and unresolved command. The serialization point is held.
func (i *InteractionExecutor) heldDependencies(ctx context.Context) ([]plugin.Dependency, error) {
	if i.config.InstallationCheckpoints == nil {
		return nil, errors.New("execution: installation checkpoint reader is required")
	}
	held, err := i.config.InstallationCheckpoints.PendingCheckpointDependencies(ctx)
	if err != nil {
		return nil, err
	}
	commands, err := i.config.InstallationCommands.PendingCommandDependencies(ctx)
	if err != nil {
		return nil, err
	}
	held = append(held, commands...)
	if i.config.ModelComposition != nil {
		held = append(held, i.config.ModelComposition.HeldDependencies()...)
	}
	for _, session := range i.sessions.snapshot() {
		held = append(held, session.installationDependencies()...)
	}
	return plugin.CompactDependencies(held), nil
}

// installationDependencies is the session's single dependency projection: the
// canonical union of its frozen manifests' installation releases.
func (i *interactionSession) installationDependencies() []plugin.Dependency {
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	var dependencies []plugin.Dependency
	for _, binding := range append(append(slices.Clone(i.modelObservers), i.modelContexts...), i.modelInputs...) {
		dependencies = append(dependencies, plugin.Dependency{InstallationID: binding.InstallationID, Digest: binding.Digest})
	}
	if deployments != nil {
		for _, manifest := range deployments.manifests {
			dependencies = append(dependencies, manifest.Installations...)
		}
	}
	return plugin.CompactDependencies(dependencies)
}

func installationDependencies(dependencies []plugin.Dependency) []installationDependencyWire {
	result := make([]installationDependencyWire, 0, len(dependencies))
	for _, dependency := range dependencies {
		result = append(result, installationDependencyWire{InstallationID: dependency.InstallationID.String(), Digest: dependency.Digest.String()})
	}
	return result
}

package execution

import (
	"context"
	"github.com/Tangerg/flame/qimu"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// Held projects only live business owners. Durable checkpoint and command
// references belong to the SQLite owner, which reads their original bindings.
func (i *InteractionExecutor) ID() string { return "flame.executions" }

func (i *InteractionExecutor) Held(ctx context.Context, id qimu.InstallationID) ([]qimu.Dependency, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var held []plugin.Dependency
	if i.config.CallComposition != nil {
		held = append(held, i.config.CallComposition.HeldDependencies()...)
	}
	for _, session := range i.sessions.snapshot() {
		held = append(held, session.installationDependencies()...)
	}
	var result []qimu.Dependency
	for _, dependency := range held {
		if dependency.InstallationID.String() == id.String() {
			result = append(result, qimu.Dependency{InstallationID: id, Release: qimu.Digest(dependency.Digest.String())})
		}
	}
	return qimu.CompactDependencies(result), nil
}

// installationDependencies is the session's single dependency projection: the
// canonical union of its frozen manifests' installation releases.
func (i *interactionSession) installationDependencies() []plugin.Dependency {
	i.state.mu.Lock()
	deployments := i.state.deployments
	var dependencies []plugin.Dependency
	for _, activation := range i.compositionActivations {
		dependencies = append(dependencies, activation.Dependencies...)
	}
	i.state.mu.Unlock()
	if deployments != nil {
		for _, manifest := range deployments.manifests {
			dependencies = append(dependencies, manifest.Installations...)
		}
	}
	return plugin.CompactDependencies(dependencies)
}

func (i *interactionSession) checkpointPluginReferences() ([]plugin.Activation, []plugin.Dependency) {
	i.state.mu.Lock()
	deployments := i.state.deployments
	var bindings []plugin.Activation
	for _, activation := range i.compositionActivations {
		bindings = append(bindings, activation.Clone())
	}
	i.state.mu.Unlock()
	var resources []plugin.Dependency
	if deployments != nil {
		for _, manifest := range deployments.manifests {
			for _, activation := range manifest.Bindings {
				if !slices.ContainsFunc(bindings, activation.Equal) {
					bindings = append(bindings, activation.Clone())
				}
			}
			resources = append(resources, manifest.Resources...)
		}
	}
	return bindings, plugin.CompactDependencies(resources)
}

func installationDependencies(dependencies []plugin.Dependency) []installationDependencyWire {
	result := make([]installationDependencyWire, 0, len(dependencies))
	for _, dependency := range dependencies {
		result = append(result, installationDependencyWire{InstallationID: dependency.InstallationID.String(), Digest: dependency.Digest.String()})
	}
	return result
}

// dependencyInstallations projects the original business binding at Qimu admission.
func dependencyInstallations(dependencies []plugin.Dependency) []qimu.InstallationID {
	result := make([]qimu.InstallationID, 0, len(dependencies))
	for _, dependency := range dependencies {
		result = append(result, qimu.InstallationID(dependency.InstallationID.String()))
	}
	return result
}

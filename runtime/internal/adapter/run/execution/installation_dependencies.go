package execution

import (
	"context"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
)

func (i *InteractionExecutor) ID() string { return "flame.executions" }

func (i *InteractionExecutor) Held(ctx context.Context, id framehost.InstallationID) ([]framevalue.Digest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	held := i.config.CallComposition.HeldDependencies()
	for _, session := range i.sessions.snapshot() {
		held = append(held, session.installationDependencies()...)
	}
	var releases []framevalue.Digest
	for _, dependency := range held {
		if dependency.InstallationID.Host() == id {
			releases = append(releases, dependency.Digest)
		}
	}
	return releases, nil
}

func (i *interactionSession) installationDependencies() []plugin.Dependency {
	i.state.mu.Lock()
	deployments := i.state.deployments
	var dependencies []plugin.Dependency
	for _, activation := range i.compositionActivations {
		dependencies = append(dependencies, activation.Dependencies...)
	}
	i.state.mu.Unlock()
	if deployments != nil {
		for _, group := range deployments.toolGroups {
			dependencies = append(dependencies, group.manifest.Installations...)
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
		for _, group := range deployments.toolGroups {
			for _, activation := range group.manifest.Bindings {
				if !slices.ContainsFunc(bindings, activation.Equal) {
					bindings = append(bindings, activation.Clone())
				}
			}
			resources = append(resources, group.manifest.Resources...)
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

func (i *InteractionExecutor) publishInteraction(ctx context.Context, session *interactionSession) error {
	bindings, resources := session.checkpointPluginReferences()
	return i.config.Admission.Admit(ctx, func(admitted context.Context) error {
		if err := i.config.ValidatePluginSources(admitted, bindings, resources); err != nil {
			return err
		}
		return i.sessions.register(session)
	})
}

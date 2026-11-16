package testsupport

import (
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
)

// ServerActivation creates exact immutable routing evidence for a test server.
// It does not create a running instance or claim that any Host admitted it.
func ServerActivation(configuration, server string, dependencies ...plugin.Dependency) plugin.Activation {
	dependencies = plugin.CompactDependencies(dependencies)
	binding := framehost.BindingSnapshot{Roots: []framehost.ExportRef{}, UnitRoots: []framehost.UnitRef{}, Units: []framehost.UnitBinding{}}
	for _, dependency := range dependencies {
		source := framehost.UnitRef{InstallationID: dependency.InstallationID.Host(), Release: dependency.Digest, UnitID: framehost.UnitID("backend/" + server)}
		binding.UnitRoots = append(binding.UnitRoots, source)
		binding.Units = append(binding.Units, framehost.UnitBinding{Source: source, Configuration: framevalue.Hash([]byte(configuration)), Imports: []framehost.ImportBinding{}})
	}
	data, err := binding.CanonicalJSON()
	if err != nil {
		panic(err)
	}
	value, err := framevalue.ParseJSON(data)
	if err != nil {
		panic(err)
	}
	digest := framevalue.Hash(data)
	return plugin.Activation{Binding: value, Identity: digest, Dependencies: dependencies}
}

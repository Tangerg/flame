package testsupport

import (
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"
	framevalue "github.com/Tangerg/frame/value"
)

// ServerActivation creates exact immutable routing evidence for a test server.
// It does not create a running instance or claim that any Host admitted it.
func ServerActivation(configuration, server string, dependencies ...plugin.Dependency) plugin.Activation {
	dependencies = plugin.CompactDependencies(dependencies)
	binding := frame.BindingSnapshot{FormatVersion: 1, Roots: []frame.ExportRef{}, UnitRoots: []frame.UnitRef{}, Units: []frame.UnitBinding{}}
	for _, dependency := range dependencies {
		source := frame.UnitRef{InstallationID: dependency.InstallationID.Host(), Release: dependency.Digest, UnitID: frame.UnitID("backend/" + server)}
		binding.UnitRoots = append(binding.UnitRoots, source)
		binding.Units = append(binding.Units, frame.UnitBinding{Source: source, Configuration: framevalue.Hash([]byte(configuration)), Imports: []frame.ImportDeclaration{}})
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

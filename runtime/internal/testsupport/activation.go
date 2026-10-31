package testsupport

import (
	"github.com/Tangerg/qimu"
	"github.com/Tangerg/qimu/integrity"
	"github.com/Tangerg/qimu/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// ServerActivation creates exact immutable routing evidence for a test server.
// It does not create a running instance or claim that any Host admitted it.
func ServerActivation(configuration, server string, dependencies ...plugin.Dependency) plugin.Activation {
	dependencies = plugin.CompactDependencies(dependencies)
	binding := qimu.BindingSnapshot{FormatVersion: 1, Roots: []qimu.ExportRef{}, UnitRoots: []qimu.UnitRef{}, Units: []qimu.UnitBinding{}}
	for _, dependency := range dependencies {
		source := qimu.UnitRef{InstallationID: qimu.InstallationID(dependency.InstallationID.String()), Release: dependency.Digest, UnitID: qimu.UnitID("backend/" + server)}
		binding.UnitRoots = append(binding.UnitRoots, source)
		binding.Units = append(binding.Units, qimu.UnitBinding{Source: source, Configuration: integrity.Hash([]byte(configuration)), Imports: []qimu.ImportDeclaration{}})
	}
	data, err := binding.CanonicalJSON()
	if err != nil {
		panic(err)
	}
	value, err := jsonvalue.Parse(data)
	if err != nil {
		panic(err)
	}
	digest, err := integrity.ParseDigest(integrity.Hash(data).String())
	if err != nil {
		panic(err)
	}
	return plugin.Activation{Binding: value, Identity: digest, Dependencies: dependencies}
}

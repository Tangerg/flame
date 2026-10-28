package testsupport

import (
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// ServerActivation creates exact immutable routing evidence for a test server.
// It does not create a running instance or claim that any Host admitted it.
func ServerActivation(configuration, server string, dependencies ...plugin.Dependency) plugin.Activation {
	dependencies = plugin.CompactDependencies(dependencies)
	binding := qimu.BindingSnapshot{FormatVersion: 1, Roots: []qimu.ExportRef{}, UnitRoots: []qimu.UnitRef{}, Units: []qimu.UnitBinding{}}
	for _, dependency := range dependencies {
		source := qimu.UnitRef{InstallationID: qimu.InstallationID(dependency.InstallationID.String()), Release: qimu.Digest(dependency.Digest.String()), UnitID: qimu.UnitID("backend/" + server)}
		binding.UnitRoots = append(binding.UnitRoots, source)
		binding.Units = append(binding.Units, qimu.UnitBinding{Source: source, Configuration: qimu.Hash([]byte(configuration)), Imports: []qimu.ImportDeclaration{}})
	}
	data, err := binding.CanonicalJSON()
	if err != nil {
		panic(err)
	}
	value, err := jsonvalue.Parse(data)
	if err != nil {
		panic(err)
	}
	digest, err := identity.ParseDigest(qimu.Hash(data).String())
	if err != nil {
		panic(err)
	}
	return plugin.Activation{Binding: value, Identity: digest, Dependencies: dependencies}
}

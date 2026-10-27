package toolset

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// Retained supplies the original execution's authority before discovery or
// collision resolution. Its graphs belong to that execution's checkpoint;
// restoring them never publishes them as the current catalog for new work.
type Retained struct {
	MCP       []domaintool.Ref
	Bindings  []plugin.Activation
	Resources []plugin.Dependency
}

func (r *Retained) validate() error {
	if r == nil {
		return nil
	}
	for index, ref := range r.MCP {
		if _, _, valid := ref.MCP(); !valid || slices.Contains(r.MCP[:index], ref) {
			return fmt.Errorf("%w: retained MCP references must be distinct", plugin.ErrInvalid)
		}
	}
	for _, binding := range r.Bindings {
		if _, err := plugins.DecodeActivation(binding); err != nil {
			return err
		}
	}
	return plugin.ValidateDependencies(r.Resources)
}

func (r *Retained) validateManifest(manifest Manifest) error {
	if r == nil {
		return nil
	}
	for _, resource := range manifest.Resources {
		if !slices.Contains(r.Resources, resource) {
			return fmt.Errorf("%w: captured content source changed", plugin.ErrStale)
		}
	}
	for _, binding := range manifest.Bindings {
		if !slices.ContainsFunc(r.Bindings, binding.Equal) {
			return fmt.Errorf("%w: restored executable has a different setup binding", plugin.ErrStale)
		}
	}
	return nil
}

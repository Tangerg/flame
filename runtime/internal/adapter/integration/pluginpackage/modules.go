package pluginpackage

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/distribution"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type moduleGraph struct {
	SchemaVersion    int           `json:"schemaVersion"`
	Entry            string        `json:"entry"`
	Modules          []moduleEntry `json:"modules"`
	SharedSpecifiers []string      `json:"sharedSpecifiers"`
}
type moduleEntry struct {
	Path           string   `json:"path"`
	Imports        []string `json:"imports"`
	DynamicImports []string `json:"dynamicImports"`
	Exports        []string `json:"exports"`
}

func componentModule(ctx context.Context, root *os.Root, component plugin.ClientComponent) (*distribution.Module, error) {
	directory := path.Dir(component.Entry)
	encoded, err := read(ctx, root, path.Join(directory, "modules.generated.json"), maxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: component module graph: %w", plugin.ErrInvalid, err)
	}
	var graph moduleGraph
	if err := json.Unmarshal(encoded, &graph, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("%w: component module graph: %w", plugin.ErrInvalid, err)
	}
	if graph.SchemaVersion != 1 || graph.Entry != path.Base(component.Entry) || len(graph.Modules) == 0 {
		return nil, plugin.ErrInvalid
	}
	modules := map[string]moduleEntry{}
	for _, module := range graph.Modules {
		if !plugin.ValidResourcePath(module.Path) || modules[module.Path].Path != "" {
			return nil, plugin.ErrInvalid
		}
		modules[module.Path] = module
	}
	entry, found := modules[graph.Entry]
	if !found || !slices.Contains(entry.Exports, component.Export) {
		return nil, fmt.Errorf("%w: component export differs from compiled module graph", plugin.ErrInvalid)
	}
	shared := map[string]bool{}
	for _, module := range graph.Modules {
		for _, dependency := range append(slices.Clone(module.Imports), module.DynamicImports...) {
			if _, found := modules[dependency]; found {
				continue
			}
			if !slices.Contains(hostModuleSpecifiers[:], dependency) {
				return nil, fmt.Errorf("%w: component shared module is outside host profile", plugin.ErrInvalid)
			}
			shared[dependency] = true
		}
	}
	if len(shared) != len(graph.SharedSpecifiers) {
		return nil, plugin.ErrInvalid
	}
	for _, name := range graph.SharedSpecifiers {
		if !shared[name] {
			return nil, plugin.ErrInvalid
		}
		delete(shared, name)
	}
	if len(shared) != 0 {
		return nil, plugin.ErrInvalid
	}
	result := &distribution.Module{Entry: component.Entry, Imports: []distribution.ModuleImport{}}
	for _, module := range graph.Modules {
		if module.Path != graph.Entry {
			result.Imports = append(result.Imports, distribution.ModuleImport{Specifier: "./" + module.Path, Path: path.Join(directory, module.Path)})
		}
	}
	for _, name := range graph.SharedSpecifiers {
		result.Imports = append(result.Imports, distribution.ModuleImport{Specifier: name, Shared: &distribution.HostModuleReference{Profile: hostModuleProfile}})
	}
	slices.SortFunc(result.Imports, func(a, b distribution.ModuleImport) int { return strings.Compare(a.Specifier, b.Specifier) })
	return result, nil
}

// ComponentModules supplies the exact native shared profile and complete local
// module closure to Flame's browser bootstrap without forwarding private config.
func (r *Releases) ComponentModules(ctx context.Context, release plugin.Release, id plugin.ContributionID) (*distribution.Module, error) {
	lease, err := r.content.Acquire(ctx, qimu.Digest(release.Digest().String()))
	if err != nil {
		return nil, err
	}
	defer lease.Close()
	for _, unit := range lease.Release().Manifest.Units {
		if unit.ID == plugins.ComponentUnit(id) && unit.Module != nil {
			return unit.Module, nil
		}
	}
	return nil, plugin.ErrNotFound
}

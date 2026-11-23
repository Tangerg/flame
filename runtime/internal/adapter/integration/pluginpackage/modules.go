package pluginpackage

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame/distribution"
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
	encoded, err := read(ctx, root, path.Join(directory, moduleGraphFilename), maxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: component module graph: %w", plugin.ErrInvalid, err)
	}
	var graph moduleGraph
	if err := json.Unmarshal(encoded, &graph, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("%w: component module graph: %w", plugin.ErrInvalid, err)
	}
	if err := graph.validate(component); err != nil {
		return nil, err
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

func (g moduleGraph) importedHostModules(local map[string]moduleEntry) (map[string]bool, error) {
	shared := make(map[string]bool)
	for _, module := range g.Modules {
		for _, dependency := range slices.Concat(module.Imports, module.DynamicImports) {
			if _, found := local[dependency]; found {
				continue
			}
			if !slices.Contains(hostModuleSpecifiers[:], dependency) {
				return nil, fmt.Errorf("%w: component shared module is outside host profile", plugin.ErrInvalid)
			}
			shared[dependency] = true
		}
	}
	return shared, nil
}

func (g moduleGraph) validate(component plugin.ClientComponent) error {
	if g.SchemaVersion != moduleGraphSchemaVersion || g.Entry != path.Base(component.Entry) || len(g.Modules) == 0 {
		return plugin.ErrInvalid
	}
	modules := make(map[string]moduleEntry, len(g.Modules))
	for _, module := range g.Modules {
		if _, duplicate := modules[module.Path]; duplicate || !plugin.ValidResourcePath(module.Path) {
			return plugin.ErrInvalid
		}
		modules[module.Path] = module
	}
	entry, found := modules[g.Entry]
	if !found || !slices.Contains(entry.Exports, component.Export) {
		return fmt.Errorf("%w: component export differs from compiled module graph", plugin.ErrInvalid)
	}
	shared, err := g.importedHostModules(modules)
	if err != nil {
		return err
	}
	declared := make(map[string]bool, len(g.SharedSpecifiers))
	for _, name := range g.SharedSpecifiers {
		if declared[name] {
			return plugin.ErrInvalid
		}
		declared[name] = true
	}
	if !maps.Equal(shared, declared) {
		return plugin.ErrInvalid
	}
	return nil
}

func (r *Releases) ComponentModules(ctx context.Context, release plugin.Release, id plugin.ContributionID) (_ *distribution.Module, returned error) {
	lease, err := r.content.Acquire(ctx, release.Digest())
	if err != nil {
		return nil, err
	}
	defer func() { returned = errors.Join(returned, lease.Close()) }()
	for _, unit := range lease.Snapshot().Manifest.Units {
		if unit.ID == plugins.ComponentUnit(id) && unit.Module != nil {
			return unit.Module, nil
		}
	}
	return nil, plugin.ErrNotFound
}

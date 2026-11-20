package pluginpackage

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/frame/distribution"
	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
)

type BuildTarget struct {
	OS   string
	Arch string
}

// A packaged native backend runs only where Flame ships; this is the sole
// statement of that set, and build scripts defer to it.
var (
	nativeOperatingSystems = []string{"darwin", "linux", "windows"}
	nativeArchitectures    = []string{"amd64", "arm64"}
)

func (t BuildTarget) native() bool {
	return slices.Contains(nativeOperatingSystems, t.OS) && slices.Contains(nativeArchitectures, t.Arch)
}

// BuildRelease is the author boundary shared by the repository builder and its
// independent executable. It parses metadata without running package code and
// uses the same declaration projection that Runtime installs.
func BuildRelease(ctx context.Context, source string, target BuildTarget, archive io.Writer) (_ distribution.Release, err error) {
	if !filepath.IsAbs(source) || archive == nil {
		return distribution.Release{}, plugin.ErrInvalid
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return distribution.Release{}, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	builder, err := parseMetadata(ctx, root)
	if err != nil {
		return distribution.Release{}, err
	}
	declaration, err := builder.Declaration()
	if err != nil {
		return distribution.Release{}, err
	}
	projection, err := plugins.ProjectFrameworkDeclaration(declaration, pluginmcp.ProcessProfile)
	if err != nil {
		return distribution.Release{}, err
	}
	manifest, err := authorManifest(ctx, root, declaration, projection, target)
	if err != nil {
		return distribution.Release{}, err
	}
	manifest.Artifacts, err = inventoryArtifacts(ctx, root, manifest)
	if err != nil {
		return distribution.Release{}, err
	}
	lock, err := distribution.Build(ctx, source, manifest, archive)
	if err != nil {
		return distribution.Release{}, err
	}
	manifest.Artifacts = slices.Clone(lock.Artifacts)
	manifest.Release = lock.Release
	if err := writeRecord(root, distribution.ManifestName, manifest); err != nil {
		return distribution.Release{}, err
	}
	if err := writeRecord(root, distribution.LockName, lock); err != nil {
		return distribution.Release{}, err
	}
	return distribution.Release{Digest: lock.Release, Manifest: manifest, Lock: lock}, nil
}

// inventoryArtifacts lists every packaged file. A native variant entry must
// already be executable, because the archive preserves the author's mode.
func inventoryArtifacts(ctx context.Context, root *os.Root, manifest distribution.Manifest) ([]distribution.Artifact, error) {
	nativeEntries := make(map[string]struct{})
	for _, unit := range manifest.Units {
		for _, variant := range unit.Variants {
			if variant.Runtime == nil {
				nativeEntries[variant.Path] = struct{}{}
			}
		}
	}
	var artifacts []distribution.Artifact
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." || entry.IsDir() || name == distribution.ManifestName || name == distribution.LockName {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: non-regular artifact %q", plugin.ErrInvalid, name)
		}
		executable := info.Mode().Perm()&0111 != 0
		if _, native := nativeEntries[name]; native {
			if !executable && runtime.GOOS != "windows" {
				return fmt.Errorf("%w: native artifact %q must have executable permissions", plugin.ErrInvalid, name)
			}
			executable = true
		}
		artifacts = append(artifacts, distribution.Artifact{Path: name, Executable: executable})
		return nil
	})
	return artifacts, err
}

func writeRecord(root *os.Root, name string, record any) error {
	encoded, err := json.Marshal(record, jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	return root.WriteFile(name, append(encoded, '\n'), 0600)
}

func authorManifest(ctx context.Context, root *os.Root, declaration plugin.Declaration, projection plugins.FrameworkDefinition, target BuildTarget) (distribution.Manifest, error) {
	manifest := distribution.Manifest{Package: declaration.Name, Version: declaration.Version, Artifacts: []distribution.Artifact{}, Units: []distribution.Unit{}}
	if manifest.Version == "" {
		return manifest, fmt.Errorf("%w: a release version is required", plugin.ErrInvalid)
	}
	for _, contract := range projection.Contracts {
		manifest.Contracts = append(manifest.Contracts, contract.Ref())
	}
	for _, projected := range projection.Units {
		declared := projected.Declaration
		unit := distribution.Unit{ID: declared.ID, Profile: declared.Profile, Exports: slices.Clone(declared.Exports), Imports: slices.Clone(declared.Imports)}
		switch unit.Profile {
		case pluginmcp.ProcessProfile:
			variants, err := processVariants(declaration, unit.ID, target)
			if err != nil {
				return manifest, err
			}
			unit.Variants = variants
		case framehost.BrowserJS:
			index := slices.IndexFunc(declaration.Components, func(component plugin.ClientComponent) bool { return plugins.ComponentUnit(component.ID) == unit.ID })
			if index < 0 {
				return manifest, plugin.ErrInvalid
			}
			module, err := componentModule(ctx, root, declaration.Components[index])
			if err != nil {
				return manifest, err
			}
			unit.Module = module
		case framehost.NativeGo:
		default:
			return manifest, fmt.Errorf("%w: unsupported Flame profile %s", plugin.ErrInvalid, unit.Profile)
		}
		manifest.Units = append(manifest.Units, unit)
		manifest.Contributions = append(manifest.Contributions, pointContributions(declaration, unit)...)
	}
	return manifest, nil
}

// processVariants pins a managed backend to every shipped Node runtime, and a
// native backend to the single target it was compiled for.
func processVariants(declaration plugin.Declaration, unitID framehost.UnitID, target BuildTarget) ([]distribution.Variant, error) {
	name, err := plugins.BackendServer(unitID)
	if err != nil {
		return nil, err
	}
	index := slices.IndexFunc(declaration.Servers, func(server plugin.Server) bool { return server.Name == name })
	if index < 0 {
		return nil, plugin.ErrInvalid
	}
	server := declaration.Servers[index]
	managed, ok := server.Startup.(plugin.ManagedBackend)
	if !ok {
		path, found := server.PackagedCommand()
		if !found {
			return nil, fmt.Errorf("%w: installed process entry must be a packaged executable", plugin.ErrInvalid)
		}
		if !target.native() {
			return nil, fmt.Errorf("%w: native package target is required", plugin.ErrInvalid)
		}
		return []distribution.Variant{{OS: target.OS, Arch: target.Arch, Path: path}}, nil
	}
	targets, err := localruntime.NodeTargets()
	if err != nil {
		return nil, err
	}
	variants := make([]distribution.Variant, 0, len(targets))
	for _, selected := range targets {
		executable, err := framevalue.ParseDigest(selected.ExecutableDigest)
		if err != nil {
			return nil, fmt.Errorf("node %s/%s executable: %w", selected.OS, selected.Arch, err)
		}
		variants = append(variants, distribution.Variant{
			OS: selected.OS, Arch: selected.Arch, Path: managed.Entry,
			Runtime: &distribution.RuntimeReference{Name: string(managed.Runtime), Version: selected.Version, Digest: executable},
		})
	}
	return variants, nil
}

// pointContributions exports each point a unit implements. Only the point
// values unit carries declared metadata; every other contribution's is null.
func pointContributions(declaration plugin.Declaration, unit distribution.Unit) []distribution.Contribution {
	var contributions []distribution.Contribution
	for _, exported := range unit.Exports {
		if exported.Contract.Kind != framehost.PointKind {
			continue
		}
		metadata := []byte("null")
		if unit.ID == plugins.PointValuesUnit {
			for _, value := range declaration.PointValues {
				if value.ID.String() == exported.Name {
					metadata = value.Value.Bytes()
					break
				}
			}
		}
		contributions = append(contributions, distribution.Contribution{Unit: unit.ID, Export: exported.Name, Point: exported.Contract, Key: exported.Name, Metadata: metadata})
	}
	return contributions
}

// validateProjection prevents consumer metadata from declaring a second set of
// executable units. Runtime may apply Flame policy only to this original graph.
func validateProjection(ctx context.Context, root *os.Root, release plugin.Release, manifest distribution.Manifest) error {
	projection, err := plugins.ProjectFrameworkDeclaration(release.Declaration(), pluginmcp.ProcessProfile)
	if err != nil {
		return err
	}
	// A Flame native build targets one OS/architecture. This is an input to
	// the sole author projection, not a second interpretation of unit entries.
	var target BuildTarget
	for _, unit := range manifest.Units {
		if unit.Profile != pluginmcp.ProcessProfile {
			continue
		}
		for _, variant := range unit.Variants {
			if variant.Runtime != nil {
				continue
			}
			if len(unit.Variants) != 1 {
				return fmt.Errorf("%w: a native Flame build has one target", plugin.ErrInvalid)
			}
			current := BuildTarget{OS: variant.OS, Arch: variant.Arch}
			if target != (BuildTarget{}) && target != current {
				return fmt.Errorf("%w: inconsistent native build targets", plugin.ErrInvalid)
			}
			target = current
		}
	}
	expected, err := authorManifest(ctx, root, release.Declaration(), projection, target)
	if err != nil {
		return err
	}
	// Frame has already verified the original artifact inventory. Reuse those
	// exact facts while comparing every declaration, entry, pin and contribution
	// through Frame's canonical identity, including extension metadata.
	expected.Artifacts = manifest.Artifacts
	identity, err := expected.Identity()
	if err != nil {
		return err
	}
	actual, err := manifest.Identity()
	if err != nil {
		return err
	}
	if identity != actual {
		return fmt.Errorf("%w: neutral declaration differs from Flame projection", plugin.ErrInvalid)
	}
	return nil
}

func (*Releases) Project(release plugin.Release) (plugins.FrameworkRelease, error) {
	return plugins.ProjectFrameworkRelease(release, pluginmcp.ProcessProfile)
}

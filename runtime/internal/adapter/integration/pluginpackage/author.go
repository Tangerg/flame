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

	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/distribution"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type BuildTarget struct {
	OS   string
	Arch string
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
	projection, err := plugins.ProjectFrameworkDeclaration(declaration)
	if err != nil {
		return distribution.Release{}, err
	}
	manifest, err := authorManifest(ctx, root, declaration, projection, target)
	if err != nil {
		return distribution.Release{}, err
	}
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." || entry.IsDir() {
			return nil
		}
		if name == distribution.ManifestName || name == distribution.LockName {
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
		for _, unit := range manifest.Units {
			for _, variant := range unit.Variants {
				if variant.Path == name && variant.Runtime == nil {
					if !executable && runtime.GOOS != "windows" {
						return fmt.Errorf("%w: native artifact %q must have executable permissions", plugin.ErrInvalid, name)
					}
					executable = true
				}
			}
		}
		manifest.Artifacts = append(manifest.Artifacts, distribution.Artifact{Path: name, Executable: executable})
		return nil
	})
	if err != nil {
		return distribution.Release{}, err
	}
	lock, err := distribution.Build(ctx, source, manifest, archive)
	if err != nil {
		return distribution.Release{}, err
	}
	manifest.Artifacts = slices.Clone(lock.Artifacts)
	manifest.Release = lock.Release
	for _, value := range []struct {
		name  string
		value any
	}{{distribution.ManifestName, manifest}, {distribution.LockName, lock}} {
		encoded, err := json.Marshal(value.value, jsontext.WithIndent("  "))
		if err != nil {
			return distribution.Release{}, err
		}
		if err := root.WriteFile(value.name, append(encoded, '\n'), 0600); err != nil {
			return distribution.Release{}, err
		}
	}
	return distribution.Release{Digest: lock.Release, Manifest: manifest, Lock: lock}, nil
}

func authorManifest(ctx context.Context, root *os.Root, declaration plugin.Declaration, projection plugins.FrameworkDefinition, target BuildTarget) (distribution.Manifest, error) {
	manifest := distribution.Manifest{SchemaVersion: distribution.ManifestSchemaVersion, Package: declaration.Name, Version: declaration.Version, Artifacts: []distribution.Artifact{}, Units: []distribution.Unit{}}
	if manifest.Version == "" {
		return manifest, fmt.Errorf("%w: a release version is required", plugin.ErrInvalid)
	}
	for _, contract := range projection.Contracts {
		manifest.Contracts = append(manifest.Contracts, contract.Reference)
	}
	for _, projected := range projection.Units {
		declarationUnit := projected.Declaration
		unit := distribution.Unit{ID: declarationUnit.ID, Profile: declarationUnit.Profile, Exports: slices.Clone(declarationUnit.Exports)}
		for _, imported := range declarationUnit.Imports {
			unit.Imports = append(unit.Imports, distribution.Import{Slot: imported.Slot, Contract: imported.Contract, Optional: imported.Optional})
		}
		switch unit.Profile {
		case qimu.MCPProcess:
			name, err := plugins.BackendServer(unit.ID)
			if err != nil {
				return manifest, err
			}
			index := slices.IndexFunc(declaration.Servers, func(server plugin.Server) bool { return server.Name == name })
			if index < 0 {
				return manifest, plugin.ErrInvalid
			}
			server := declaration.Servers[index]
			if server.Managed != nil {
				targets, err := localruntime.NodeTargets()
				if err != nil {
					return manifest, err
				}
				for _, selected := range targets {
					unit.Variants = append(unit.Variants, distribution.Variant{OS: selected.OS, Arch: selected.Arch, Path: server.Managed.Entry, Runtime: &distribution.RuntimeReference{Name: "node", Version: selected.Version, Digest: qimu.Digest(selected.ExecutableDigest)}})
				}
			} else {
				path, found := server.PackagedCommand()
				if !found {
					return manifest, fmt.Errorf("%w: installed process entry must be a packaged executable", plugin.ErrInvalid)
				}
				if !slices.Contains([]string{"darwin", "linux", "windows"}, target.OS) || !slices.Contains([]string{"amd64", "arm64"}, target.Arch) {
					return manifest, fmt.Errorf("%w: native package target is required", plugin.ErrInvalid)
				}
				unit.Variants = []distribution.Variant{{OS: target.OS, Arch: target.Arch, Path: path}}
			}
		case qimu.BrowserJS:
			index := slices.IndexFunc(declaration.Components, func(component plugin.ClientComponent) bool { return plugins.ComponentUnit(component.ID) == unit.ID })
			if index < 0 {
				return manifest, plugin.ErrInvalid
			}
			module, err := componentModule(ctx, root, declaration.Components[index])
			if err != nil {
				return manifest, err
			}
			unit.Module = module
		case qimu.NativeGo:
		default:
			return manifest, fmt.Errorf("%w: unsupported Flame profile %s", plugin.ErrInvalid, unit.Profile)
		}
		manifest.Units = append(manifest.Units, unit)
		for _, exported := range unit.Exports {
			if exported.Contract.Kind != qimu.PointKind {
				continue
			}
			metadata := []byte("null")
			if unit.ID == plugins.PointValuesUnit {
				for _, value := range declaration.PointValues {
					if value.ID.String() == exported.Name {
						metadata = value.Value.JSON()
						break
					}
				}
			}
			manifest.Contributions = append(manifest.Contributions, distribution.Contribution{Unit: unit.ID, Export: exported.Name, Point: exported.Contract, Key: exported.Name, Metadata: metadata})
		}
	}
	return manifest, nil
}

// validateProjection prevents consumer metadata from declaring a second set of
// executable units. Runtime may apply Flame policy only to this original graph.
func validateProjection(ctx context.Context, root *os.Root, release plugin.Release, manifest distribution.Manifest) error {
	projection, err := plugins.ProjectFrameworkDeclaration(release.Declaration())
	if err != nil {
		return err
	}
	// A Flame native build targets one OS/architecture. This is an input to
	// the sole author projection, not a second interpretation of unit entries.
	var target BuildTarget
	for _, unit := range manifest.Units {
		if unit.Profile != qimu.MCPProcess {
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
	// Qimu has already verified the original artifact inventory. Reuse those
	// exact facts while comparing every declaration, entry, pin and contribution
	// through Qimu's canonical identity, including extension metadata.
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

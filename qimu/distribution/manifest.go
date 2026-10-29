// Package distribution validates immutable plugin releases. It never chooses a
// provider or grants an installation permission to execute.
package distribution

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/jsonvalue"
)

const ManifestName = "manifest.json"
const LockName = "plugin.lock.json"

const ManifestSchemaVersion = 1
const LockSchemaVersion = 1

var (
	ErrInvalid   = errors.New("distribution: invalid release")
	ErrIntegrity = errors.New("distribution: content integrity failure")
	ErrUntrusted = errors.New("distribution: release was not trusted")
	ErrCapacity  = errors.New("distribution: content limit exceeded")
)

type Artifact struct {
	Path       string      `json:"path"`
	Digest     qimu.Digest `json:"digest"`
	Size       int64       `json:"size"`
	Executable bool        `json:"executable,omitempty"`
}

// An import is still a declaration here. Installation and binding supply its
// exact provider references; a package cannot name a future InstallationID.
type Import struct {
	Slot     string           `json:"slot"`
	Contract qimu.ContractRef `json:"contract"`
	Optional bool             `json:"optional,omitempty"`
}

type RuntimeReference struct {
	Name    string      `json:"name"`
	Version string      `json:"version"`
	Digest  qimu.Digest `json:"digest"`
}

type Variant struct {
	OS      string            `json:"os"`
	Arch    string            `json:"arch"`
	Path    string            `json:"path"`
	Runtime *RuntimeReference `json:"runtime,omitempty"`
}

type ModuleImport struct {
	Specifier string               `json:"specifier"`
	Path      string               `json:"path,omitempty"`
	Shared    *HostModuleReference `json:"shared,omitempty"`
}

// HostModuleReference selects a versioned native realm profile. It is not a
// claim about an interpreter binary or portable wire compatibility. The host
// admits each specifier against its concrete implementation of this profile.
type HostModuleReference struct {
	Profile string `json:"profile"`
}

type Module struct {
	Entry   string         `json:"entry"`
	Imports []ModuleImport `json:"imports,omitempty"`
}

type Unit struct {
	ID                    qimu.UnitID              `json:"id"`
	Profile               qimu.Profile             `json:"profile"`
	Imports               []Import                 `json:"imports,omitempty"`
	Exports               []qimu.ExportDeclaration `json:"exports,omitempty"`
	Variants              []Variant                `json:"variants,omitempty"`
	Module                *Module                  `json:"module,omitempty"`
	Resources             []string                 `json:"resources,omitempty"`
	RequestedCapabilities []string                 `json:"requestedCapabilities,omitempty"`
}

type Contribution struct {
	Unit     qimu.UnitID      `json:"unit"`
	Export   string           `json:"export"`
	Point    qimu.ContractRef `json:"point"`
	Key      string           `json:"key"`
	Metadata json.RawMessage  `json:"metadata,omitzero"`
}

type Manifest struct {
	SchemaVersion int                `json:"schemaVersion"`
	Package       string             `json:"package"`
	Version       string             `json:"version"`
	Release       qimu.Digest        `json:"release,omitempty"`
	Artifacts     []Artifact         `json:"artifacts"`
	Contracts     []qimu.ContractRef `json:"contracts,omitempty"`
	Units         []Unit             `json:"units"`
	Contributions []Contribution     `json:"contributions,omitempty"`
}

type LockedRelease struct {
	Package string      `json:"package"`
	Release qimu.Digest `json:"release"`
}

type Lock struct {
	SchemaVersion int                `json:"schemaVersion"`
	Package       string             `json:"package"`
	Release       qimu.Digest        `json:"release"`
	Artifacts     []Artifact         `json:"artifacts"`
	Contracts     []qimu.ContractRef `json:"contracts,omitempty"`
	Dependencies  []LockedRelease    `json:"dependencies,omitempty"`
}

func ParseManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := jsonv2.Unmarshal(data, &manifest, jsonv2.RejectUnknownMembers(true)); err != nil {
		return manifest, errors.Join(ErrInvalid, err)
	}
	return manifest, manifest.Validate()
}

func ParseLock(data []byte) (Lock, error) {
	var lock Lock
	if err := jsonv2.Unmarshal(data, &lock, jsonv2.RejectUnknownMembers(true)); err != nil {
		return lock, errors.Join(ErrInvalid, err)
	}
	if lock.SchemaVersion != LockSchemaVersion || !qimu.ValidName(lock.Package) || lock.Release.Validate() != nil {
		return lock, ErrInvalid
	}
	seen := map[string]struct{}{}
	for _, dependency := range lock.Dependencies {
		if !qimu.ValidName(dependency.Package) || dependency.Release.Validate() != nil {
			return lock, ErrInvalid
		}
		if _, ok := seen[dependency.Package]; ok {
			return lock, ErrInvalid
		}
		seen[dependency.Package] = struct{}{}
	}
	return lock, nil
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != ManifestSchemaVersion || !qimu.ValidName(m.Package) || !qimu.ValidName(m.Version) {
		return ErrInvalid
	}
	if m.Release != "" && m.Release.Validate() != nil {
		return ErrInvalid
	}
	artifacts := make(map[string]Artifact, len(m.Artifacts))
	paths := map[string]string{}
	for _, artifact := range m.Artifacts {
		if !validPath(artifact.Path) || artifact.Path == ManifestName || artifact.Path == LockName || artifact.Size < 0 || artifact.Digest.Validate() != nil {
			return fmt.Errorf("%w: artifact", ErrInvalid)
		}
		if err := admitPath(paths, artifact.Path); err != nil {
			return err
		}
		artifacts[artifact.Path] = artifact
	}
	contracts := map[qimu.ContractRef]struct{}{}
	nominal := map[string]qimu.ContractRef{}
	for _, contract := range m.Contracts {
		if err := contract.Validate(); err != nil {
			return errors.Join(ErrInvalid, err)
		}
		if _, ok := contracts[contract]; ok {
			return fmt.Errorf("%w: duplicate contract", ErrInvalid)
		}
		key := string(contract.Kind) + "\x00" + contract.Name + "\x00" + contract.Version
		if prior, ok := nominal[key]; ok && prior != contract {
			return fmt.Errorf("%w: conflicting contract version", ErrInvalid)
		}
		nominal[key] = contract
		contracts[contract] = struct{}{}
	}
	units := map[qimu.UnitID]Unit{}
	for _, unit := range m.Units {
		if unit.ID.Validate() != nil || unit.Profile.Validate() != nil {
			return fmt.Errorf("%w: unit identity", ErrInvalid)
		}
		if _, exists := units[unit.ID]; exists {
			return fmt.Errorf("%w: duplicate unit", ErrInvalid)
		}
		units[unit.ID] = unit
		slots := map[string]struct{}{}
		for _, imported := range unit.Imports {
			if !qimu.ValidName(imported.Slot) {
				return fmt.Errorf("%w: import slot", ErrInvalid)
			}
			if _, exists := slots[imported.Slot]; exists {
				return fmt.Errorf("%w: duplicate import slot", ErrInvalid)
			}
			if _, exists := contracts[imported.Contract]; !exists {
				return fmt.Errorf("%w: undeclared import contract", ErrInvalid)
			}
			slots[imported.Slot] = struct{}{}
		}
		exports := map[string]struct{}{}
		for _, export := range unit.Exports {
			if !qimu.ValidName(export.Name) {
				return fmt.Errorf("%w: export name", ErrInvalid)
			}
			if _, exists := exports[export.Name]; exists {
				return fmt.Errorf("%w: duplicate export", ErrInvalid)
			}
			if _, exists := contracts[export.Contract]; !exists {
				return fmt.Errorf("%w: undeclared export contract", ErrInvalid)
			}
			exports[export.Name] = struct{}{}
		}
		targets := map[string]struct{}{}
		for _, variant := range unit.Variants {
			artifact, exists := artifacts[variant.Path]
			if !exists || !qimu.ValidName(variant.OS) || !qimu.ValidName(variant.Arch) {
				return fmt.Errorf("%w: unit artifact", ErrInvalid)
			}
			if variant.Runtime == nil && !artifact.Executable {
				return fmt.Errorf("%w: process entry is not executable", ErrInvalid)
			}
			if variant.Runtime != nil && !validRuntime(*variant.Runtime) {
				return fmt.Errorf("%w: runtime reference", ErrInvalid)
			}
			key := variant.OS + "/" + variant.Arch
			if _, exists := targets[key]; exists {
				return fmt.Errorf("%w: duplicate target variant", ErrInvalid)
			}
			targets[key] = struct{}{}
		}
		if unit.Module != nil {
			if _, exists := artifacts[unit.Module.Entry]; !exists {
				return fmt.Errorf("%w: module entry", ErrInvalid)
			}
			specifiers := map[string]struct{}{}
			for _, imported := range unit.Module.Imports {
				if !qimu.ValidName(imported.Specifier) || (imported.Path == "") == (imported.Shared == nil) {
					return fmt.Errorf("%w: module import", ErrInvalid)
				}
				if _, exists := specifiers[imported.Specifier]; exists {
					return fmt.Errorf("%w: duplicate module import", ErrInvalid)
				}
				specifiers[imported.Specifier] = struct{}{}
				if imported.Shared != nil {
					if !qimu.ValidName(imported.Shared.Profile) {
						return fmt.Errorf("%w: shared module", ErrInvalid)
					}
				} else if _, exists := artifacts[imported.Path]; !exists {
					return fmt.Errorf("%w: module path", ErrInvalid)
				}
			}
		}
		for _, resource := range unit.Resources {
			if _, exists := artifacts[resource]; !exists {
				return fmt.Errorf("%w: resource path", ErrInvalid)
			}
		}
		for _, capability := range unit.RequestedCapabilities {
			if !qimu.ValidName(capability) {
				return fmt.Errorf("%w: capability name", ErrInvalid)
			}
		}
		switch unit.Profile {
		case qimu.MCPProcess:
			if len(unit.Variants) == 0 || unit.Module != nil {
				return fmt.Errorf("%w: process unit entry", ErrInvalid)
			}
			for _, imported := range unit.Imports {
				if imported.Contract.Mode != qimu.PortableMode {
					return fmt.Errorf("%w: process import is native", ErrInvalid)
				}
			}
			for _, exported := range unit.Exports {
				if exported.Contract.Mode != qimu.PortableMode {
					return fmt.Errorf("%w: process export is native", ErrInvalid)
				}
			}
		case qimu.BrowserJS:
			if unit.Module == nil || len(unit.Variants) != 0 {
				return fmt.Errorf("%w: browser unit entry", ErrInvalid)
			}
		case qimu.StaticResource:
			if len(unit.Resources) == 0 || unit.Module != nil || len(unit.Variants) != 0 || len(unit.Imports) != 0 {
				return fmt.Errorf("%w: resource unit entry", ErrInvalid)
			}
		case qimu.NativeGo:
			if unit.Module != nil || len(unit.Variants) != 0 {
				return fmt.Errorf("%w: native unit is supplied by the host build", ErrInvalid)
			}
		default:
			return fmt.Errorf("%w: unsupported distribution profile %q", ErrInvalid, unit.Profile)
		}
	}
	keys := map[string]struct{}{}
	for _, contribution := range m.Contributions {
		unit, ok := units[contribution.Unit]
		if !ok || !qimu.ValidName(contribution.Key) || !qimu.ValidName(contribution.Export) {
			return fmt.Errorf("%w: contribution", ErrInvalid)
		}
		if _, ok := contracts[contribution.Point]; !ok || contribution.Point.Kind != qimu.PointKind {
			return fmt.Errorf("%w: contribution point", ErrInvalid)
		}
		if !slices.ContainsFunc(unit.Exports, func(export qimu.ExportDeclaration) bool {
			return export.Name == contribution.Export && export.Contract == contribution.Point
		}) {
			return fmt.Errorf("%w: contribution export", ErrInvalid)
		}
		key := string(contribution.Unit) + "/" + contribution.Point.Name + "/" + contribution.Key
		if _, ok := keys[key]; ok {
			return fmt.Errorf("%w: duplicate contribution", ErrInvalid)
		}
		keys[key] = struct{}{}
		if len(contribution.Metadata) != 0 {
			if _, err := jsonvalue.Parse(contribution.Metadata); err != nil {
				return errors.Join(ErrInvalid, err)
			}
		}
	}
	return nil
}

// Canonical preserves all declaration order. Artifact order is a set and is
// normalized; metadata is JSON syntax, not an inferred schema equivalence.
func (m Manifest) Canonical() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	m.Release = ""
	m.Artifacts = slices.Clone(m.Artifacts)
	slices.SortFunc(m.Artifacts, func(a, b Artifact) int { return strings.Compare(a.Path, b.Path) })
	encoded, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	canonical, err := jsonvalue.Parse(encoded)
	if err != nil {
		return nil, err
	}
	return canonical.JSON(), nil
}

func (m Manifest) Identity() (qimu.Digest, error) {
	data, err := m.Canonical()
	if err != nil {
		return "", err
	}
	return qimu.Hash(data), nil
}

func (m Manifest) Lock(dependencies []LockedRelease) (Lock, error) {
	digest, err := m.Identity()
	if err != nil {
		return Lock{}, err
	}
	lock := Lock{SchemaVersion: LockSchemaVersion, Package: m.Package, Release: digest, Artifacts: slices.Clone(m.Artifacts), Contracts: slices.Clone(m.Contracts), Dependencies: slices.Clone(dependencies)}
	slices.SortFunc(lock.Artifacts, func(a, b Artifact) int { return strings.Compare(a.Path, b.Path) })
	data, err := json.Marshal(lock)
	if err != nil {
		return Lock{}, err
	}
	return ParseLock(data)
}

func (l Lock) Verify(m Manifest) error {
	expected, err := m.Lock(l.Dependencies)
	if err != nil {
		return err
	}
	l.Artifacts = slices.Clone(l.Artifacts)
	slices.SortFunc(l.Artifacts, func(a, b Artifact) int { return strings.Compare(a.Path, b.Path) })
	a, _ := json.Marshal(expected)
	b, _ := json.Marshal(l)
	if !bytes.Equal(a, b) {
		return errors.Join(ErrIntegrity, errors.New("lock does not describe the release"))
	}
	return nil
}

func validRuntime(value RuntimeReference) bool {
	return qimu.ValidName(value.Name) && qimu.ValidName(value.Version) && value.Digest.Validate() == nil
}

// Portable archive paths deliberately use ASCII. This rejects Unicode and
// case-fold aliases rather than depending on a destination filesystem's rules.
const maxArtifactPathBytes = 1024
const maxArtifactPathSegmentBytes = 240

func validPath(value string) bool {
	if value == "" || len(value) > maxArtifactPathBytes || !fs.ValidPath(value) || value == "." || strings.Contains(value, "\\") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") || len(segment) > maxArtifactPathSegmentBytes {
			return false
		}
		stem := strings.ToLower(strings.SplitN(segment, ".", 2)[0])
		if stem == "con" || stem == "prn" || stem == "aux" || stem == "nul" || (len(stem) == 4 && (strings.HasPrefix(stem, "com") || strings.HasPrefix(stem, "lpt")) && stem[3] >= '1' && stem[3] <= '9') {
			return false
		}
	}
	for _, r := range value {
		if r < '!' || r > '~' || strings.ContainsRune(":*?\"<>|", r) {
			return false
		}
	}
	return true
}

func admitPath(paths map[string]string, value string) error {
	if !validPath(value) {
		return fmt.Errorf("%w: unsafe path", ErrInvalid)
	}
	key := strings.ToLower(value)
	if _, exists := paths[key]; exists {
		return fmt.Errorf("%w: duplicate or aliased path", ErrInvalid)
	}
	for ancestor := path.Dir(value); ancestor != "."; ancestor = path.Dir(ancestor) {
		ancestorKey := strings.ToLower(ancestor)
		if prior, exists := paths[ancestorKey]; exists && prior != ancestor+"/" {
			return fmt.Errorf("%w: file or alias is a path ancestor", ErrInvalid)
		}
		paths[ancestorKey] = ancestor + "/"
	}
	paths[key] = value
	return nil
}

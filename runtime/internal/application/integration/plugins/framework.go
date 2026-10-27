package plugins

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// ContractDocument is an immutable declaration projection. Qimu admits the
// descriptor once; Flame's selection and applicability policies remain here.
type ContractDocument struct {
	Reference  qimu.ContractRef
	Descriptor []byte
}

type FrameworkRelease struct {
	Plugin    *qimu.Plugin
	Contracts []ContractDocument
}

// FrameworkConfiguration contains only Flame-owned configuration. Installation
// identity, selected release and desired enablement belong to Qimu's record.
// Values may contain credentials and must not be forwarded to a browser realm.
type FrameworkConfiguration struct {
	Source          string                 `json:"source"`
	Values          map[string]string      `json:"values"`
	DisabledServers []mcpserver.ServerName `json:"disabledServers"`
	DisabledSkills  []string               `json:"disabledSkills"`
}

func EncodeFrameworkConfiguration(record plugin.Record) ([]byte, error) {
	value := FrameworkConfiguration{Source: record.Source, Values: record.Values, DisabledServers: slices.Clone(record.DisabledServers), DisabledSkills: slices.Clone(record.DisabledSkills)}
	if value.Values == nil {
		value.Values = map[string]string{}
	}
	if value.DisabledServers == nil {
		value.DisabledServers = []mcpserver.ServerName{}
	}
	if value.DisabledSkills == nil {
		value.DisabledSkills = []string{}
	}
	slices.SortFunc(value.DisabledServers, func(a, b mcpserver.ServerName) int { return strings.Compare(a.String(), b.String()) })
	slices.Sort(value.DisabledSkills)
	encoded, err := jsonvalue.Encode(value)
	if err != nil {
		return nil, err
	}
	return encoded.JSON(), nil
}

func DecodeFrameworkConfiguration(record qimu.InstallationRecord) (plugin.Record, error) {
	var value FrameworkConfiguration
	if err := json.Unmarshal(record.Configuration, &value, json.RejectUnknownMembers(true)); err != nil {
		return plugin.Record{}, err
	}
	id, err := identity.ParseInstallation(record.ID.String())
	if err != nil {
		return plugin.Record{}, err
	}
	digest, err := identity.ParseDigest(record.Release.String())
	if err != nil {
		return plugin.Record{}, err
	}
	return plugin.Record{ID: id, Source: value.Source, Selected: digest, Enabled: record.Enabled, Values: value.Values, DisabledServers: value.DisabledServers, DisabledSkills: value.DisabledSkills}, nil
}

func BackendUnit(server mcpserver.ServerName) qimu.UnitID {
	return qimu.UnitID("backend/" + server.String())
}

func BackendServer(unit qimu.UnitID) (mcpserver.ServerName, error) {
	value, found := strings.CutPrefix(string(unit), "backend/")
	if !found {
		return mcpserver.ServerName{}, plugin.ErrInvalid
	}
	return mcpserver.ParseServerName(value)
}

func RequirementSlot(contribution, requirement plugin.ContributionID) string {
	return contribution.String() + "/" + requirement.String()
}

const PointValuesUnit qimu.UnitID = "point-values"

func ComponentUnit(id plugin.ContributionID) qimu.UnitID {
	return qimu.UnitID("component/" + id.String())
}

func ComponentContract() (qimu.Contract[any], error) {
	return qimu.NewContract[any]("io.github.tangerg.flame/component", "1")
}

type ComponentConfiguration struct {
	Export        string            `json:"export"`
	Component     ComponentMetadata `json:"component"`
	ModuleProfile string            `json:"moduleProfile,omitempty"`
	SharedModules []string          `json:"sharedModules"`
}

type ComponentMetadata struct {
	ID           string             `json:"id"`
	Title        string             `json:"title"`
	Scope        string             `json:"scope"`
	Placement    string             `json:"placement"`
	Contribution string             `json:"contribution,omitempty"`
	Material     string             `json:"material,omitempty"`
	Selector     *ComponentSelector `json:"selector,omitempty"`
}

type ComponentSelector struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// PublicComponentConfiguration is the complete browser setup projection. It
// reads immutable release declarations, never private installation Values.
func PublicComponentConfiguration(release plugin.Release, id plugin.ContributionID) ([]byte, error) {
	for _, component := range release.Declaration().Components {
		if component.ID != id {
			continue
		}
		metadata := ComponentMetadata{ID: id.String(), Title: component.Title, Scope: string(component.Scope), Placement: string(component.Placement), Contribution: component.Contribution.String(), Material: component.Material}
		if component.Selector != nil {
			metadata.Selector = &ComponentSelector{Kind: string(component.Selector.Kind), Name: component.Selector.Name}
		}
		value, err := jsonvalue.Encode(ComponentConfiguration{Export: component.Export, Component: metadata, SharedModules: []string{}})
		if err != nil {
			return nil, err
		}
		return value.JSON(), nil
	}
	return nil, plugin.ErrNotFound
}

func capabilityContract(contract *portable.Capability) (qimu.Contract[any], error) {
	if contract == nil {
		return qimu.Contract[any]{}, plugin.ErrInvalid
	}
	ref := qimu.ContractRef{Name: contract.Descriptor().Reference, Version: "1", Digest: qimu.Hash(contract.Identity().JSON()), Kind: qimu.ServiceKind, Mode: qimu.PortableMode}
	return qimu.BindContract[any](ref, contract.Identity().JSON())
}

// CapabilityPoint is Flame's collection of implementations of a portable
// callable contract. Selection and decoration order are consumer policies;
// membership and callable value have separate original descriptors.
func CapabilityPoint(contract *portable.Capability) (qimu.Point[jsonvalue.Value, any], error) {
	value, err := capabilityContract(contract)
	if err != nil {
		return qimu.Point[jsonvalue.Value, any]{}, err
	}
	return qimu.NewPortablePoint[jsonvalue.Value](contract.Descriptor().Reference, "1", []byte(`{"type":"null"}`), value)
}

func DataPoint(contract *portable.Point) (qimu.Point[jsonvalue.Value, jsonvalue.Value], error) {
	if contract == nil {
		return qimu.Point[jsonvalue.Value, jsonvalue.Value]{}, plugin.ErrInvalid
	}
	ref := qimu.ContractRef{Name: contract.Definition().Reference, Version: "1", Digest: qimu.Hash(contract.Identity().JSON()), Kind: qimu.ServiceKind, Mode: qimu.PortableMode}
	value, err := qimu.BindContract[jsonvalue.Value](ref, contract.Identity().JSON())
	if err != nil {
		return qimu.Point[jsonvalue.Value, jsonvalue.Value]{}, err
	}
	return qimu.NewPortablePoint[jsonvalue.Value](ref.Name, "1", contract.Definition().Value, value)
}

// FrameworkDefinition is the one Flame-to-Qimu declaration projection shared
// by author tooling and Runtime. It carries no release or installation identity.
type FrameworkDefinition struct {
	Units     []qimu.Unit
	Contracts []ContractDocument
}

func ProjectFrameworkRelease(release plugin.Release) (FrameworkRelease, error) {
	projection, err := ProjectFrameworkDeclaration(release.Declaration())
	if err != nil {
		return FrameworkRelease{}, err
	}
	plugin, err := qimu.Define(qimu.Definition{Revision: qimu.Digest(release.Digest().String()), Units: projection.Units})
	if err != nil {
		return FrameworkRelease{}, err
	}
	return FrameworkRelease{Plugin: plugin, Contracts: projection.Contracts}, nil
}

func ProjectFrameworkDeclaration(declaration plugin.Declaration) (FrameworkDefinition, error) {
	contracts := map[qimu.ContractRef][]byte{}
	addCapability := func(encoded jsonvalue.Value) (qimu.ContractRef, error) {
		contract, err := portable.ParseCapability(encoded.JSON())
		if err != nil {
			return qimu.ContractRef{}, err
		}
		value, err := capabilityContract(contract)
		if err != nil {
			return qimu.ContractRef{}, err
		}
		point, err := CapabilityPoint(contract)
		if err != nil {
			return qimu.ContractRef{}, err
		}
		contracts[value.Ref()] = value.Descriptor()
		contracts[point.Ref()] = point.Descriptor()
		return point.Ref(), nil
	}
	units := make([]qimu.Unit, 0, len(declaration.Servers)+1)
	for _, server := range declaration.Servers {
		unit := qimu.UnitDeclaration{ID: BackendUnit(server.Name), Profile: qimu.MCPProcess, Imports: []qimu.ImportDeclaration{}, Exports: []qimu.ExportDeclaration{}}
		for _, contribution := range declaration.Contributions {
			if contribution.Server != server.Name {
				continue
			}
			ref, err := addCapability(contribution.Contract)
			if err != nil {
				return FrameworkDefinition{}, err
			}
			unit.Exports = append(unit.Exports, qimu.ExportDeclaration{Name: contribution.ID.String(), Contract: ref})
			for _, requirement := range contribution.Requires {
				ref, err := addCapability(requirement.Contract)
				if err != nil {
					return FrameworkDefinition{}, err
				}
				unit.Imports = append(unit.Imports, qimu.ImportDeclaration{Slot: RequirementSlot(contribution.ID, requirement.ID), Contract: ref, Optional: requirement.Optional})
			}
		}
		units = append(units, qimu.Unit{Declaration: unit})
	}
	for _, point := range declaration.Points {
		contract, err := portable.ParsePoint(point.Contract.JSON())
		if err != nil {
			return FrameworkDefinition{}, err
		}
		point, err := DataPoint(contract)
		if err != nil {
			return FrameworkDefinition{}, err
		}
		contracts[point.Ref()] = point.Descriptor()
		contracts[point.ValueContract()] = contract.Identity().JSON()
	}
	if len(declaration.PointValues) > 0 {
		unit := qimu.UnitDeclaration{ID: PointValuesUnit, Profile: qimu.NativeGo, Imports: []qimu.ImportDeclaration{}, Exports: []qimu.ExportDeclaration{}}
		values := make([]qimu.ExportValue, 0, len(declaration.PointValues))
		for _, contribution := range declaration.PointValues {
			contract, err := portable.ParsePoint(contribution.Contract.JSON())
			if err != nil {
				return FrameworkDefinition{}, err
			}
			point, err := DataPoint(contract)
			if err != nil {
				return FrameworkDefinition{}, err
			}
			contracts[point.Ref()] = point.Descriptor()
			contracts[point.ValueContract()] = contract.Identity().JSON()
			unit.Exports = append(unit.Exports, qimu.ExportDeclaration{Name: contribution.ID.String(), Contract: point.Ref()})
			values = append(values, qimu.Contribute(contribution.ID.String(), point, contribution.Value, contribution.Value))
		}
		units = append(units, qimu.Unit{Declaration: unit, Setup: func(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) { return slices.Clone(values), nil }})
	}
	if len(declaration.Components) > 0 {
		contract, err := ComponentContract()
		if err != nil {
			return FrameworkDefinition{}, err
		}
		contracts[contract.Ref()] = contract.Descriptor()
		for _, component := range declaration.Components {
			units = append(units, qimu.Unit{Declaration: qimu.UnitDeclaration{ID: ComponentUnit(component.ID), Profile: qimu.BrowserJS, Imports: []qimu.ImportDeclaration{}, Exports: []qimu.ExportDeclaration{{Name: "component", Contract: contract.Ref()}}}})
		}
	}
	result := FrameworkDefinition{Units: units}
	for ref, descriptor := range contracts {
		result.Contracts = append(result.Contracts, ContractDocument{Reference: ref, Descriptor: descriptor})
	}
	slices.SortFunc(result.Contracts, func(a, b ContractDocument) int {
		return strings.Compare(string(a.Reference.Kind)+a.Reference.String(), string(b.Reference.Kind)+b.Reference.String())
	})
	return result, nil
}

func ContributionSource(binding ContributionBinding) (qimu.ExportRef, error) {
	contract, err := portable.ParseCapability(binding.Contract.JSON())
	if err != nil {
		return qimu.ExportRef{}, err
	}
	point, err := CapabilityPoint(contract)
	if err != nil {
		return qimu.ExportRef{}, err
	}
	return qimu.ExportRef{InstallationID: qimu.InstallationID(binding.InstallationID.String()), Release: qimu.Digest(binding.Digest.String()), UnitID: BackendUnit(binding.ServerName), Export: binding.ContributionID.String(), Contract: point.Ref()}, nil
}

// FrameworkError gives Core failures one Flame category without losing their
// original cause at installation, binding, or execution boundaries.
func FrameworkError(err error) error {
	switch {
	case errors.Is(err, qimu.ErrDependencyCycle), errors.Is(err, qimu.ErrInvalid), errors.Is(err, qimu.ErrContractMismatch):
		return errors.Join(plugin.ErrInvalid, err)
	case errors.Is(err, qimu.ErrStaleBinding):
		return errors.Join(plugin.ErrStale, err)
	case errors.Is(err, qimu.ErrInUse):
		return errors.Join(plugin.ErrInUse, err)
	case errors.Is(err, qimu.ErrUnavailable), errors.Is(err, qimu.ErrAmbiguous), errors.Is(err, qimu.ErrFenced):
		return errors.Join(plugin.ErrUnavailable, err)
	default:
		return err
	}
}

func RegisterFrameworkRelease(host *qimu.Host, release FrameworkRelease) error {
	if host == nil || release.Plugin == nil {
		return fmt.Errorf("plugins: framework host and release are required")
	}
	for _, contract := range release.Contracts {
		if err := host.RegisterDescriptor(contract.Reference, contract.Descriptor); err != nil {
			return err
		}
	}
	return nil
}

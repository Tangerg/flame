package plugins

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/frame"
	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
)

type FrameworkRelease struct {
	Plugin    *framehost.Plugin
	Contracts []framehost.Declaration
}

const (
	backendUnitPrefix   = "backend/"
	componentUnitPrefix = "component/"
)

// FrameworkConfiguration contains only Flame-owned configuration. Installation
// identity, selected release and desired enablement belong to Frame's record.
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
	encoded, err := framevalue.EncodeJSON(value)
	if err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

func DecodeFrameworkConfiguration(record framehost.InstallationRecord) (plugin.Record, error) {
	var value FrameworkConfiguration
	if err := json.Unmarshal(record.Configuration, &value, json.RejectUnknownMembers(true)); err != nil {
		return plugin.Record{}, err
	}
	id, err := identity.ParseHostInstallation(record.ID)
	if err != nil {
		return plugin.Record{}, err
	}
	digest := record.Release
	if err := digest.Validate(); err != nil {
		return plugin.Record{}, err
	}
	return plugin.Record{ID: id, Source: value.Source, Selected: digest, Enabled: record.Enabled, Values: value.Values, DisabledServers: value.DisabledServers, DisabledSkills: value.DisabledSkills}, nil
}

func BackendUnit(server mcpserver.ServerName) framehost.UnitID {
	return framehost.UnitID(backendUnitPrefix + server.String())
}

func BackendServer(unit framehost.UnitID) (mcpserver.ServerName, error) {
	value, found := strings.CutPrefix(string(unit), backendUnitPrefix)
	if !found {
		return mcpserver.ServerName{}, plugin.ErrInvalid
	}
	return mcpserver.ParseServerName(value)
}

func RequirementSlot(contribution, requirement plugin.ContributionID) string {
	return contribution.String() + "/" + requirement.String()
}

const PointValuesUnit framehost.UnitID = "point-values"

func ComponentUnit(id plugin.ContributionID) framehost.UnitID {
	return framehost.UnitID(componentUnitPrefix + id.String())
}

func ComponentContract() (framehost.Contract[any], error) {
	return framehost.NewContract[any]("io.github.tangerg.flame/component", "1")
}

type ComponentConfiguration struct {
	Export        string            `json:"export"`
	Component     ComponentMetadata `json:"component"`
	ModuleProfile string            `json:"moduleProfile,omitempty"`
	SharedModules []string          `json:"sharedModules"`
}

type ComponentMetadata struct {
	ToolCategory builtin.ToolCategory `json:"toolCategory,omitzero"`
	TargetView   string               `json:"targetView,omitzero"`

	ID              string                       `json:"id"`
	Title           string                       `json:"title"`
	LocalizedTitles map[string]string            `json:"localizedTitles,omitzero"`
	Scope           extension.ComponentScope     `json:"scope"`
	Placement       extension.ComponentPlacement `json:"placement"`
	Contribution    string                       `json:"contribution,omitempty"`
	Material        string                       `json:"material,omitempty"`
	SettingsIntent  string                       `json:"settingsIntent,omitempty"`
	Selector        *ComponentSelector           `json:"selector,omitempty"`
}

type ComponentSelector struct {
	Kind extension.RendererSelectorKind `json:"kind"`
	Name string                         `json:"name"`
}

// PublicComponentConfiguration is the complete browser setup projection. It
// reads immutable release declarations, never private installation Values.
func PublicComponentConfiguration(release plugin.Release, id plugin.ContributionID) ([]byte, error) {
	for _, component := range release.Declaration().Components {
		if component.ID != id {
			continue
		}
		metadata := ComponentMetadata{ID: id.String(), Title: component.Title, LocalizedTitles: component.LocalizedTitles, ToolCategory: component.ToolCategory, TargetView: component.TargetView, Scope: component.Scope, Placement: component.Placement, Contribution: component.Contribution.String(), Material: component.Material, SettingsIntent: component.SettingsIntent}
		if component.Selector != nil {
			metadata.Selector = &ComponentSelector{Kind: component.Selector.Kind, Name: component.Selector.Name}
		}
		value, err := framevalue.EncodeJSON(ComponentConfiguration{Export: component.Export, Component: metadata, SharedModules: []string{}})
		if err != nil {
			return nil, err
		}
		return value.Bytes(), nil
	}
	return nil, plugin.ErrNotFound
}

func capabilityContract(contract *plugincontract.Capability) (framehost.Contract[any], error) {
	if contract == nil {
		return framehost.Contract[any]{}, plugin.ErrInvalid
	}
	return framehost.NewPortableContract[any]("1", contract.Service())
}

// CapabilityPoint is Flame's collection of implementations of a portable
// callable contract. Selection and decoration order are consumer policies;
// membership and callable value have separate original descriptors.
func CapabilityPoint(contract *plugincontract.Capability) (framehost.Point[framevalue.JSON, any], error) {
	value, err := capabilityContract(contract)
	if err != nil {
		return framehost.Point[framevalue.JSON, any]{}, err
	}
	return framehost.NewPortablePoint[framevalue.JSON](contract.Descriptor().Reference, "1", []byte(`{"type":"null"}`), value)
}

func DataPoint(contract *plugincontract.Point) (framehost.Point[framevalue.JSON, framevalue.JSON], error) {
	if contract == nil {
		return framehost.Point[framevalue.JSON, framevalue.JSON]{}, plugin.ErrInvalid
	}
	ref := framehost.ContractRef{Name: contract.Definition().Reference, Version: "1", Digest: framevalue.Hash(contract.Identity().Bytes()), Kind: framehost.ServiceKind, Mode: framehost.PortableMode}
	value, err := framehost.BindContract[framevalue.JSON](ref, contract.Identity().Bytes())
	if err != nil {
		return framehost.Point[framevalue.JSON, framevalue.JSON]{}, err
	}
	return framehost.NewPortablePoint[framevalue.JSON](ref.Name, "1", contract.Definition().Value, value)
}

// FrameworkDefinition is the one Flame-to-Frame declaration projection shared
// by author tooling and Runtime. Typed declarations retain the local Go binding;
// a descriptor alone cannot admit an in-process implementation.
type FrameworkDefinition struct {
	Units     []framehost.Unit
	Contracts []framehost.Declaration
}

func ProjectFrameworkRelease(release plugin.Release, backendProfile framehost.Profile) (FrameworkRelease, error) {
	projection, err := ProjectFrameworkDeclaration(release.Declaration(), backendProfile)
	if err != nil {
		return FrameworkRelease{}, err
	}
	plugin, err := framehost.Define(framehost.Definition{Revision: release.Digest(), Units: projection.Units})
	if err != nil {
		return FrameworkRelease{}, err
	}
	return FrameworkRelease{Plugin: plugin, Contracts: projection.Contracts}, nil
}

func ProjectFrameworkDeclaration(declaration plugin.Declaration, backendProfile framehost.Profile) (FrameworkDefinition, error) {
	projection := frameworkProjection{contracts: map[framehost.ContractRef]framehost.Declaration{}}
	units := make([]framehost.Unit, 0, len(declaration.Servers)+1)
	for _, server := range declaration.Servers {
		unit, err := projection.backendUnit(declaration, server.Name, backendProfile)
		if err != nil {
			return FrameworkDefinition{}, err
		}
		units = append(units, unit)
	}
	for _, definition := range declaration.Points {
		if _, err := projection.dataPoint(definition.Contract); err != nil {
			return FrameworkDefinition{}, err
		}
	}
	if len(declaration.PointValues) > 0 {
		unit, err := projection.pointValuesUnit(declaration.PointValues)
		if err != nil {
			return FrameworkDefinition{}, err
		}
		units = append(units, unit)
	}
	if len(declaration.Components) > 0 {
		components, err := projection.componentUnits(declaration.Components)
		if err != nil {
			return FrameworkDefinition{}, err
		}
		units = append(units, components...)
	}
	return FrameworkDefinition{Units: units, Contracts: projection.sortedContracts()}, nil
}

// frameworkProjection collects each contract once, however many units name it.
type frameworkProjection struct {
	contracts map[framehost.ContractRef]framehost.Declaration
}

func (p frameworkProjection) capability(encoded framevalue.JSON) (framehost.ContractRef, error) {
	contract, err := plugincontract.ParseCapability(encoded.Bytes())
	if err != nil {
		return framehost.ContractRef{}, err
	}
	value, err := capabilityContract(contract)
	if err != nil {
		return framehost.ContractRef{}, err
	}
	point, err := CapabilityPoint(contract)
	if err != nil {
		return framehost.ContractRef{}, err
	}
	p.contracts[value.Ref()] = value
	p.contracts[point.Ref()] = point
	return point.Ref(), nil
}

func (p frameworkProjection) dataPoint(encoded framevalue.JSON) (framehost.Point[framevalue.JSON, framevalue.JSON], error) {
	point, value, err := dataPointDeclarations(encoded)
	if err != nil {
		return point, err
	}
	p.contracts[value.Ref()] = value
	p.contracts[point.Ref()] = point
	return point, nil
}

func (p frameworkProjection) backendUnit(declaration plugin.Declaration, server mcpserver.ServerName, profile framehost.Profile) (framehost.Unit, error) {
	unit := framehost.UnitDeclaration{ID: BackendUnit(server), Profile: profile, Imports: []framehost.ImportDeclaration{}, Exports: []framehost.ExportDeclaration{}}
	for _, contribution := range declaration.Contributions {
		if contribution.Server != server {
			continue
		}
		ref, err := p.capability(contribution.Contract)
		if err != nil {
			return framehost.Unit{}, err
		}
		unit.Exports = append(unit.Exports, framehost.ExportDeclaration{Name: contribution.ID.String(), Contract: ref})
		for _, requirement := range contribution.Requires {
			ref, err := p.capability(requirement.Contract)
			if err != nil {
				return framehost.Unit{}, err
			}
			unit.Imports = append(unit.Imports, framehost.ImportDeclaration{Slot: RequirementSlot(contribution.ID, requirement.ID), Contract: ref, Optional: requirement.Optional})
		}
	}
	return framehost.Unit{Declaration: unit}, nil
}

func (p frameworkProjection) pointValuesUnit(contributions []plugin.PointValue) (framehost.Unit, error) {
	unit := framehost.UnitDeclaration{ID: PointValuesUnit, Profile: framehost.NativeGo, Imports: []framehost.ImportDeclaration{}, Exports: []framehost.ExportDeclaration{}}
	values := make([]framehost.ExportValue, 0, len(contributions))
	for _, contribution := range contributions {
		point, err := p.dataPoint(contribution.Contract)
		if err != nil {
			return framehost.Unit{}, err
		}
		unit.Exports = append(unit.Exports, framehost.ExportDeclaration{Name: contribution.ID.String(), Contract: point.Ref()})
		values = append(values, framehost.Contribute(contribution.ID.String(), point, contribution.Value, contribution.Value))
	}
	return framehost.Unit{Declaration: unit, Setup: func(context.Context, framehost.Setup) ([]framehost.ExportValue, error) {
		return slices.Clone(values), nil
	}}, nil
}

func (p frameworkProjection) componentUnits(components []plugin.ClientComponent) ([]framehost.Unit, error) {
	contract, err := ComponentContract()
	if err != nil {
		return nil, err
	}
	p.contracts[contract.Ref()] = contract
	units := make([]framehost.Unit, 0, len(components))
	for _, component := range components {
		units = append(units, framehost.Unit{Declaration: framehost.UnitDeclaration{ID: ComponentUnit(component.ID), Profile: framehost.BrowserJS, Imports: []framehost.ImportDeclaration{}, Exports: []framehost.ExportDeclaration{{Name: extension.ComponentExport, Contract: contract.Ref()}}}})
	}
	return units, nil
}

func (p frameworkProjection) sortedContracts() []framehost.Declaration {
	contracts := slices.Collect(maps.Values(p.contracts))
	slices.SortFunc(contracts, func(a, b framehost.Declaration) int {
		return strings.Compare(string(a.Ref().Kind)+a.Ref().String(), string(b.Ref().Kind)+b.Ref().String())
	})
	return contracts
}

func dataPointDeclarations(encoded framevalue.JSON) (framehost.Point[framevalue.JSON, framevalue.JSON], framehost.Contract[framevalue.JSON], error) {
	var point framehost.Point[framevalue.JSON, framevalue.JSON]
	var value framehost.Contract[framevalue.JSON]
	contract, err := plugincontract.ParsePoint(encoded.Bytes())
	if err != nil {
		return point, value, err
	}
	point, err = DataPoint(contract)
	if err != nil {
		return point, value, err
	}
	value, err = framehost.BindContract[framevalue.JSON](point.ValueContract(), contract.Identity().Bytes())
	return point, value, err
}

func ContributionSource(binding ContributionBinding) (framehost.ExportRef, error) {
	contract, err := plugincontract.ParseCapability(binding.Contract.Bytes())
	if err != nil {
		return framehost.ExportRef{}, err
	}
	point, err := CapabilityPoint(contract)
	if err != nil {
		return framehost.ExportRef{}, err
	}
	return framehost.ExportRef{InstallationID: binding.InstallationID.Host(), Release: binding.Digest, UnitID: BackendUnit(binding.ServerName), Export: binding.ContributionID.String(), Contract: point.Ref()}, nil
}

// FrameworkError gives Core failures one Flame category without losing their
// original cause at installation, binding, or execution boundaries.
func FrameworkError(err error) error {
	switch {
	case errors.Is(err, framehost.ErrDependencyCycle), errors.Is(err, frame.ErrInvalid), errors.Is(err, frame.ErrContractMismatch):
		return errors.Join(plugin.ErrInvalid, err)
	case errors.Is(err, framehost.ErrStaleBinding):
		return errors.Join(plugin.ErrStale, err)
	case errors.Is(err, framehost.ErrInUse):
		return errors.Join(plugin.ErrInUse, err)
	case errors.Is(err, framehost.ErrUnavailable), errors.Is(err, framehost.ErrAmbiguous), errors.Is(err, framehost.ErrFenced):
		return errors.Join(plugin.ErrUnavailable, err)
	default:
		return err
	}
}

func RegisterFrameworkRelease(host framehost.Host, release FrameworkRelease) error {
	if !host.Valid() || release.Plugin == nil {
		return fmt.Errorf("plugins: framework host and release are required")
	}
	return host.Register(release.Contracts...)
}

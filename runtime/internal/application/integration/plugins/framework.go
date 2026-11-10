package plugins

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"
)

type FrameworkRelease struct {
	Plugin    *frame.Plugin
	Contracts []frame.Declaration
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

func DecodeFrameworkConfiguration(record frame.InstallationRecord) (plugin.Record, error) {
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

func BackendUnit(server mcpserver.ServerName) frame.UnitID {
	return frame.UnitID(backendUnitPrefix + server.String())
}

func BackendServer(unit frame.UnitID) (mcpserver.ServerName, error) {
	value, found := strings.CutPrefix(string(unit), backendUnitPrefix)
	if !found {
		return mcpserver.ServerName{}, plugin.ErrInvalid
	}
	return mcpserver.ParseServerName(value)
}

func RequirementSlot(contribution, requirement plugin.ContributionID) string {
	return contribution.String() + "/" + requirement.String()
}

const PointValuesUnit frame.UnitID = "point-values"

func ComponentUnit(id plugin.ContributionID) frame.UnitID {
	return frame.UnitID(componentUnitPrefix + id.String())
}

func ComponentContract() (frame.Contract[any], error) {
	return frame.NewContract[any]("io.github.tangerg.flame/component", "1")
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

func capabilityContract(contract *plugincontract.Capability) (frame.Contract[any], error) {
	if contract == nil {
		return frame.Contract[any]{}, plugin.ErrInvalid
	}
	return frame.NewPortableContract[any]("1", contract.Service())
}

// CapabilityPoint is Flame's collection of implementations of a portable
// callable contract. Selection and decoration order are consumer policies;
// membership and callable value have separate original descriptors.
func CapabilityPoint(contract *plugincontract.Capability) (frame.Point[framevalue.JSON, any], error) {
	value, err := capabilityContract(contract)
	if err != nil {
		return frame.Point[framevalue.JSON, any]{}, err
	}
	return frame.NewPortablePoint[framevalue.JSON](contract.Descriptor().Reference, "1", []byte(`{"type":"null"}`), value)
}

func DataPoint(contract *plugincontract.Point) (frame.Point[framevalue.JSON, framevalue.JSON], error) {
	if contract == nil {
		return frame.Point[framevalue.JSON, framevalue.JSON]{}, plugin.ErrInvalid
	}
	ref := frame.ContractRef{Name: contract.Definition().Reference, Version: "1", Digest: framevalue.Hash(contract.Identity().Bytes()), Kind: frame.ServiceKind, Mode: frame.PortableMode}
	value, err := frame.BindContract[framevalue.JSON](ref, contract.Identity().Bytes())
	if err != nil {
		return frame.Point[framevalue.JSON, framevalue.JSON]{}, err
	}
	return frame.NewPortablePoint[framevalue.JSON](ref.Name, "1", contract.Definition().Value, value)
}

// FrameworkDefinition is the one Flame-to-Frame declaration projection shared
// by author tooling and Runtime. Typed declarations retain the local Go binding;
// a descriptor alone cannot admit an in-process implementation.
type FrameworkDefinition struct {
	Units     []frame.Unit
	Contracts []frame.Declaration
}

func ProjectFrameworkRelease(release plugin.Release, backendProfile frame.Profile) (FrameworkRelease, error) {
	projection, err := ProjectFrameworkDeclaration(release.Declaration(), backendProfile)
	if err != nil {
		return FrameworkRelease{}, err
	}
	plugin, err := frame.Define(frame.Definition{Revision: release.Digest(), Units: projection.Units})
	if err != nil {
		return FrameworkRelease{}, err
	}
	return FrameworkRelease{Plugin: plugin, Contracts: projection.Contracts}, nil
}

func ProjectFrameworkDeclaration(declaration plugin.Declaration, backendProfile frame.Profile) (FrameworkDefinition, error) {
	contracts := map[frame.ContractRef]frame.Declaration{}
	addCapability := func(encoded framevalue.JSON) (frame.ContractRef, error) {
		contract, err := plugincontract.ParseCapability(encoded.Bytes())
		if err != nil {
			return frame.ContractRef{}, err
		}
		value, err := capabilityContract(contract)
		if err != nil {
			return frame.ContractRef{}, err
		}
		point, err := CapabilityPoint(contract)
		if err != nil {
			return frame.ContractRef{}, err
		}
		contracts[value.Ref()] = value
		contracts[point.Ref()] = point
		return point.Ref(), nil
	}
	addDataPoint := func(encoded framevalue.JSON) (frame.Point[framevalue.JSON, framevalue.JSON], error) {
		point, value, err := dataPointDeclarations(encoded)
		if err != nil {
			return point, err
		}
		contracts[value.Ref()] = value
		contracts[point.Ref()] = point
		return point, nil
	}

	units := make([]frame.Unit, 0, len(declaration.Servers)+1)
	for _, server := range declaration.Servers {
		unit := frame.UnitDeclaration{ID: BackendUnit(server.Name), Profile: backendProfile, Imports: []frame.ImportDeclaration{}, Exports: []frame.ExportDeclaration{}}
		for _, contribution := range declaration.Contributions {
			if contribution.Server != server.Name {
				continue
			}
			ref, err := addCapability(contribution.Contract)
			if err != nil {
				return FrameworkDefinition{}, err
			}
			unit.Exports = append(unit.Exports, frame.ExportDeclaration{Name: contribution.ID.String(), Contract: ref})
			for _, requirement := range contribution.Requires {
				ref, err := addCapability(requirement.Contract)
				if err != nil {
					return FrameworkDefinition{}, err
				}
				unit.Imports = append(unit.Imports, frame.ImportDeclaration{Slot: RequirementSlot(contribution.ID, requirement.ID), Contract: ref, Optional: requirement.Optional})
			}
		}
		units = append(units, frame.Unit{Declaration: unit})
	}
	for _, definition := range declaration.Points {
		if _, err := addDataPoint(definition.Contract); err != nil {
			return FrameworkDefinition{}, err
		}
	}
	if len(declaration.PointValues) > 0 {
		unit := frame.UnitDeclaration{ID: PointValuesUnit, Profile: frame.NativeGo, Imports: []frame.ImportDeclaration{}, Exports: []frame.ExportDeclaration{}}
		values := make([]frame.ExportValue, 0, len(declaration.PointValues))
		for _, contribution := range declaration.PointValues {
			point, err := addDataPoint(contribution.Contract)
			if err != nil {
				return FrameworkDefinition{}, err
			}
			unit.Exports = append(unit.Exports, frame.ExportDeclaration{Name: contribution.ID.String(), Contract: point.Ref()})
			values = append(values, frame.Contribute(contribution.ID.String(), point, contribution.Value, contribution.Value))
		}
		units = append(units, frame.Unit{Declaration: unit, Setup: func(context.Context, *frame.Setup) ([]frame.ExportValue, error) { return slices.Clone(values), nil }})
	}
	if len(declaration.Components) > 0 {
		contract, err := ComponentContract()
		if err != nil {
			return FrameworkDefinition{}, err
		}
		contracts[contract.Ref()] = contract
		for _, component := range declaration.Components {
			units = append(units, frame.Unit{Declaration: frame.UnitDeclaration{ID: ComponentUnit(component.ID), Profile: frame.BrowserJS, Imports: []frame.ImportDeclaration{}, Exports: []frame.ExportDeclaration{{Name: extension.ComponentExport, Contract: contract.Ref()}}}})
		}
	}
	result := FrameworkDefinition{Units: units}
	for _, contract := range contracts {
		result.Contracts = append(result.Contracts, contract)
	}
	slices.SortFunc(result.Contracts, func(a, b frame.Declaration) int {
		return strings.Compare(string(a.Ref().Kind)+a.Ref().String(), string(b.Ref().Kind)+b.Ref().String())
	})
	return result, nil
}

func dataPointDeclarations(encoded framevalue.JSON) (frame.Point[framevalue.JSON, framevalue.JSON], frame.Contract[framevalue.JSON], error) {
	var point frame.Point[framevalue.JSON, framevalue.JSON]
	var value frame.Contract[framevalue.JSON]
	contract, err := plugincontract.ParsePoint(encoded.Bytes())
	if err != nil {
		return point, value, err
	}
	point, err = DataPoint(contract)
	if err != nil {
		return point, value, err
	}
	value, err = frame.BindContract[framevalue.JSON](point.ValueContract(), contract.Identity().Bytes())
	return point, value, err
}

func ContributionSource(binding ContributionBinding) (frame.ExportRef, error) {
	contract, err := plugincontract.ParseCapability(binding.Contract.Bytes())
	if err != nil {
		return frame.ExportRef{}, err
	}
	point, err := CapabilityPoint(contract)
	if err != nil {
		return frame.ExportRef{}, err
	}
	return frame.ExportRef{InstallationID: binding.InstallationID.Host(), Release: binding.Digest, UnitID: BackendUnit(binding.ServerName), Export: binding.ContributionID.String(), Contract: point.Ref()}, nil
}

// FrameworkError gives Core failures one Flame category without losing their
// original cause at installation, binding, or execution boundaries.
func FrameworkError(err error) error {
	switch {
	case errors.Is(err, frame.ErrDependencyCycle), errors.Is(err, frame.ErrInvalid), errors.Is(err, frame.ErrContractMismatch):
		return errors.Join(plugin.ErrInvalid, err)
	case errors.Is(err, frame.ErrStaleBinding):
		return errors.Join(plugin.ErrStale, err)
	case errors.Is(err, frame.ErrInUse):
		return errors.Join(plugin.ErrInUse, err)
	case errors.Is(err, frame.ErrUnavailable), errors.Is(err, frame.ErrAmbiguous), errors.Is(err, frame.ErrFenced):
		return errors.Join(plugin.ErrUnavailable, err)
	default:
		return err
	}
}

func RegisterFrameworkRelease(host *frame.Host, release FrameworkRelease) error {
	if host == nil || release.Plugin == nil {
		return fmt.Errorf("plugins: framework host and release are required")
	}
	return host.Register(release.Contracts...)
}

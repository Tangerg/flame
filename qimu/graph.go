package qimu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

func (h *Host) unitLocked(ref UnitRef) (Unit, *installed, error) {
	installation := h.installations[ref.InstallationID]
	if installation == nil || !installation.record.Enabled || installation.record.Release != ref.Release {
		return Unit{}, nil, ErrUnavailable
	}
	for _, unit := range installation.plugin.units {
		if unit.Declaration.ID == ref.UnitID {
			return unit, installation, nil
		}
	}
	return Unit{}, nil, ErrUnavailable
}

func (h *Host) exportLocked(ref ExportRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	unit, _, err := h.unitLocked(ref.Unit())
	if err != nil {
		return err
	}
	for _, export := range unit.Declaration.Exports {
		if export.Name == ref.Export && export.Contract == ref.Contract {
			return nil
		}
	}
	return fmt.Errorf("%w: exact export %s", ErrContractMismatch, ref.Export)
}

func (h *Host) candidatesLocked(contract ContractRef) []ExportRef {
	var result []ExportRef
	for _, installation := range h.installations {
		if !installation.record.Enabled {
			continue
		}
		for _, unit := range installation.record.Units {
			for _, export := range unit.Exports {
				if export.Contract == contract {
					result = append(result, ExportRef{InstallationID: installation.record.ID, Release: installation.record.Release, UnitID: unit.ID, Export: export.Name, Contract: contract})
				}
			}
		}
	}
	slices.SortFunc(result, func(a, b ExportRef) int {
		if value := compareUnit(a.Unit(), b.Unit()); value != 0 {
			return value
		}
		if a.Export < b.Export {
			return -1
		}
		if a.Export > b.Export {
			return 1
		}
		return 0
	})
	return result
}

type BindingChoice struct {
	Unit      UnitRef
	Slot      string
	Providers []ExportRef
	// Failure records a consumer policy refusal for this slot. It is evaluated
	// only if the selected graph reaches the slot and is never persisted.
	Failure error
}

type choiceKey struct {
	unit UnitRef
	slot string
}

type bindingRead struct {
	unit       UnitRef
	slot       string
	contract   ContractRef
	candidates []ExportRef
}

type bindingPreparation struct {
	reads []bindingRead
}

func (h *Host) validateBindingReadsLocked(reads []bindingRead) error {
	for _, read := range reads {
		if !slices.Equal(read.candidates, h.candidatesLocked(read.contract)) {
			return fmt.Errorf("%w: providers changed for %s/%s import %s", ErrStaleBinding, read.unit.InstallationID, read.unit.UnitID, read.slot)
		}
	}
	return nil
}

func (h *Host) resolveLocked(roots []ExportRef, unitRoots []UnitRef, realm RealmID, choices []BindingChoice) (*BindingPlan, error) {
	selected := make(map[choiceKey][]ExportRef, len(choices))
	refused := make(map[choiceKey]error)
	for _, choice := range choices {
		if choice.Providers == nil && choice.Failure == nil || choice.Providers != nil && choice.Failure != nil {
			return nil, ErrInvalid
		}
		unit, _, err := h.unitLocked(choice.Unit)
		if err != nil {
			return nil, err
		}
		found := false
		for _, declared := range unit.Declaration.Imports {
			if declared.Slot != choice.Slot {
				continue
			}
			found = true
			if declared.Providers != nil && !slices.Equal(declared.Providers, choice.Providers) {
				return nil, ErrContractMismatch
			}
		}
		if !found {
			return nil, ErrContractMismatch
		}
		key := choiceKey{unit: choice.Unit, slot: choice.Slot}
		if _, duplicate := selected[key]; duplicate {
			return nil, ErrInvalid
		}
		selected[key] = slices.Clone(choice.Providers)
		refused[key] = choice.Failure
	}
	result := BindingSnapshot{FormatVersion: 1, Roots: slices.Clone(roots), UnitRoots: slices.Clone(unitRoots), Units: []UnitBinding{}}
	if result.Roots == nil {
		result.Roots = []ExportRef{}
	}
	preparation := &bindingPreparation{}
	visiting, visited := make(map[UnitRef]bool), make(map[UnitRef]bool)
	var path []string
	var resolve func(UnitRef) error
	resolve = func(ref UnitRef) error {
		if visiting[ref] {
			return fmt.Errorf("%w: %s", ErrDependencyCycle, strings.Join(append(slices.Clone(path), ref.InstallationID.String()+"/"+string(ref.UnitID)), " -> "))
		}
		if visited[ref] {
			return nil
		}
		unit, installation, err := h.unitLocked(ref)
		if err != nil {
			return err
		}
		visiting[ref] = true
		path = append(path, ref.InstallationID.String()+"/"+string(ref.UnitID))
		defer func() { path = path[:len(path)-1] }()
		bound := UnitBinding{Source: ref, Configuration: Hash(installation.record.Configuration), Imports: make([]ImportDeclaration, len(unit.Declaration.Imports))}
		requirements := unit.Declaration.Imports
		actualRealm, err := h.realmLocked(unit.Declaration, realm)
		if err != nil {
			return err
		}
		if existing := h.instances[instanceKey{source: ref, realm: actualRealm}]; existing != nil && !existing.retiring && existing.binding.Configuration == bound.Configuration {
			requirements = existing.binding.Imports
			preparation.reads = append(preparation.reads, existing.preparation...)
		}
		for i, requirement := range requirements {
			if failure := refused[choiceKey{unit: ref, slot: requirement.Slot}]; failure != nil {
				return fmt.Errorf("unit %s/%s requires %s: %w", ref.InstallationID, ref.UnitID, requirement.Slot, failure)
			}
			bound.Imports[i] = requirement.Clone()
			providers := requirement.Providers
			if providers == nil {
				preparation.reads = append(preparation.reads, bindingRead{unit: ref, slot: requirement.Slot, contract: requirement.Contract, candidates: h.candidatesLocked(requirement.Contract)})
			}
			if chosen, exists := selected[choiceKey{unit: ref, slot: requirement.Slot}]; exists {
				if providers != nil && !slices.Equal(providers, chosen) {
					return fmt.Errorf("%w: existing setup binding for %s", ErrStaleBinding, requirement.Slot)
				}
				providers = chosen
			}
			if providers == nil {
				providers = h.candidatesLocked(requirement.Contract)
				if requirement.Contract.Kind == ServiceKind && len(providers) > 1 {
					return fmt.Errorf("%w: import %s", ErrAmbiguous, requirement.Slot)
				}
			}
			if len(providers) == 0 && !requirement.Optional {
				return fmt.Errorf("%w: required import %s", ErrUnavailable, requirement.Slot)
			}
			if providers == nil {
				providers = []ExportRef{}
			}
			bound.Imports[i].Providers = slices.Clone(providers)
			for _, provider := range providers {
				if provider.Contract != requirement.Contract {
					return ErrContractMismatch
				}
				if err := h.exportLocked(provider); err != nil {
					return err
				}
				if provider.Unit() == ref {
					continue
				}
				if err := resolve(provider.Unit()); err != nil {
					return fmt.Errorf("unit %s/%s requires %s: %w", ref.InstallationID, ref.UnitID, requirement.Slot, err)
				}
			}
		}
		delete(visiting, ref)
		visited[ref] = true
		result.Units = append(result.Units, bound)
		return nil
	}
	for _, root := range roots {
		if err := h.exportLocked(root); err != nil {
			return nil, err
		}
		if err := resolve(root.Unit()); err != nil {
			return nil, err
		}
	}
	for _, root := range unitRoots {
		if err := resolve(root); err != nil {
			return nil, err
		}
	}
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return &BindingPlan{owner: h.identity, binding: result, realm: realm, durable: true, preparation: preparation}, nil
}

// CheckSource validates a declaration projection without starting execution or
// making a use grant. Dispatch still requires the original shared admission.
func (h *Host) CheckSource(ctx context.Context, source ExportRef) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ErrClosed
	}
	return h.exportLocked(source)
}

// Plan resolves declarations without starting code or granting a Use. The
// returned plan retains candidate reads, including absence, for admission.
func (h *Host) Plan(ctx context.Context, roots ...ExportRef) (*BindingPlan, error) {
	return h.PlanWith(ctx, "", nil, roots...)
}

// PlanWith accepts a consumer's explicit choices for unresolved slots. The
// choices are an input to this capture, never another mutable routing table.
// An existing instance keeps its original setup binding or rejects the capture.
func (h *Host) PlanWith(ctx context.Context, realm RealmID, choices []BindingChoice, roots ...ExportRef) (*BindingPlan, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrClosed
	}
	return h.resolveLocked(roots, nil, realm, choices)
}

// PlanUnit captures a unit with no required public export, such as an approved
// background entry. Unit ownership still includes its complete exact imports.
func (h *Host) PlanUnit(ctx context.Context, realm RealmID, root UnitRef, choices []BindingChoice) (*BindingPlan, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrClosed
	}
	return h.resolveLocked(nil, []UnitRef{root}, realm, choices)
}

func subsetBinding(source BindingSnapshot, roots []ExportRef) (BindingSnapshot, error) {
	var unitRoots []UnitRef
	if slices.Equal(source.Roots, roots) {
		unitRoots = source.UnitRoots
	}
	return subsetBindingRoots(source, roots, unitRoots)
}

func subsetBindingRoots(source BindingSnapshot, roots []ExportRef, unitRoots []UnitRef) (BindingSnapshot, error) {
	units := make(map[UnitRef]UnitBinding, len(source.Units))
	for _, unit := range source.Units {
		units[unit.Source] = unit
	}
	result := BindingSnapshot{FormatVersion: 1, Roots: slices.Clone(roots), UnitRoots: slices.Clone(unitRoots), Units: []UnitBinding{}, Rule: source.Rule}
	if result.Roots == nil {
		result.Roots = []ExportRef{}
	}
	seen := make(map[UnitRef]bool)
	var visit func(UnitRef) error
	visit = func(ref UnitRef) error {
		if seen[ref] {
			return nil
		}
		unit, found := units[ref]
		if !found {
			return fmt.Errorf("%w: incomplete exact graph", ErrInvalid)
		}
		seen[ref] = true
		for _, requirement := range unit.Imports {
			for _, provider := range requirement.Providers {
				if err := visit(provider.Unit()); err != nil {
					return err
				}
			}
		}
		result.Units = append(result.Units, unit.Clone())
		return nil
	}
	for _, root := range roots {
		if err := visit(root.Unit()); err != nil {
			return BindingSnapshot{}, err
		}
	}
	for _, root := range unitRoots {
		if err := visit(root); err != nil {
			return BindingSnapshot{}, err
		}
	}
	return result, result.Validate()
}

func sameUnitBinding(a, b UnitBinding) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}

// ValidateBindingUnder checks an original durable identity against current
// installation authority. It never chooses a new provider or updates absence.
func (h *Host) ValidateBindingUnder(ctx context.Context, binding BindingSnapshot) error {
	if err := h.admission.CheckAdmitted(ctx); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.validateBindingLocked(binding)
}

func (h *Host) validateBindingLocked(binding BindingSnapshot) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	if h.closed {
		return ErrClosed
	}
	for _, unit := range binding.Units {
		declaration, installation, err := h.unitLocked(unit.Source)
		if err != nil {
			return err
		}
		if Hash(installation.record.Configuration) != unit.Configuration {
			return ErrStaleBinding
		}
		if err := h.admission.ValidateScope(installation.scope); err != nil {
			return err
		}
		if len(declaration.Declaration.Imports) != len(unit.Imports) {
			return ErrStaleBinding
		}
		for _, bound := range unit.Imports {
			found := false
			for _, declared := range declaration.Declaration.Imports {
				if declared.Slot == bound.Slot && declared.Contract == bound.Contract && declared.Optional == bound.Optional {
					if declared.Providers != nil && !slices.Equal(declared.Providers, bound.Providers) {
						return ErrStaleBinding
					}
					found = true
					break
				}
			}
			if !found {
				return ErrStaleBinding
			}
			for _, provider := range bound.Providers {
				if err := h.exportLocked(provider); err != nil {
					return err
				}
			}
		}
	}
	for _, root := range binding.Roots {
		if err := h.exportLocked(root); err != nil {
			return err
		}
	}
	return nil
}

// Restore projects committed installation records without starting resources or
// reopening the global recovery barrier. The caller must also restore retained
// business dependencies and execution protection before finishing recovery.
func (h *Host) Restore(ctx context.Context, records []InstallationRecord, plugins []*Plugin) error {
	if len(records) > h.maxInstallations {
		return ErrCapacity
	}
	definitions := make(map[Digest]*Plugin, len(plugins))
	for _, plugin := range plugins {
		if plugin == nil {
			return ErrInvalid
		}
		definitions[plugin.revision] = plugin
	}
	return h.admission.Maintenance(ctx, func() error {
		h.mu.Lock()
		defer h.mu.Unlock()
		if len(h.allInstances) != 0 || len(h.uses) != 0 {
			return ErrInUse
		}
		restored := make(map[InstallationID]*installed, len(records))
		var scopes []ScopeID
		for _, record := range records {
			if err := record.ID.Validate(); err != nil {
				return err
			}
			if err := installationScope(record.ID).Validate(); err != nil {
				return err
			}
			plugin := definitions[record.Release]
			if plugin == nil {
				return fmt.Errorf("%w: definition for retained release", ErrUnavailable)
			}
			if _, exists := restored[record.ID]; exists {
				return ErrInvalid
			}
			if err := h.validateUnitsLocked(record.Units); err != nil {
				return err
			}
			if !sameUnitDeclarations(record.Units, plugin.Units()) {
				return ErrContractMismatch
			}
			entry := &installed{record: record.Clone(), plugin: plugin}
			if record.Enabled {
				scopes = append(scopes, installationScope(record.ID))
			}
			restored[record.ID] = entry
		}
		if err := h.admission.checkScopeCapacity(scopes...); err != nil {
			return err
		}
		for _, entry := range restored {
			if !entry.record.Enabled {
				continue
			}
			var err error
			entry.scope, err = h.admission.openScope(installationScope(entry.record.ID))
			if err != nil {
				return err
			}
		}
		h.installations = restored
		return nil
	})
}

func sameUnitDeclarations(a, b []UnitDeclaration) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}

package plugins

import (
	"context"
	"errors"
	"slices"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"
)

type PointReference struct {
	Installation identity.InstallationID
	Contribution plugin.ContributionID
}

func (r PointReference) key() string { return r.Installation.String() + "/" + r.Contribution.String() }

type PointBinding struct {
	Reference PointReference
	Digest    framevalue.Digest
	Value     framevalue.JSON
}

// ResolvePoint admits consumer-owned values without executing a backend. Order
// is an immutable projection supplied by the consuming composition owner.
func ResolvePoint(ctx context.Context, host *frame.Host, store Store, contract *plugincontract.Point, target plugincontract.PointContext, order []PointReference) ([]PointBinding, error) {
	if host == nil || contract == nil {
		return nil, plugin.ErrInvalid
	}
	applicable, err := contract.Applicable(target)
	if err != nil {
		return nil, errors.Join(plugin.ErrInvalid, err)
	}
	snapshots, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	var declarations []plugin.Dependency
	var providers []frame.UnitRef
	for _, snapshot := range snapshots {
		if !snapshot.Installation.Enabled() {
			continue
		}
		declaration := snapshot.Selected.Declaration()
		declaresTarget := target.Target.Scope != extension.ScopeInstallation || target.Target.Reference == snapshot.Installation.ID().String()
		for _, definition := range declaration.Points {
			if definition.Contract == contract.Identity() && declaresTarget {
				declarations = append(declarations, snapshot.Installation.Dependency())
			}
		}
		if slices.ContainsFunc(declaration.PointValues, func(value plugin.PointValue) bool { return value.Contract == contract.Identity() }) {
			unit := frame.UnitRef{InstallationID: snapshot.Installation.ID().Host(), Release: snapshot.Selected.Digest(), UnitID: PointValuesUnit}
			providers = append(providers, unit)
		}
	}
	if len(declarations) == 0 {
		return nil, plugin.ErrUnavailable
	}
	if !applicable {
		return []PointBinding{}, nil
	}
	for _, provider := range providers {
		if _, err := host.ActivateUnit(ctx, "", provider); err != nil {
			return nil, FrameworkError(err)
		}
	}
	point, err := DataPoint(contract)
	if err != nil {
		return nil, err
	}
	snapshot, err := host.Snapshot(ctx, point.Ref())
	if err != nil {
		return nil, FrameworkError(err)
	}
	candidates, err := frame.Candidates(snapshot, point)
	if err != nil {
		return nil, FrameworkError(err)
	}
	type boundCandidate struct {
		binding   PointBinding
		candidate frame.Candidate[framevalue.JSON, framevalue.JSON]
	}
	bindings := make(map[string]boundCandidate, len(candidates))
	values := make([]plugincontract.PointValue, 0, len(candidates))
	for _, candidate := range candidates {
		binding, err := readPointBinding(candidate)
		if err != nil {
			return nil, err
		}
		key := binding.Reference.key()
		if _, exists := bindings[key]; exists {
			return nil, plugin.ErrInvalid
		}
		bindings[key] = boundCandidate{binding: binding, candidate: candidate}
		values = append(values, plugincontract.PointValue{Key: key, Data: binding.Value})
	}
	keys := make([]string, 0, len(order))
	for _, reference := range order {
		if err := reference.Installation.Validate(); err != nil {
			return nil, err
		}
		if err := reference.Contribution.Validate(); err != nil {
			return nil, err
		}
		keys = append(keys, reference.key())
	}
	composed, err := contract.Compose(values, keys)
	if err != nil {
		return nil, errors.Join(plugin.ErrUnavailable, err)
	}
	result := make([]PointBinding, 0, len(composed))
	selected := make([]frame.Candidate[framevalue.JSON, framevalue.JSON], 0, len(composed))
	for _, value := range composed {
		binding := bindings[value.Key]
		result = append(result, binding.binding)
		selected = append(selected, binding.candidate)
	}
	selection, err := frame.Select(snapshot, point, "flame-point/1", selected...)
	if err != nil {
		return nil, FrameworkError(err)
	}
	plan, err := frame.Capture(snapshot, selection.Ref())
	if err != nil {
		return nil, FrameworkError(err)
	}
	if err := host.Admission().Admit(ctx, func(admitted context.Context) error {
		if err := validatePointDeclaration(admitted, store, declarations); err != nil {
			return err
		}
		_, err := host.ValidatePlanUnder(admitted, plan)
		return err
	}); err != nil {
		return nil, FrameworkError(err)
	}
	return result, nil
}

func readPointBinding(candidate frame.Candidate[framevalue.JSON, framevalue.JSON]) (PointBinding, error) {
	source := candidate.Source()
	installation, err := identity.ParseHostInstallation(source.InstallationID)
	if err != nil {
		return PointBinding{}, err
	}
	contribution, err := plugin.ParseContributionID(source.Export)
	if err != nil {
		return PointBinding{}, err
	}
	if err := source.Release.Validate(); err != nil {
		return PointBinding{}, err
	}
	value, err := candidate.Metadata()
	if err != nil {
		return PointBinding{}, FrameworkError(err)
	}
	return PointBinding{
		Reference: PointReference{Installation: installation, Contribution: contribution},
		Digest:    source.Release, Value: value,
	}, nil
}

func validatePointDeclaration(ctx context.Context, store Store, declarations []plugin.Dependency) error {
	for _, declaration := range declarations {
		current, found, err := store.Get(ctx, declaration.InstallationID)
		if err != nil {
			return err
		}
		if found && current.Installation.Enabled() && current.Installation.Dependency() == declaration {
			return nil
		}
	}
	return plugin.ErrUnavailable
}

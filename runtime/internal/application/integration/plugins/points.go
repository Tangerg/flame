package plugins

import (
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type PointReference struct {
	Installation identity.InstallationID
	Contribution plugin.ContributionID
}

func (r PointReference) key() string { return r.Installation.String() + "/" + r.Contribution.String() }

type PointBinding struct {
	Reference PointReference
	Digest    identity.Digest
	Value     jsonvalue.Value
}

// ResolvePoint admits consumer-owned values without executing a backend. Order
// is an immutable projection supplied by the consuming composition owner.
func ResolvePoint(ctx context.Context, host *qimu.Host, store Store, contract *portable.Point, target portable.PointContext, order []PointReference) ([]PointBinding, error) {
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
	var providers []qimu.UnitRef
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
			unit := qimu.UnitRef{InstallationID: qimu.InstallationID(snapshot.Installation.ID().String()), Release: qimu.Digest(snapshot.Selected.Digest().String()), UnitID: PointValuesUnit}
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
	candidates, err := qimu.Candidates(snapshot, point)
	if err != nil {
		return nil, FrameworkError(err)
	}
	bindings := make(map[string]PointBinding, len(candidates))
	selectedByKey := make(map[string]qimu.Candidate[jsonvalue.Value, jsonvalue.Value], len(candidates))
	values := make([]portable.PointValue, 0, len(candidates))
	for _, candidate := range candidates {
		source := candidate.Source()
		installation, err := identity.ParseInstallation(source.InstallationID.String())
		if err != nil {
			return nil, err
		}
		contribution, err := plugin.ParseContributionID(source.Export)
		if err != nil {
			return nil, err
		}
		digest, err := identity.ParseDigest(source.Release.String())
		if err != nil {
			return nil, err
		}
		reference := PointReference{Installation: installation, Contribution: contribution}
		key := reference.key()
		if _, exists := bindings[key]; exists {
			return nil, plugin.ErrInvalid
		}
		value := candidate.Metadata()
		bindings[key] = PointBinding{Reference: reference, Digest: digest, Value: value}
		selectedByKey[key] = candidate
		values = append(values, portable.PointValue{Key: key, Data: value})
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
	selected := make([]qimu.Candidate[jsonvalue.Value, jsonvalue.Value], 0, len(composed))
	for _, value := range composed {
		result = append(result, bindings[value.Key])
		selected = append(selected, selectedByKey[value.Key])
	}
	selection, err := qimu.Select(snapshot, point, "flame-point/1", selected...)
	if err != nil {
		return nil, FrameworkError(err)
	}
	plan, err := qimu.Capture(snapshot, selection.Ref())
	if err != nil {
		return nil, FrameworkError(err)
	}
	if err := host.Admission().Admit(ctx, func(admitted context.Context) error {
		if err := validatePointDeclaration(admitted, store, declarations); err != nil {
			return err
		}
		return host.ValidateSelectionUnder(admitted, plan)
	}); err != nil {
		return nil, FrameworkError(err)
	}
	return result, nil
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

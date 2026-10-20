package plugins

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/point"
	"github.com/Tangerg/flame/runtime/resourceid"
)

type PointReference struct {
	Installation resourceid.InstallationID
	Contribution plugin.ContributionID
}

func (r PointReference) key() string { return r.Installation.String() + "/" + r.Contribution.String() }

type PointBinding struct {
	Reference PointReference
	Digest    fingerprint.Digest
	Value     jsoninput.Input
}

// ResolvePoint admits consumer-owned values without executing a backend. Order
// is an immutable projection supplied by the consuming composition owner.
func ResolvePoint(ctx context.Context, store Store, contract *point.Contract, target point.Context, order []PointReference) ([]PointBinding, error) {
	if contract == nil {
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
	declared := false
	bindings := map[string]PointBinding{}
	var values []point.Value
	for _, snapshot := range snapshots {
		if !snapshot.Installation.Enabled() {
			continue
		}
		declaration := snapshot.Selected.Declaration()
		declaresTarget := target.Target.Scope != "installation" || target.Target.Reference == snapshot.Installation.ID().String()
		for _, definition := range declaration.Points {
			if definition.Contract == contract.Identity() && declaresTarget {
				declared = true
			}
		}
		for _, value := range declaration.PointValues {
			if value.Contract != contract.Identity() {
				continue
			}
			reference := PointReference{Installation: snapshot.Installation.ID(), Contribution: value.ID}
			key := reference.key()
			if _, found := bindings[key]; found {
				return nil, plugin.ErrInvalid
			}
			bindings[key] = PointBinding{Reference: reference, Digest: snapshot.Selected.Digest(), Value: value.Value}
			values = append(values, point.Value{Key: key, Data: value.Value})
		}
	}
	if !declared {
		return nil, plugin.ErrUnavailable
	}
	if !applicable {
		return []PointBinding{}, nil
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
	for _, value := range composed {
		result = append(result, bindings[value.Key])
	}
	return result, nil
}

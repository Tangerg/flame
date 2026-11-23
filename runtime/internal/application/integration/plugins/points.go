package plugins

import (
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framecall "github.com/Tangerg/frame/call"
	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
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

const pointSelectionRule = "flame-point/1"

func ResolvePoint(ctx context.Context, host framehost.Host, store Store, contract *plugincontract.Point, target plugincontract.PointContext, order []PointReference) ([]PointBinding, error) {
	if !host.Valid() || contract == nil {
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
	participants := collectPointParticipants(snapshots, contract.Identity(), target.Target)
	if len(participants.declarations) == 0 {
		return nil, plugin.ErrUnavailable
	}
	if !applicable {
		return []PointBinding{}, nil
	}
	if err := activatePointProviders(ctx, host, participants.providers); err != nil {
		return nil, err
	}
	result, plan, err := capturePointComposition(ctx, host, contract, order)
	if err != nil {
		return nil, err
	}
	if err := host.Admission().Admit(ctx, func(admitted context.Context) error {
		if err := validatePointDeclaration(admitted, store, participants.declarations); err != nil {
			return err
		}
		_, err := host.ValidatePlanUnder(admitted, plan)
		return err
	}); err != nil {
		return nil, FrameworkError(err)
	}
	return result, nil
}

func capturePointComposition(ctx context.Context, host framehost.Host, contract *plugincontract.Point, order []PointReference) ([]PointBinding, framehost.BindingPlan, error) {
	point, err := DataPoint(contract)
	if err != nil {
		return nil, framehost.BindingPlan{}, err
	}
	snapshot, err := host.Snapshot(ctx, point.Ref())
	if err != nil {
		return nil, framehost.BindingPlan{}, FrameworkError(err)
	}
	candidates, err := framehost.Candidates(snapshot, point)
	if err != nil {
		return nil, framehost.BindingPlan{}, FrameworkError(err)
	}
	bound, err := bindPointCandidates(candidates)
	if err != nil {
		return nil, framehost.BindingPlan{}, err
	}
	keys, err := pointOrderKeys(order)
	if err != nil {
		return nil, framehost.BindingPlan{}, err
	}
	composed, err := contract.Compose(bound.values, keys)
	if err != nil {
		return nil, framehost.BindingPlan{}, errors.Join(plugin.ErrUnavailable, err)
	}
	result := make([]PointBinding, 0, len(composed))
	selected := make([]framehost.Candidate[framevalue.JSON, framevalue.JSON], 0, len(composed))
	for _, value := range composed {
		candidate := bound.byKey[value.Key]
		result = append(result, candidate.binding)
		selected = append(selected, candidate.candidate)
	}
	selection, err := framehost.Select(snapshot, point, pointSelectionRule, selected...)
	if err != nil {
		return nil, framehost.BindingPlan{}, FrameworkError(err)
	}
	plan, err := framehost.Capture(snapshot, selection.Ref())
	if err != nil {
		return nil, framehost.BindingPlan{}, FrameworkError(err)
	}
	return result, plan, nil
}

type pointParticipants struct {
	declarations []plugin.Dependency
	providers    []framehost.UnitRef
}

func collectPointParticipants(snapshots []plugin.Snapshot, contract framevalue.JSON, target framecall.Target) pointParticipants {
	var participants pointParticipants
	for _, snapshot := range snapshots {
		if !snapshot.Installation.Enabled() {
			continue
		}
		declaration := snapshot.Selected.Declaration()
		declaresTarget := target.Scope != extension.ScopeInstallation || target.Reference == snapshot.Installation.ID().String()
		for _, definition := range declaration.Points {
			if definition.Contract == contract && declaresTarget {
				participants.declarations = append(participants.declarations, snapshot.Installation.Dependency())
			}
		}
		if slices.ContainsFunc(declaration.PointValues, func(value plugin.PointValue) bool { return value.Contract == contract }) {
			participants.providers = append(participants.providers, framehost.UnitRef{InstallationID: snapshot.Installation.ID().Host(), Release: snapshot.Selected.Digest(), UnitID: PointValuesUnit})
		}
	}
	return participants
}

type boundPointCandidate struct {
	binding   PointBinding
	candidate framehost.Candidate[framevalue.JSON, framevalue.JSON]
}

type boundPointCandidates struct {
	byKey  map[string]boundPointCandidate
	values []plugincontract.PointValue
}

func bindPointCandidates(candidates []framehost.Candidate[framevalue.JSON, framevalue.JSON]) (boundPointCandidates, error) {
	bound := boundPointCandidates{byKey: make(map[string]boundPointCandidate, len(candidates)), values: make([]plugincontract.PointValue, 0, len(candidates))}
	for _, candidate := range candidates {
		binding, err := readPointBinding(candidate)
		if err != nil {
			return boundPointCandidates{}, err
		}
		key := binding.Reference.key()
		if _, exists := bound.byKey[key]; exists {
			return boundPointCandidates{}, plugin.ErrInvalid
		}
		bound.byKey[key] = boundPointCandidate{binding: binding, candidate: candidate}
		bound.values = append(bound.values, plugincontract.PointValue{Key: key, Data: binding.Value})
	}
	return bound, nil
}

func pointOrderKeys(order []PointReference) ([]string, error) {
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
	return keys, nil
}

func activatePointProviders(ctx context.Context, host framehost.Host, providers []framehost.UnitRef) error {
	for _, provider := range providers {
		if err := AwaitNativeUnitRetirement(ctx, host, provider); err != nil {
			return FrameworkError(err)
		}
		if _, err := host.ActivateUnit(ctx, "", provider); err != nil {
			return FrameworkError(err)
		}
	}
	return nil
}

func readPointBinding(candidate framehost.Candidate[framevalue.JSON, framevalue.JSON]) (PointBinding, error) {
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
	return PointBinding{
		Reference: PointReference{Installation: installation, Contribution: contribution},
		Digest:    source.Release, Value: candidate.Metadata,
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

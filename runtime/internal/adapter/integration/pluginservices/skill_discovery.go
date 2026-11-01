package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func (s *SkillGovernance) Discover(ctx context.Context, candidates skilllearning.Candidates, capture *promptsource.SkillCapture) (skilllearning.Catalog, error) {
	var zero skilllearning.Catalog
	if capture != nil && capture.Restore {
		if capture.Command == "" {
			return zero, nil
		}
		return s.restoreCatalog(ctx, candidates.Library, capture)
	}
	if len(candidates.Items) == 0 {
		return zero, nil
	}
	c := s.calls
	binding, borrowed, err := contributionSession(ctx, c.framework, c.installations, c.selections, c.pool, s.contract)
	if errors.Is(err, errInactive) {
		return zero, nil
	}
	if err != nil {
		return zero, err
	}
	defer borrowed.Close()
	ctx = borrowed.Context()
	if capture == nil {
		method, err := skilllearning.DiscoverMethod()
		if err != nil {
			return zero, err
		}
		catalog, err := query[skilllearning.Candidates, skilllearning.Catalog](ctx, binding, borrowed.Session(), method.Name(), candidates, sdk.ParamsBase{})
		if err != nil {
			return zero, err
		}
		return catalog, admitSkillChoices(candidates, catalog)
	}
	method, err := skilllearning.CaptureMethod()
	if err != nil {
		return zero, err
	}
	ctx = operation.Start(ctx, extension.Automatic, method.Name(), "")
	reply, err := invokeResolvedCommand[skilllearning.Candidates, skilllearning.Catalog](ctx, c, method.Name(), candidates, func(catalog skilllearning.Catalog) error { return admitSkillChoices(candidates, catalog) }, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, nil)
	if err != nil {
		return zero, err
	}
	capture.Command = operation.Identity(ctx)
	record, err := c.routing.Get(ctx, capture.Command)
	if err != nil {
		return zero, err
	}
	capture.Activation = record.Snapshot().Activation
	return reply.value, nil
}
func (s *SkillGovernance) restoreCatalog(ctx context.Context, library string, capture *promptsource.SkillCapture) (skilllearning.Catalog, error) {
	var zero skilllearning.Catalog
	cmd, err := s.calls.routing.Get(ctx, capture.Command)
	if err != nil {
		return zero, err
	}
	record := cmd.Snapshot()
	method, err := skilllearning.CaptureMethod()
	if err != nil {
		return zero, err
	}
	if record.Method != method.Name() || record.Contract != s.contract.Identity() {
		return zero, plugin.ErrStale
	}
	var original skilllearning.Candidates
	if err := json.Unmarshal(record.Input.JSON(), &original, json.RejectUnknownMembers(true)); err != nil {
		return zero, err
	}
	if original.Library != library {
		return zero, plugin.ErrStale
	}
	ctx = operation.Adopt(ctx, record.ID, record.Invocation.Origin, record.Invocation.Parent)
	reply, err := invokeResolvedCommand[skilllearning.Candidates, skilllearning.Catalog](ctx, s.calls, method.Name(), original, func(catalog skilllearning.Catalog) error { return admitSkillChoices(original, catalog) }, func(ctx context.Context) (plugins.ContributionBinding, error) { return s.calls.original(ctx, record) }, nil)
	if err != nil {
		return zero, err
	}
	capture.Activation = record.Activation
	return reply.value, nil
}

func admitSkillChoices(input skilllearning.Candidates, output skilllearning.Catalog) error {
	allowed := make(map[skilllearning.Choice]bool, len(input.Items))
	counts := make(map[string]int)
	for _, item := range input.Items {
		allowed[skilllearning.Choice{Name: item.Name, Source: item.Source}] = true
		counts[item.Name]++
	}
	seen := make(map[string]bool, len(output.Entries))
	for _, choice := range output.Entries {
		if seen[choice.Name] || (!allowed[choice] && !(choice.Source == "" && counts[choice.Name] > 1)) {
			return plugin.ErrInvalid
		}
		seen[choice.Name] = true
	}
	return nil
}

package main

import (
	"context"
	"errors"
	"maps"
	"slices"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/packages/go-sdk/skill"

	framesqlite "github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func registerDiscovery(server *pluginmcp.Backend, db framesqlite.Database) error {
	discover, err := skilllearning.DiscoverMethod()
	if err != nil {
		return err
	}
	capture, err := skilllearning.CaptureMethod()
	if err != nil {
		return err
	}
	return errors.Join(discover.RegisterServer(server, nil, func(_ context.Context, _ *sdk.ServerSession, c pluginmcp.Command[skilllearning.Candidates]) (skilllearning.Catalog, error) {
		return selectSources(c.Input), nil
	}),
		pluginstorage.RegisterCommand(server, db, capture, nil, func(_ context.Context, _ *sdk.ServerSession, c skilllearning.Candidates) (pluginstorage.CommandPlan[skilllearning.Catalog], error) {
			return pluginstorage.NewCommandPlan(selectSources(c), nil)
		}))
}

// A later scope overrides an earlier one: repository over user over installation.
var sourcePrecedence = []skill.Scope{skill.Installation, skill.User, skill.Repository}

func sourceRank(scope skill.Scope) int { return slices.Index(sourcePrecedence, scope) + 1 }

func selectSources(input skilllearning.Candidates) skilllearning.Catalog {
	type preferred struct {
		rank   int
		source string
	}
	choices := make(map[string]preferred)
	for _, candidate := range input.Items {
		rank := sourceRank(candidate.Scope)
		old := choices[candidate.Name]
		switch {
		case rank > old.rank:
			choices[candidate.Name] = preferred{rank, candidate.Source}
		case rank == old.rank:
			choices[candidate.Name] = preferred{rank: rank}
		}
	}
	catalog := skilllearning.Catalog{Entries: []skilllearning.Choice{}}
	for _, name := range slices.Sorted(maps.Keys(choices)) {
		catalog.Entries = append(catalog.Entries, skilllearning.Choice{Name: name, Source: choices[name].source})
	}
	return catalog
}

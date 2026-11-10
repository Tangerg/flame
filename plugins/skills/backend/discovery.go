package main

import (
	"context"
	"errors"
	"maps"
	"slices"

	framesqlite "github.com/Tangerg/frame/sqlite"

	"github.com/Tangerg/flame/packages/go-sdk/skill"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	storage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func registerDiscovery(server *pluginmcp.Backend, db *framesqlite.Database) error {
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
		storage.RegisterCommand(server, db, capture, nil, func(_ context.Context, _ *sdk.ServerSession, c skilllearning.Candidates) (storage.CommandPlan[skilllearning.Catalog], error) {
			return storage.NewCommandPlan(selectSources(c), nil)
		}))
}
func sourceRank(scope skill.Scope) int {
	switch scope {
	case skill.Project:
		return 3
	case skill.User:
		return 2
	case skill.Installation:
		return 1
	default:
		return 0
	}
}
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

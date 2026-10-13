package plugins

import (
	"context"
	"testing"
)

type constructionContext struct{ context.Context }

func TestCoordinatorRejectsMissingCollaborators(t *testing.T) {
	type collaborators struct {
		lifetime     context.Context
		store        Store
		catalog      Catalog
		packages     Packages
		connections  Connections
		dependencies Dependencies
	}
	for _, test := range []struct {
		name   string
		remove func(*collaborators)
	}{
		{"lifetime", func(c *collaborators) { c.lifetime = nil }},
		{"typed nil lifetime", func(c *collaborators) { c.lifetime = (*constructionContext)(nil) }},
		{"store", func(c *collaborators) { c.store = nil }},
		{"typed nil store", func(c *collaborators) { c.store = (*installationMemory)(nil) }},
		{"catalog", func(c *collaborators) { c.catalog = nil }},
		{"typed nil catalog", func(c *collaborators) { c.catalog = (*releaseMemory)(nil) }},
		{"packages", func(c *collaborators) { c.packages = nil }},
		{"typed nil packages", func(c *collaborators) { c.packages = (*installationPackages)(nil) }},
		{"connections", func(c *collaborators) { c.connections = nil }},
		{"typed nil connections", func(c *collaborators) { c.connections = (*installationConnections)(nil) }},
		{"dependencies", func(c *collaborators) { c.dependencies = nil }},
		{"typed nil dependencies", func(c *collaborators) { c.dependencies = (*installationDependencies)(nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := collaborators{t.Context(), &installationMemory{}, releaseMemory{}, installationPackages{}, installationConnections{}, &installationDependencies{}}
			test.remove(&c)
			coordinator, err := New(c.lifetime, c.store, c.catalog, c.packages, c.connections, c.dependencies, nil)
			if err == nil || coordinator != nil {
				t.Fatalf("invalid coordinator constructed: %v, %v", coordinator, err)
			}
		})
	}
}

func TestRegistryRejectsMissingCollaborators(t *testing.T) {
	for _, selections := range []SelectionStore{nil, (*requirementSelections)(nil)} {
		if registry, err := NewRegistry(userMemory{}, &installationMemory{}, &singleServerPackages{}, selections); err == nil || registry != nil {
			t.Fatalf("missing selection store constructed a registry: %v, %v", registry, err)
		}
	}
	for _, test := range []struct {
		name          string
		user          userServers
		installations Store
		packages      PackageSources
	}{
		{"user", nil, &installationMemory{}, &singleServerPackages{}},
		{"typed nil user", (*userMemory)(nil), &installationMemory{}, &singleServerPackages{}},
		{"installations", userMemory{}, nil, &singleServerPackages{}},
		{"typed nil installations", userMemory{}, (*installationMemory)(nil), &singleServerPackages{}},
		{"packages", userMemory{}, &installationMemory{}, nil},
		{"typed nil packages", userMemory{}, &installationMemory{}, (*singleServerPackages)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry, err := NewRegistry(test.user, test.installations, test.packages, requirementSelections{})
			if err == nil || registry != nil {
				t.Fatalf("invalid registry constructed: %v, %v", registry, err)
			}
		})
	}
}

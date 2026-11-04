package plugins

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"

	"github.com/Tangerg/frame"
)

type constructionContext struct{ context.Context }

func TestCoordinatorRejectsMissingCollaborators(t *testing.T) {
	type collaborators struct {
		lifetime    context.Context
		store       Store
		catalog     catalog
		packages    packages
		connections connections
		host        *frame.Host
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
		{"host", func(c *collaborators) { c.host = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := collaborators{t.Context(), &installationMemory{}, releaseMemory{}, installationPackages{}, installationConnections{}, testFramework(t, nil)}
			test.remove(&c)
			coordinator, err := newTestCoordinator(c.lifetime, c.host, c.store, c.catalog, c.packages, c.connections, nil)
			if err == nil || coordinator != nil {
				t.Fatalf("invalid coordinator constructed: %v, %v", coordinator, err)
			}
		})
	}
}

func TestRegistryRejectsMissingCollaborators(t *testing.T) {
	for _, selections := range []SelectionStore{nil, (*requirementSelections)(nil)} {
		if registry, err := NewRegistry(testFramework(t, nil), &installationMemory{}, &singleServerPackages{}, selections, &requirementCompositions{}); err == nil || registry != nil {
			t.Fatalf("missing selection store constructed a registry: %v, %v", registry, err)
		}
	}
	for _, test := range []struct {
		name          string
		installations Store
		packages      packageSources
	}{
		{"installations", nil, &singleServerPackages{}},
		{"typed nil installations", (*installationMemory)(nil), &singleServerPackages{}},
		{"packages", &installationMemory{}, nil},
		{"typed nil packages", &installationMemory{}, (*singleServerPackages)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry, err := NewRegistry(testFramework(t, nil), test.installations, test.packages, requirementSelections{}, &requirementCompositions{})
			if err == nil || registry != nil {
				t.Fatalf("invalid registry constructed: %v, %v", registry, err)
			}
		})
	}
}

func newTestCoordinator(lifetime context.Context, host *frame.Host, store Store, catalog catalog, packages packages, connections connections, publish invalidation.Publish) (*Coordinator, error) {
	admission := frame.NewAdmission()
	if host != nil {
		admission = host.Admission()
	}
	return New(lifetime, host, store, catalog, packages, connections, admission.Admit, publish)
}

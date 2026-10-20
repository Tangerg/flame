package pluginpackage

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/resourceid"
)

type afterReadStore struct {
	plugins.Store
	after func()
}

func (s *afterReadStore) fire() {
	if s.after != nil {
		action := s.after
		s.after = nil
		action()
	}
}
func (s *afterReadStore) List(ctx context.Context) ([]plugin.Snapshot, error) {
	values, err := s.Store.List(ctx)
	if err == nil {
		s.fire()
	}
	return values, err
}
func (s *afterReadStore) Get(ctx context.Context, id resourceid.InstallationID) (plugin.Snapshot, bool, error) {
	value, found, err := s.Store.Get(ctx, id)
	if err == nil {
		s.fire()
	}
	return value, found, err
}

func TestReadsSurviveConcurrentReleaseReclaim(t *testing.T) {
	for _, kind := range []string{"plugins.list", "MCP definitions", "MCP definition", "Skills bundles"} {
		t.Run(kind, func(t *testing.T) {
			releases, store, users := testReleaseStore(t)
			source := writePackage(t, map[string]string{
				"plugin.json":            portableManifest,
				"mcp.json":               `{"$schema":"` + MCPSchema + `","mcpServers":{"remote":{"type":"streamable-http","url":"https://example.test/mcp"}}}`,
				"skills/review/SKILL.md": "---\nname: review\ndescription: Review a result\n---\nReview carefully.\n",
			})
			connections := &recordedConnections{}
			owner, err := plugins.New(t.Context(), store, releases.catalog, releases, connections, passthroughDependencies{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			installed, err := owner.Install(t.Context(), source)
			if err != nil {
				t.Fatal(err)
			}
			id := installed.View.ID
			if _, err := owner.Enable(t.Context(), id); err != nil {
				t.Fatal(err)
			}
			wrapped := &afterReadStore{Store: store}
			wrapped.after = func() {
				if err := owner.Uninstall(t.Context(), id); err != nil {
					t.Fatal(err)
				}
				if _, err := releases.catalog.Get(t.Context(), installed.Selected.Digest()); !errors.Is(err, plugin.ErrNotFound) {
					t.Fatalf("release was not reclaimed: %v", err)
				}
			}
			reader, err := plugins.New(t.Context(), wrapped, releases.catalog, releases, connections, passthroughDependencies{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			registry, err := plugins.NewRegistry(users, wrapped, releases, emptySelections{}, emptyCompositions{})
			if err != nil {
				t.Fatal(err)
			}
			current, found, err := store.Get(t.Context(), id)
			if err != nil || !found {
				t.Fatal(err)
			}
			serverID, err := current.Installation.ServerID(testsupport.ServerName("remote"))
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "plugins.list":
				_, err = reader.List(t.Context())
			case "MCP definitions":
				_, err = registry.Definitions(t.Context())
			case "MCP definition":
				_, _, err = registry.Definition(t.Context(), serverID)
			case "Skills bundles":
				_, err = mustPackageSkills(t, releases, wrapped).SkillBundles(t.Context())
			}
			if err != nil {
				t.Fatalf("ordinary concurrent uninstall broke %s: %v", kind, err)
			}
		})
	}
}

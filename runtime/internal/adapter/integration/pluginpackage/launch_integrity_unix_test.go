//go:build unix

package pluginpackage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type replaceAfterAdmissionRegistry struct {
	*plugins.Registry
	replace func()
}

func (r *replaceAfterAdmissionRegistry) Connection(ctx context.Context, id mcpserver.ID) (mcpapp.Launch, error) {
	launch, err := r.Registry.Connection(ctx, id)
	if err == nil && r.replace != nil {
		replace := r.replace
		r.replace = nil
		replace()
	}
	return launch, err
}

func TestLaunchExecutesAdmittedContentAfterPublishedBytesChange(t *testing.T) {
	for _, entry := range []string{"startup", "configure", "reconnect"} {
		for _, mutation := range []string{"replace directory", "rewrite files"} {
			t.Run(entry+"/"+mutation, func(t *testing.T) {
				releases, store, users := testReleaseStore(t)
				source := writePackage(t, map[string]string{
					"plugin.json": portableManifest,
					"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"sh","args":["${PLUGIN_ROOT}/backend.sh"]}}}`,
					"backend.sh":  `printf '%s/%s' "$(cat "$PLUGIN_ROOT/content")" "$(cat ./content)" > "$PLUGIN_DATA/started"`,
					"content":     "original",
				})
				owner, err := plugins.New(t.Context(), store, releases.catalog, releases, &recordedConnections{}, passthroughDependencies{}, nil)
				if err != nil {
					t.Fatal(err)
				}
				installed, err := owner.Install(t.Context(), source)
				if err != nil {
					t.Fatal(err)
				}
				id := installed.View.ID
				if _, err := owner.Approve(t.Context(), id, installed.Selected.Digest()); err != nil {
					t.Fatal(err)
				}
				if _, err := owner.Enable(t.Context(), id); err != nil {
					t.Fatal(err)
				}
				registry, err := plugins.NewRegistry(users, store, releases)
				if err != nil {
					t.Fatal(err)
				}
				current, err := store.Get(t.Context(), id)
				if err != nil {
					t.Fatal(err)
				}
				serverID, err := current.Installation.ServerID(testsupport.ServerName("backend"))
				if err != nil {
					t.Fatal(err)
				}
				root, err := releases.Root(installed.Selected.Digest())
				if err != nil {
					t.Fatal(err)
				}
				wrapped := &replaceAfterAdmissionRegistry{Registry: registry, replace: func() {
					if mutation == "replace directory" {
						if err := os.Rename(root, root+"-retired"); err != nil {
							t.Fatal(err)
						}
						if err := os.Mkdir(root, 0700); err != nil {
							t.Fatal(err)
						}
					} else {
						for _, name := range []string{"backend.sh", "content"} {
							if err := os.Chmod(filepath.Join(root, name), 0600); err != nil {
								t.Fatal(err)
							}
						}
					}
					for name, body := range map[string]string{"backend.sh": `printf unapproved > "$PLUGIN_DATA/started"`, "content": "unapproved"} {
						if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
							t.Fatal(err)
						}
					}
				}}
				var servers []mcpserver.Server
				if entry == "startup" {
					server, found, err := registry.Definition(t.Context(), serverID)
					if err != nil || !found {
						t.Fatalf("definition: %v, %v", found, err)
					}
					servers = []mcpserver.Server{server}
				}
				pool, err := mcpconnection.Open(t.Context(), t.Context(), servers, nil, wrapped)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = pool.Shutdown(context.WithoutCancel(t.Context())) })
				if entry == "configure" {
					_ = pool.Configure(t.Context(), serverID)
				}
				if entry == "reconnect" {
					_ = pool.Reconnect(t.Context(), serverID)
				}
				marker := filepath.Join(releases.dataRoot(id), "started")
				content, err := os.ReadFile(marker)
				if err != nil || string(content) != "original/original" {
					t.Fatalf("launch did not execute the admitted script, environment and working directory: %q, %v", content, err)
				}
				if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(releases.executionDirectory())
				if err != nil || len(entries) != 0 {
					t.Fatalf("failed handshake retained execution content: %d, %v", len(entries), err)
				}
			})
		}
	}
}

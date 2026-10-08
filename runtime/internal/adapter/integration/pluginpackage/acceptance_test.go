package pluginpackage

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

func TestUnknownRootFieldIsDiagnosedAndOtherwiseIgnored(t *testing.T) {
	r := testReleases(t)
	manifest := strings.TrimSuffix(portableManifest, "}") + `,"homepage-extra":{"anything":[1,2]},"extensions":{"io.github.tangerg.flame":{"apiVersion":1,"contributes":{"themes":[{"id":"night","title":"Night","scheme":"dark","colors":{"background":"#101010"}}]}}}}`
	source := writePackage(t, map[string]string{
		"plugin.json":            manifest,
		"mcp.json":               `{"$schema":"` + MCPSchema + `","mcpServers":{"remote":{"type":"streamable-http","url":"https://example.test/mcp"}}}`,
		"skills/review/SKILL.md": "---\nname: review\ndescription: Review a result\n---\nRead the results carefully.\n",
	})
	release, err := publishPackage(t.Context(), r, source)
	if err != nil {
		t.Fatalf("unknown root field refused the package: %v", err)
	}
	declaration := release.Declaration()
	want := []plugin.Diagnostic{{Component: plugin.Component{Kind: plugin.ComponentManifestField, Name: "homepage-extra"}, Code: plugin.DiagnosticUnknownField}}
	if !reflect.DeepEqual(declaration.Diagnostics, want) {
		t.Fatalf("diagnostics = %+v, want %+v", declaration.Diagnostics, want)
	}
	if declaration.Name != "test.content" || len(declaration.Servers) != 1 || len(declaration.Skills) != 1 || len(declaration.Themes) != 1 {
		t.Fatalf("an unknown root field withdrew independent components: %+v", declaration)
	}
}

type passthroughDependencies struct{}

func (passthroughDependencies) ChangeInstallation(_ context.Context, _ resourceid.InstallationID, _ plugins.ChangeAdmission, change func([]plugin.Dependency) error) error {
	return change(nil)
}

func (passthroughDependencies) UnderAdmission(_ context.Context, inspect func([]plugin.Dependency) error) error {
	return inspect(nil)
}

type recordedConnections struct{ reconciled [][]mcpserver.ID }

func (c *recordedConnections) WithdrawInstallation([]mcpserver.ID) error { return nil }

func (c *recordedConnections) ReconcileInstallation(_ context.Context, names []mcpserver.ID) error {
	c.reconciled = append(c.reconciled, names)
	return nil
}

func TestReleaseUpdateRetainsPluginDataWithoutImplicitMigration(t *testing.T) {
	releases, installations, _ := testReleaseStore(t)
	files := map[string]string{
		"plugin.json": portableManifest,
		"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"sh","args":["${PLUGIN_ROOT}/backend.sh"],"cwd":"${PLUGIN_DATA}/work"}}}`,
		"backend.sh":  `printf started > "$PLUGIN_DATA/state"`,
	}
	source := writePackage(t, files)
	connections := &recordedConnections{}
	coordinator, err := plugins.New(t.Context(), installations, releases.catalog, releases, connections, passthroughDependencies{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := coordinator.Install(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	id := installed.View.ID
	if _, err := coordinator.Approve(t.Context(), id, installed.Selected.Digest()); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Enable(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	data := releases.dataRoot(id)
	if err := os.WriteFile(filepath.Join(data, "state"), []byte("private v1 state"), 0600); err != nil {
		t.Fatalf("enabled installation has no prepared data directory: %v", err)
	}
	before := snapshotTree(t, data)

	if err := os.WriteFile(filepath.Join(source, "backend.sh"), []byte(`printf started-v2 > "$PLUGIN_DATA/state"`), 0600); err != nil {
		t.Fatal(err)
	}
	staged, err := coordinator.Stage(t.Context(), id, source)
	if err != nil || staged.Staged == nil {
		t.Fatalf("stage = %+v, %v", staged, err)
	}
	selected, err := coordinator.Select(t.Context(), id, staged.Staged.Digest())
	if err != nil {
		t.Fatal(err)
	}
	if selected.Selected.Digest() == installed.Selected.Digest() || selected.View.State != plugin.Unapproved {
		t.Fatalf("update did not return the installation on the new release to review: %+v", selected.View)
	}
	if after := snapshotTree(t, data); !reflect.DeepEqual(after, before) {
		t.Fatalf("release update changed private plugin data: before %v, after %v", before, after)
	}
	current, err := installations.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	servers, err := realizedServers(t.Context(), releases, current.Installation, selected.Selected)
	if err != nil || len(servers) != 1 {
		t.Fatalf("descriptors = %+v, %v", servers, err)
	}
	if servers[0].Env["PLUGIN_DATA"] != data || servers[0].Dir != filepath.Join(data, "work") {
		t.Fatalf("updated release resolves another data directory: env %q, dir %q, want %q", servers[0].Env["PLUGIN_DATA"], servers[0].Dir, data)
	}
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			tree[relative] = "dir"
			return nil
		}
		content, err := os.ReadFile(path)
		tree[relative] = string(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

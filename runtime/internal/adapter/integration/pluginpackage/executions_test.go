package pluginpackage

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func executionFixture(t *testing.T) (*Releases, *plugin.Installation, plugin.Release) {
	t.Helper()
	r := testReleases(t)
	source := writePackage(t, map[string]string{
		"plugin.json": portableManifest,
		"mcp.json":    `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"sh","args":["${PLUGIN_ROOT}/backend.sh"]}}}`,
		"backend.sh":  "original",
	})
	release, err := publishPackage(t.Context(), r, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	return r, installation, release
}

func TestExecutionContentLivesUntilItsLastConnectionRetires(t *testing.T) {
	r, installation, release := executionFixture(t)
	first, found, err := r.Connection(t.Context(), installation, release, testsupport.ServerName("backend"))
	if err != nil || !found || first.Retire == nil {
		t.Fatalf("first launch: %v, %v", found, err)
	}
	t.Cleanup(func() { _ = first.Retire.Close() })
	second, found, err := r.Connection(t.Context(), installation, release, testsupport.ServerName("backend"))
	if err != nil || !found || second.Retire == nil {
		t.Fatalf("second launch: %v, %v", found, err)
	}
	t.Cleanup(func() { _ = second.Retire.Close() })
	root := first.Stdio.Env[plugin.RootVariable]
	if root != second.Stdio.Env[plugin.RootVariable] {
		t.Fatal("connections of one digest did not share execution content")
	}
	other, err := New(r.directory, r.catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.ReclaimExecutions(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := r.Reclaim(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.catalog.Get(t.Context(), release.Digest()); !errors.Is(err, plugin.ErrNotFound) {
		t.Fatalf("release was not reclaimed: %v", err)
	}
	if err := first.Retire.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Retire.Close(); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(root, "backend.sh")); err != nil || string(content) != "original" {
		t.Fatalf("release reclamation or peer retirement invalidated live execution: %q, %v", content, err)
	}
	if err := second.Retire.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("last retirement retained execution content: %v", err)
	}
}

func TestExecutionCopyVerifiesTheBytesItWillLaunch(t *testing.T) {
	r, _, release := executionFixture(t)
	source, err := r.currentRoot(t.Context(), release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	path := filepath.Join(source.Name(), "backend.sh")
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if lease, err := r.execution(t.Context(), release.Digest(), source); err == nil {
		_ = lease.Close()
		t.Fatal("a source proof vouched for subsequently changed execution bytes")
	}
	entries, err := os.ReadDir(r.executionDirectory())
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed copy retained private execution content: %d, %v", len(entries), err)
	}
}

func TestStartupRemovesAbandonedExecutionContent(t *testing.T) {
	r, _, _ := executionFixture(t)
	if err := os.MkdirAll(r.executionDirectory(), 0700); err != nil {
		t.Fatal(err)
	}
	tree, err := newStaging(r.executionDirectory())
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.seal(); err != nil {
		t.Fatal(err)
	}
	if err := tree.root.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.ReclaimExecutions(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tree.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("startup retained abandoned execution content: %v", err)
	}
}

func TestConcurrentConnectionsShareOwnedExecutionContent(t *testing.T) {
	r, installation, release := executionFixture(t)
	const consumers = 8
	start := make(chan struct{})
	results := make(chan mcpapp.Launch, consumers)
	failures := make(chan error, consumers)
	var calls sync.WaitGroup
	for range consumers {
		calls.Go(func() {
			<-start
			launch, _, err := r.Connection(t.Context(), installation, release, testsupport.ServerName("backend"))
			if err != nil {
				failures <- err
				return
			}
			results <- launch
		})
	}
	close(start)
	calls.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	var roots []string
	for launch := range results {
		roots = append(roots, launch.Stdio.Env[plugin.RootVariable])
		if err := launch.Retire.Close(); err != nil {
			t.Error(err)
		}
	}
	if len(roots) != consumers {
		t.Fatalf("admitted %d of %d connections", len(roots), consumers)
	}
	for _, root := range roots {
		if root != roots[0] {
			t.Fatal("concurrent connections created competing content projections")
		}
		if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("all connections retired but execution content remained: %v", err)
		}
	}
}

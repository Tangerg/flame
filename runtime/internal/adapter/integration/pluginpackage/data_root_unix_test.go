//go:build unix

package pluginpackage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestDataRootCapturesPhysicalParentWithoutFollowingNamespaceRedirection(t *testing.T) {
	original := t.TempDir()
	replacement := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(original, alias); err != nil {
		t.Fatal(err)
	}
	base := testReleases(t)
	releases, err := New(base.content, filepath.Join(alias, "data"), base.catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(original, "data")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("construction prepared private data: %v", err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(replacement, alias); err != nil {
		t.Fatal(err)
	}
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "mcp.json": `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"./bin/backend"}}}`, "bin/backend": "native fixture"})
	release, err := publishPackage(t.Context(), releases, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	launch, found, err := releases.Connection(t.Context(), installation, release, testsupport.ServerName("backend"))
	if err != nil || !found {
		t.Fatalf("original data authority: %v, %v", found, err)
	}
	defer launch.Retire.Close()
	physical, err := filesystem.CanonicalPath(original)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := launch.Stdio.Env[plugin.DataVariable], filepath.Join(physical, "data", installation.ID().String()); got != want {
		t.Fatalf("data authority = %q, want original %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(replacement, "data")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("parent alias retargeted preparation: %v", err)
	}
	unsafeParent := t.TempDir()
	if err := os.Symlink(replacement, filepath.Join(unsafeParent, "data")); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "/"} {
		redirected, err := New(base.content, filepath.Join(unsafeParent, "data")+suffix, base.catalog)
		if err != nil {
			t.Fatal(err)
		}
		if unsafe, found, err := redirected.Connection(t.Context(), installation, release, testsupport.ServerName("backend")); err == nil || found {
			if unsafe.Retire != nil {
				_ = unsafe.Retire.Close()
			}
			t.Fatal("data namespace redirection granted connection authority")
		}
	}
}

func TestDataRootPreservesPhysicalParentTraversal(t *testing.T) {
	base := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "mcp.json": `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"./bin/backend"}}}`, "bin/backend": "native fixture"})
	release, err := publishPackage(t.Context(), base, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		parent string
		suffix string
	}{
		{name: "symlink parent", parent: "/alias/.."},
		{name: "missing component before symlink", parent: "/missing/../alias/.."},
		{name: "trailing separator", parent: "/alias/..", suffix: "/"},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := t.TempDir()
			nested := filepath.Join(original, "nested")
			if err := os.Mkdir(nested, 0700); err != nil {
				t.Fatal(err)
			}
			lexical := t.TempDir()
			if err := os.Symlink(nested, filepath.Join(lexical, "alias")); err != nil {
				t.Fatal(err)
			}
			releases, err := New(base.content, lexical+test.parent+"/data"+test.suffix, base.catalog)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(original, "data")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("construction prepared private data: %v", err)
			}
			installation, err := plugin.New(testsupport.InstallationID(t), source, release)
			if err != nil {
				t.Fatal(err)
			}
			if err := installation.Enable(release); err != nil {
				t.Fatal(err)
			}
			launch, found, err := releases.Connection(t.Context(), installation, release, testsupport.ServerName("backend"))
			if err != nil || !found {
				t.Fatalf("physical data authority: %v, %v", found, err)
			}
			defer launch.Retire.Close()
			physical, err := filesystem.CanonicalPath(original)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := launch.Stdio.Env[plugin.DataVariable], filepath.Join(physical, "data", installation.ID().String()); got != want {
				t.Fatalf("data authority = %q, want physical %q", got, want)
			}
			if _, err := os.Stat(filepath.Join(lexical, "data")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("lexical parent received private data: %v", err)
			}
		})
	}
}

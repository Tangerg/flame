package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/pluginhost"
)

func TestFirstPartyInitializationResumesOriginalInstallationsAfterCatalogFailure(t *testing.T) {
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: testsupport.TempDirectory(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stores.Close(); err != nil {
			t.Error(err)
		}
	})
	lifetime := newRuntimeLifetime(context.Background(), nil)
	t.Cleanup(func() {
		if err := closeRuntimeLifetime(lifetime); err != nil {
			t.Error(err)
		}
	})
	fixture := pluginhost.New(t, stores)
	packages, err := buildPluginPackages(assemblyConfig{stores: stores}, lifetime)
	if err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(testsupport.TempDirectory(t), "catalog")
	if err := os.CopyFS(catalog, os.DirFS(firstPartyPackages)); err != nil {
		t.Fatal(err)
	}
	invalid := filepath.Join(catalog, "zz-invalid")
	if err := os.Mkdir(invalid, 0700); err != nil {
		t.Fatal(err)
	}
	if err := initializeFirstPartyPlugins(t.Context(), stores, fixture.Host, packages, catalog); err == nil {
		t.Fatal("invalid distribution appeared initialized")
	}
	initialized, err := stores.Installations.Initialized(t.Context())
	if err != nil || initialized {
		t.Fatalf("partial catalog marked initialized: %t, %v", initialized, err)
	}
	original, err := stores.Installations.List(t.Context())
	if err != nil || len(original) == 0 {
		t.Fatalf("original settled installations were lost: %d, %v", len(original), err)
	}
	if err := os.Remove(invalid); err != nil {
		t.Fatal(err)
	}
	if err := initializeFirstPartyPlugins(t.Context(), stores, fixture.Host, packages, catalog); err != nil {
		t.Fatal(err)
	}
	sources, err := os.ReadDir(catalog)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := stores.Installations.List(t.Context())
	if err != nil || len(rows) != len(sources) {
		t.Fatalf("resumed catalog installed %d of %d packages: %v", len(rows), len(sources), err)
	}
	for _, previous := range original {
		current, found, err := stores.Installations.Get(t.Context(), previous.Installation.ID())
		if err != nil || !found || current.Selected.Digest() != previous.Selected.Digest() {
			t.Fatalf("retry replaced an original installation: %v", err)
		}
	}
	if err := initializeFirstPartyPlugins(t.Context(), stores, fixture.Host, packages, filepath.Join(catalog, "absent")); err != nil {
		t.Fatalf("initialized restart reread distribution: %v", err)
	}
}

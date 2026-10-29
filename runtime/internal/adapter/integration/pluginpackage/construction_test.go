package pluginpackage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/distribution"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

func TestReleaseStoreRejectsMissingCatalogBeforeCreatingDirectory(t *testing.T) {
	stores, err := persistencefixture.Open(t, t.Context(), filepath.Join(t.TempDir(), "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stores.Close(); err != nil {
			t.Error(err)
		}
	})
	content, err := distribution.Open(distribution.Config{
		Directory: filepath.Join(t.TempDir(), "content"), Admission: stores.Admission,
		Trust:    func(context.Context, distribution.Source, distribution.Manifest, distribution.Lock) error { return nil },
		Retained: func(context.Context, qimu.Digest) (bool, error) { return false, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := content.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if releases, err := New(content, filepath.Join(t.TempDir(), "valid-data"), stores.PluginReleases); err != nil || releases == nil {
		t.Fatalf("valid release store fixture rejected: %v, %v", releases, err)
	}
	for _, test := range []struct {
		name    string
		catalog releaseCatalog
	}{
		{"nil", nil}, {"typed nil", (*persistence.ReleaseStore)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "releases")
			releases, err := New(content, directory, test.catalog)
			if err == nil || releases != nil {
				t.Fatalf("missing catalog constructed a release store: %v, %v", releases, err)
			}
			if _, err := os.Stat(directory); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("invalid construction created a release directory: %v", err)
			}
		})
	}
}

func TestPackageSkillsRejectsMissingCollaborators(t *testing.T) {
	releases, store, _ := testReleaseStore(t)
	for _, test := range []struct {
		name          string
		releases      *Releases
		installations skillInstallations
	}{
		{"releases", nil, store},
		{"installations", releases, nil},
		{"typed nil installations", releases, (*persistence.InstallationStore)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			skills, err := NewSkills(test.releases, test.installations)
			if err == nil || skills != nil {
				t.Fatalf("invalid package Skills constructed: %v, %v", skills, err)
			}
		})
	}
}

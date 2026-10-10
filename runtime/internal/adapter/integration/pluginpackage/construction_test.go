package pluginpackage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestReleaseStoreRejectsMissingCatalogBeforeCreatingDirectory(t *testing.T) {
	for _, catalog := range []releaseCatalog{nil, (*sqlite.ReleaseStore)(nil)} {
		directory := filepath.Join(t.TempDir(), "releases")
		releases, err := New(directory, catalog)
		if err == nil || releases != nil {
			t.Fatalf("invalid release store constructed: %v, %v", releases, err)
		}
		if _, err := os.Stat(directory); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("invalid construction created a release directory: %v", err)
		}
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
		{"typed nil installations", releases, (*sqlite.InstallationStore)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			skills, err := NewSkills(test.releases, test.installations)
			if err == nil || skills != nil {
				t.Fatalf("invalid package Skills constructed: %v, %v", skills, err)
			}
		})
	}
}

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestFirstPartyInitializationFailureCannotCommitPartialDefaults(t *testing.T) {
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: testsupport.TempDirectory(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stores.Close(); err != nil {
			t.Error(err)
		}
	})
	catalog := testsupport.TempDirectory(t)
	if err := os.CopyFS(filepath.Join(catalog, "a-valid"), os.DirFS(filepath.Join(firstPartyPackages, "usage"))); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(catalog, "z-invalid"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := initializeFirstPartyPlugins(t.Context(), stores, catalog); err == nil {
		t.Fatal("invalid distribution appeared initialized")
	}
	rows, err := stores.Installations.List(t.Context())
	if err != nil || len(rows) != 0 {
		t.Fatalf("partial default catalog committed: (%v, %v)", rows, err)
	}
	if err := initializeFirstPartyPlugins(t.Context(), stores, firstPartyPackages); err != nil {
		t.Fatal(err)
	}
	rows, err = stores.Installations.List(t.Context())
	if err != nil || len(rows) != 10 {
		t.Fatalf("valid initialization after failure = (%v, %v)", rows, err)
	}
	if err := initializeFirstPartyPlugins(t.Context(), stores, filepath.Join(catalog, "absent")); err != nil {
		t.Fatalf("restart reread the distribution: %v", err)
	}
}

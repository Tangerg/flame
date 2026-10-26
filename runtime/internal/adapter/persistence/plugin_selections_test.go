package persistence

import (
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestProviderSelectionSurvivesRestartWithoutAnInstallationOrImplicitReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	store := newProviderSelectionStore(db)
	selection, err := plugin.NewProviderSelection(plugin.SelectionRecord{Role: "consumer.role", Installation: testsupport.InstallationID(t), Contribution: plugin.ContributionID("provider")})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), selection); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store = newProviderSelectionStore(db)
	actual, found, err := store.Get(t.Context(), "consumer.role")
	if err != nil || !found || actual != selection {
		t.Fatalf("original choice changed: %+v, %t, %v", actual, found, err)
	}
	if _, found, err := store.Get(t.Context(), "another.role"); err != nil || found {
		t.Fatalf("missing role selected a provider: %t, %v", found, err)
	}
	if err := store.Save(t.Context(), plugin.ProviderSelection{}); err == nil {
		t.Fatal("invalid choice was persisted")
	}
	values, err := store.List(t.Context())
	if err != nil || len(values) != 1 || values[0] != selection {
		t.Fatalf("invalid update changed configuration: %+v, %v", values, err)
	}
}

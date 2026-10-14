package sqlite_test

import (
	"encoding/json/jsontext"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestCompositionSurvivesRestartInOriginalOrderWithoutInstallationLookup(t *testing.T) {
	contract, err := capability.New(capability.Config{Reference: "model-observer", Composition: capability.Decoration, Scope: "call"}, capability.MethodDescriptor{Name: "observe", Kind: capability.Query, Target: "call", Input: jsontext.Value(`{"type":"object"}`), Output: jsontext.Value(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	value, err := plugin.NewOrderedComposition(plugin.CompositionRecord{Contract: contract.Identity(), Members: []plugin.CompositionMember{{Installation: testsupport.InstallationID(t), Contribution: "outer"}, {Installation: testsupport.InstallationID(t), Contribution: "inner"}}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewCompositionStore(db).Save(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewCompositionStore(db)
	actual, found, err := store.Get(t.Context(), "model-observer")
	if err != nil || !found || !reflect.DeepEqual(actual.Snapshot(), value.Snapshot()) {
		t.Fatalf("persisted order changed: %+v, %t, %v", actual.Snapshot(), found, err)
	}
	if err := store.Save(t.Context(), plugin.OrderedComposition{}); err == nil {
		t.Fatal("invalid composition persisted")
	}
	values, err := store.List(t.Context())
	if err != nil || len(values) != 1 || !reflect.DeepEqual(values[0].Snapshot(), value.Snapshot()) {
		t.Fatalf("failed update changed original order: %+v, %v", values, err)
	}
}

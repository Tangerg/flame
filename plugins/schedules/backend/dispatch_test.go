package main

import (
	"path/filepath"
	"testing"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"

	"github.com/Tangerg/flame/plugins/schedules/internal/sqlite"
)

func TestCapturedRuntimeTargetSurvivesRestartAndRefusesAnotherStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := pluginstorage.BindRuntimeTarget(t.Context(), db, "original"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := pluginstorage.BindRuntimeTarget(t.Context(), db, "original"); err != nil {
		t.Fatal(err)
	}
	if err := pluginstorage.BindRuntimeTarget(t.Context(), db, "another"); err == nil {
		t.Fatal("captured target was replaced")
	}
	var original string
	if err := scanDatabase(t.Context(), db, `SELECT namespace FROM runtime_target WHERE id=1`, nil, &original); err != nil || original != "original" {
		t.Fatalf("original target=%q error=%v", original, err)
	}
}

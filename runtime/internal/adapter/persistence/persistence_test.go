package persistence

import (
	"os"
	"path/filepath"
	"testing"

	sqlitestore "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestOpenRequiresAndUsesExplicitProcessPaths(t *testing.T) {
	if _, err := Open(t.Context(), Config{}); err == nil {
		t.Fatal("Open accepted an empty data directory")
	}
	if _, err := Open(t.Context(), Config{DataDirectory: "relative-data"}); err == nil {
		t.Fatal("Open accepted a relative data directory")
	}

	dataDirectory := filepath.Join(t.TempDir(), "data")
	bundle, err := Open(t.Context(), Config{DataDirectory: dataDirectory})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = bundle.Close() })
	if bundle.DataDirectory != dataDirectory {
		t.Fatalf("DataDirectory = %q, want %q", bundle.DataDirectory, dataDirectory)
	}
	if bundle.IdempotencyNamespace.String() == "" {
		t.Fatal("Open returned an empty idempotency namespace")
	}
	if _, statErr := os.Stat(filepath.Join(dataDirectory, "flame.db")); statErr != nil {
		t.Fatalf("data directory does not own flame.db: %v", statErr)
	}
}

func TestBundleCloseIsIdempotent(t *testing.T) {
	db, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := &Bundle{db: db}
	if err := bundle.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := bundle.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := db.Ping(); err == nil {
		t.Fatal("database remained usable after Bundle.Close")
	}
}

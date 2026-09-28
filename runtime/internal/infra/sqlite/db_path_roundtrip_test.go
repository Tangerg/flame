package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenPreservesLiteralDatabasePath(t *testing.T) {
	names := []string{"space name", "percent%25", "percent%2f"}
	if runtime.GOOS != "windows" {
		names = append(names, "query?name", "fragment#name")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name, "flame.db")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			db, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			var sequence int
			var schema, actual string
			if err := db.QueryRowContext(ctx, "PRAGMA database_list").Scan(&sequence, &schema, &actual); err != nil {
				t.Fatal(err)
			}
			wantInfo, err := os.Stat(path)
			if err != nil {
				t.Fatalf("requested database was not created: %v", err)
			}
			actualInfo, err := os.Stat(actual)
			if err != nil || !os.SameFile(wantInfo, actualInfo) {
				t.Fatalf("database path = %q, want physical file %q: %v", actual, path, err)
			}
			if _, err := db.ExecContext(ctx, "CREATE TABLE path_roundtrip (value TEXT); INSERT INTO path_roundtrip VALUES ('retained')"); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			var value string
			if err := reopened.QueryRowContext(ctx, "SELECT value FROM path_roundtrip").Scan(&value); err != nil || value != "retained" {
				t.Fatalf("reopened value = %q: %v", value, err)
			}
		})
	}
}

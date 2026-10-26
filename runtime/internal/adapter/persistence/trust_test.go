package persistence

import (
	"context"
	"path/filepath"
	"testing"
)

func TestTrustStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := NewTrustStore(db)

	const root = "/home/me/project"

	// Default: untrusted.
	if ok, _ := s.IsTrusted(ctx, root); ok {
		t.Fatal("a never-trusted project should not be trusted")
	}

	// Trust → trusted; idempotent.
	if err := s.Trust(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err := s.Trust(ctx, root); err != nil {
		t.Fatalf("re-trust should be idempotent: %v", err)
	}
	if ok, _ := s.IsTrusted(ctx, root); !ok {
		t.Fatal("project should be trusted after Trust")
	}

	// Untrust → gone.
	if err := s.Untrust(ctx, root); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.IsTrusted(ctx, root); ok {
		t.Fatal("project should be untrusted after Untrust")
	}
}

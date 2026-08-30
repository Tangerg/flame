package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/provider"
)

func TestProviderStoreUpdatePreservesOmittedFieldsAndClearsExplicitly(t *testing.T) {
	db, err := Open(t.Context(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewProviderStore(db)

	oldKey, _ := provider.NewAPIKey("sk-old")
	oldBaseURL, _ := provider.NewBaseURL("https://old.test")
	got, err := store.Update(t.Context(), "openai", provider.Patch{
		APIKey: provider.Set(oldKey), BaseURL: provider.Set(oldBaseURL),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertStoredProvider(t, got, "sk-old", "https://old.test")

	newKey, _ := provider.NewAPIKey("sk-new")
	got, err = store.Update(t.Context(), "openai", provider.Patch{APIKey: provider.Set(newKey)})
	if err != nil {
		t.Fatal(err)
	}
	assertStoredProvider(t, got, "sk-new", "https://old.test")

	got, err = store.Update(t.Context(), "openai", provider.Patch{BaseURL: provider.Clear[provider.BaseURL]()})
	if err != nil {
		t.Fatal(err)
	}
	assertStoredProvider(t, got, "sk-new", "")
}

func TestProviderStoreUsesNullForAbsentConfiguration(t *testing.T) {
	db, err := Open(t.Context(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewProviderStore(db)
	if _, err := store.Update(t.Context(), "openai", provider.Patch{}); err != nil {
		t.Fatal(err)
	}
	var key, baseURL sql.NullString
	if err := db.QueryRowContext(t.Context(), `SELECT api_key, base_url FROM providers WHERE id = ?`, "openai").Scan(&key, &baseURL); err != nil {
		t.Fatal(err)
	}
	if key.Valid || baseURL.Valid {
		t.Fatalf("absent configuration encoded as key=%+v baseURL=%+v", key, baseURL)
	}
}

func assertStoredProvider(t *testing.T, entry provider.Provider, wantKey, wantBaseURL string) {
	t.Helper()
	key, configured := entry.APIKey()
	if wantKey == "" {
		if configured {
			t.Fatalf("credential = %q, want absent", key.Reveal())
		}
	} else if !configured || key.Reveal() != wantKey {
		t.Fatalf("credential = (%q, %v), want %q", key.Reveal(), configured, wantKey)
	}
	baseURL, present := entry.BaseURL()
	if wantBaseURL == "" {
		if present {
			t.Fatalf("base URL = %q, want absent", baseURL.String())
		}
	} else if !present || baseURL.String() != wantBaseURL {
		t.Fatalf("base URL = (%q, %v), want %q", baseURL.String(), present, wantBaseURL)
	}
}

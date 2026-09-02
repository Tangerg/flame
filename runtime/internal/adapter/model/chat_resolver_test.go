package model

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	sqlitestore "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// TestChatResolverRejectsUnconfigured verifies an explicit provider that
// has no key errors out (the "configure it first" path); once configured it
// resolves a client from the current registry snapshot. The provider is taken
// as given — never inferred from the model.
func TestChatResolverRejectsUnconfigured(t *testing.T) {
	db, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	ps := sqlitestore.NewProviderStore(db) // empty: deepseek not configured
	r, err := NewChatResolver(ps)
	if err != nil {
		t.Fatal(err)
	}

	_, err = r.ResolveChat(t.Context(), testDeepSeekSelection(t, "deepseek-v4-pro"))
	if err == nil {
		t.Fatal("expected an error resolving against an unconfigured provider")
	}
	var failure *run.FailureError
	if !errors.As(err, &failure) || failure.Kind != run.FailureInvalidCredentials {
		t.Fatalf("unconfigured provider error = %#v, want invalid-credentials failure", err)
	}

	apiKey, _ := provider.NewAPIKey("k")
	_, err = ps.Update(t.Context(), "deepseek", provider.Patch{APIKey: provider.Set(apiKey)})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := r.ResolveChat(t.Context(), testDeepSeekSelection(t, "deepseek-v4-pro"))
	if err != nil || resolved.Client() == nil {
		t.Fatalf("ResolveChat after configure: client=%v err=%v", resolved.Client(), err)
	}
	// Client construction is cheap and immutable. Re-resolving deliberately
	// avoids a process-lifetime cache retaining old credential generations.
	if c2, _ := r.ResolveChat(t.Context(), testDeepSeekSelection(t, "deepseek-v4-pro")); c2.Client() == resolved.Client() {
		t.Error("resolver retained a process-lifetime client")
	}
	// A different model on the same provider builds a distinct client.
	if c3, _ := r.ResolveChat(t.Context(), testDeepSeekSelection(t, "deepseek-v4-flash")); c3.Client() == resolved.Client() {
		t.Error("different model should resolve a distinct client")
	}
}

func TestChatResolverBuildsOptionalCredentialProviderWithoutRegistryRow(t *testing.T) {
	db, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	selection, err := modelref.New("ollama", "local-model")
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewChatResolver(sqlitestore.NewProviderStore(db))
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolver.ResolveChat(t.Context(), selection)
	if err != nil || resolved.Client() == nil {
		t.Fatalf("ResolveChat optional credential provider = %v, %v", resolved.Client(), err)
	}
}

func testDeepSeekSelection(t testing.TB, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New("deepseek", model)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

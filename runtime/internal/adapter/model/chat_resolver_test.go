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
	r := NewChatResolver(ps)

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
	c, err := r.ResolveChat(t.Context(), testDeepSeekSelection(t, "deepseek-v4-pro"))
	if err != nil || c == nil {
		t.Fatalf("ResolveChat after configure: client=%v err=%v", c, err)
	}
	// Client construction is cheap and immutable. Re-resolving deliberately
	// avoids a process-lifetime cache retaining old credential generations.
	if c2, _ := r.ResolveChat(t.Context(), testDeepSeekSelection(t, "deepseek-v4-pro")); c2 == c {
		t.Error("resolver retained a process-lifetime client")
	}
	// A different model on the same provider builds a distinct client.
	if c3, _ := r.ResolveChat(t.Context(), testDeepSeekSelection(t, "deepseek-v4-flash")); c3 == c {
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
	client, err := NewChatResolver(sqlitestore.NewProviderStore(db)).ResolveChat(t.Context(), selection)
	if err != nil || client == nil {
		t.Fatalf("ResolveChat optional credential provider = %v, %v", client, err)
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

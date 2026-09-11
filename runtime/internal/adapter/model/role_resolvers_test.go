package model

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chat"
)

type recordingChatResolver struct {
	resolve func(modelref.Selection) (ResolvedChat, error)
}

func (r recordingChatResolver) ResolveChat(
	_ context.Context,
	selection modelref.Selection,
) (ResolvedChat, error) {
	return r.resolve(selection)
}

type staticRoleSource struct {
	role modelref.Role
}

func (s staticRoleSource) Role() modelref.Role { return s.role }

type pointerInputTokenCounter struct{}

func (*pointerInputTokenCounter) CountInputTokens(
	context.Context,
	*chat.Request,
) (int64, error) {
	return 1, nil
}

func TestResolvedChatRejectsTypedNilInputTokenCounter(t *testing.T) {
	model := newTestChatModel(t)
	var counter *pointerInputTokenCounter

	resolved, err := NewResolvedChat(model, counter)

	if err == nil || !dependency.Missing(resolved.Model()) {
		t.Fatalf("NewResolvedChat typed-nil counter = (%v, %v), want invalid construction", resolved, err)
	}
}

func TestLiveUtilityClientResolvesMainForEveryUse(t *testing.T) {
	selection := mustRoleSelection(t, "anthropic", "claude-test")
	model := newTestChatModel(t)
	calls := 0
	resolver := recordingChatResolver{resolve: func(got modelref.Selection) (ResolvedChat, error) {
		calls++
		if got != selection {
			t.Fatalf("selection = %#v, want %#v", got, selection)
		}
		return mustResolvedChat(t, model, nil), nil
	}}
	resolve, err := LiveUtilityClient(resolver, selection, staticRoleSource{})
	if err != nil {
		t.Fatal(err)
	}

	first, firstErr := resolve(t.Context())
	second, secondErr := resolve(t.Context())
	if firstErr != nil || secondErr != nil {
		t.Fatalf("resolve errors = (%v, %v)", firstErr, secondErr)
	}
	// A client is a projection of the resolved model, so each resolution builds
	// its own. What must hold is that both are usable and that neither was
	// served from a cache.
	if first == nil || second == nil {
		t.Fatalf("resolved clients = (%v, %v), want two usable clients", first, second)
	}
	if calls != 2 {
		t.Fatalf("resolver calls = %d, want 2 current-registry reads", calls)
	}
}

func TestLiveUtilityClientReturnsConfiguredRoleFailureWithoutFallback(t *testing.T) {
	mainSelection := mustRoleSelection(t, "anthropic", "claude-main")
	utilityRole := mustRole(t, "openai", "utility-model")
	utilitySelection := utilityRole.Selection()
	model := newTestChatModel(t)
	var resolved []modelref.Selection
	resolver := recordingChatResolver{resolve: func(selection modelref.Selection) (ResolvedChat, error) {
		resolved = append(resolved, selection)
		if selection.Equal(utilitySelection) {
			return ResolvedChat{}, errors.New("utility provider unavailable")
		}
		return mustResolvedChat(t, model, nil), nil
	}}
	resolve, err := LiveUtilityClient(resolver, mainSelection, staticRoleSource{role: utilityRole})
	if err != nil {
		t.Fatal(err)
	}

	got, err := resolve(t.Context())
	if err == nil || got != nil || !strings.Contains(err.Error(), "utility provider unavailable") {
		t.Fatalf("resolve configured utility = (%p, %v), want exact failure", got, err)
	}
	if len(resolved) != 1 || !resolved[0].Equal(utilitySelection) {
		t.Fatalf("resolved selections = %#v, want utility only", resolved)
	}
}

func mustRoleSelection(t testing.TB, providerID, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(providerID, model)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

func mustRole(t testing.TB, providerID, model string) modelref.Role {
	t.Helper()
	role, err := modelref.NewRole(providerID, model)
	if err != nil {
		t.Fatal(err)
	}
	return role
}

func newTestChatModel(t testing.TB) chat.Model {
	t.Helper()
	return chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("unused test model")
	})
}

func mustResolvedChat(t testing.TB, model chat.Model, counter InputTokenCounter) ResolvedChat {
	t.Helper()
	resolved, err := NewResolvedChat(model, counter)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

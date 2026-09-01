package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
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
	selection modelref.Selection
}

func (s staticRoleSource) Role() modelref.Selection { return s.selection }

func TestLiveUtilityClientResolvesMainForEveryUse(t *testing.T) {
	selection := mustRoleSelection(t, "anthropic", "claude-test")
	client := newTestChatClient(t)
	calls := 0
	resolver := recordingChatResolver{resolve: func(got modelref.Selection) (ResolvedChat, error) {
		calls++
		if got != selection {
			t.Fatalf("selection = %#v, want %#v", got, selection)
		}
		return mustResolvedChat(t, client, nil), nil
	}}
	resolve := LiveUtilityClient(resolver, selection, staticRoleSource{})

	first, firstErr := resolve(t.Context())
	second, secondErr := resolve(t.Context())
	if firstErr != nil || secondErr != nil {
		t.Fatalf("resolve errors = (%v, %v)", firstErr, secondErr)
	}
	if first != client || second != client {
		t.Fatalf("resolved clients = (%p, %p), want (%p, %p)", first, second, client, client)
	}
	if calls != 2 {
		t.Fatalf("resolver calls = %d, want 2 current-registry reads", calls)
	}
}

func TestLiveUtilityClientReturnsConfiguredRoleFailureWithoutFallback(t *testing.T) {
	mainSelection := mustRoleSelection(t, "anthropic", "claude-main")
	utilitySelection := mustRoleSelection(t, "openai", "utility-model")
	client := newTestChatClient(t)
	var resolved []modelref.Selection
	resolver := recordingChatResolver{resolve: func(selection modelref.Selection) (ResolvedChat, error) {
		resolved = append(resolved, selection)
		if selection == utilitySelection {
			return ResolvedChat{}, errors.New("utility provider unavailable")
		}
		return mustResolvedChat(t, client, nil), nil
	}}
	resolve := LiveUtilityClient(resolver, mainSelection, staticRoleSource{selection: utilitySelection})

	got, err := resolve(t.Context())
	if err == nil || got != nil || !strings.Contains(err.Error(), "utility provider unavailable") {
		t.Fatalf("resolve configured utility = (%p, %v), want exact failure", got, err)
	}
	if len(resolved) != 1 || resolved[0] != utilitySelection {
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

func newTestChatClient(t testing.TB) *chatclient.Client {
	t.Helper()
	client, err := chatclient.New(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("unused test model")
	}), chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return &client
}

func mustResolvedChat(t testing.TB, client *chatclient.Client, counter InputTokenCounter) ResolvedChat {
	t.Helper()
	resolved, err := NewResolvedChat(client, counter)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

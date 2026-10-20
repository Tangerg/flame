package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/dependency"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	modelchoice "github.com/Tangerg/flame/runtime/model"
	"github.com/Tangerg/scope/core/chat"
)

type recordingChatResolver struct {
	resolve func(modelchoice.Selection) (ResolvedChat, error)
}

func (r recordingChatResolver) ResolveChat(
	_ context.Context,
	selection modelchoice.Selection,
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

func TestLiveUtilityModelResolvesMainForEveryUse(t *testing.T) {
	selection := testsupport.MustModelSelection("anthropic", "claude-test")
	model := newTestChatModel(t)
	calls := 0
	resolver := recordingChatResolver{resolve: func(got modelchoice.Selection) (ResolvedChat, error) {
		calls++
		if got != selection {
			t.Fatalf("selection = %#v, want %#v", got, selection)
		}
		return mustResolvedChat(t, model, nil), nil
	}}
	resolve, err := LiveUtilityModel(resolver, selection, staticRoleSource{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	first, releaseFirst, firstErr := resolve(t.Context(), AuxiliaryBudget{MaxInputBytes: 1024, MaxOutputTokens: 64})
	second, releaseSecond, secondErr := resolve(t.Context(), AuxiliaryBudget{MaxInputBytes: 1024, MaxOutputTokens: 64})
	if firstErr != nil || secondErr != nil {
		t.Fatalf("resolve errors = (%v, %v)", firstErr, secondErr)
	}
	defer releaseFirst()
	defer releaseSecond()
	if first.Model() == nil || second.Model() == nil {
		t.Fatalf("resolved models = (%v, %v), want two usable models", first, second)
	}
	if first.Selection() != selection || second.Selection() != selection {
		t.Fatal("utility capture detached its exact selection")
	}
	if calls != 2 {
		t.Fatalf("resolver calls = %d, want 2 current-registry reads", calls)
	}
}

func TestLiveUtilityModelReturnsConfiguredRoleFailureWithoutFallback(t *testing.T) {
	mainSelection := testsupport.MustModelSelection("anthropic", "claude-main")
	utilityRole := mustRole(t, "openai", "utility-model")
	utilitySelection := utilityRole.Selection()
	model := newTestChatModel(t)
	var resolved []modelchoice.Selection
	resolver := recordingChatResolver{resolve: func(selection modelchoice.Selection) (ResolvedChat, error) {
		resolved = append(resolved, selection)
		if selection.Equal(utilitySelection) {
			return ResolvedChat{}, errors.New("utility provider unavailable")
		}
		return mustResolvedChat(t, model, nil), nil
	}}
	resolve, err := LiveUtilityModel(resolver, mainSelection, staticRoleSource{role: utilityRole}, nil)
	if err != nil {
		t.Fatal(err)
	}

	got, _, err := resolve(t.Context(), AuxiliaryBudget{MaxInputBytes: 1024, MaxOutputTokens: 64})
	if err == nil || got.Model() != nil || !strings.Contains(err.Error(), "utility provider unavailable") {
		t.Fatalf("resolve configured utility = (%+v, %v), want exact failure", got, err)
	}
	if len(resolved) != 1 || !resolved[0].Equal(utilitySelection) {
		t.Fatalf("resolved selections = %#v, want utility only", resolved)
	}
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

type utilitySelectorFunc func(context.Context, modelapp.SelectionInput) (modelchoice.Selection, error)

func (f utilitySelectorFunc) SelectModel(ctx context.Context, input modelapp.SelectionInput) (modelchoice.Selection, error) {
	return f(ctx, input)
}

func TestUtilitySelectionPrecedesExactProviderConstructionAndNeverFallsBack(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "selected", true: "refused"}[fail], func(t *testing.T) {
			proposed := testsupport.MustModelSelection("provider", "proposed-model")
			selected := testsupport.MustModelSelection("provider", "selected-model")
			model := newTestChatModel(t)
			var steps []string
			resolver := recordingChatResolver{resolve: func(selection modelchoice.Selection) (ResolvedChat, error) {
				steps = append(steps, "resolve")
				if selection != selected {
					t.Errorf("constructed original route after selection: %+v", selection)
				}
				return mustResolvedChat(t, model, nil), nil
			}}
			refusal := errors.New("selector unavailable")
			selector := utilitySelectorFunc(func(_ context.Context, input modelapp.SelectionInput) (modelchoice.Selection, error) {
				steps = append(steps, "select")
				if input.Proposed != proposed || input.Request != nil || input.Context.Use != modelchoice.Utility || input.Context.Limits == nil || *input.Context.Limits != (modelchoice.SelectionLimits{InputKind: modelchoice.TextBytes, MaxInputBytes: 1024, MaxOutputTokens: 64}) {
					t.Error("utility selection invented input or changed proposal")
				}
				if fail {
					return modelchoice.Selection{}, refusal
				}
				return selected, nil
			})
			utility, err := LiveUtilityModel(resolver, proposed, staticRoleSource{}, selector)
			if err != nil {
				t.Fatal(err)
			}
			captured, release, err := utility(t.Context(), AuxiliaryBudget{MaxInputBytes: 1024, MaxOutputTokens: 64})
			if fail {
				if !errors.Is(err, refusal) || len(steps) != 1 || release != nil || captured.Model() != nil {
					t.Fatalf("selector failure fell back: %+v %+v %v", steps, captured, err)
				}
			} else {
				if err != nil || captured.Selection() != selected || len(steps) != 2 || steps[0] != "select" || steps[1] != "resolve" {
					t.Fatalf("selected construction: %+v %+v %v", steps, captured, err)
				}
				release()
			}
		})
	}
}

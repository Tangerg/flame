package hooks

import (
	"context"
	"testing"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
)

func TestBoundOwnsRetainedHookConfiguration(t *testing.T) {
	rules := []hookdata.Hook{{Event: hookdata.SessionStart, Inject: "original context"}}
	bound := NewBound(rules, func(_ context.Context, event hookprovider.Evaluation) (hookprovider.Advice, error) {
		return hookprovider.Advice{Decision: hookdata.CommandDecision{Verdict: hookdata.CommandAllow, InjectContext: event.Rules[0].Inject}}, nil
	}, nil, nil)
	rules[0].Inject = "changed"
	if got := bound.Run(t.Context(), hookdata.Input{Event: hookdata.SessionStart}).InjectContext; got != "original context" {
		t.Fatalf("retained advice = %q", got)
	}
}

package agentexec

import (
	"errors"
	"testing"

	agent "github.com/Tangerg/scope/agent"
)

func TestUnknownEffectDiagnosticsPreserveFirstCauseAndIdentity(t *testing.T) {
	first, err := agent.ParseEffectID("effect:first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := agent.ParseEffectID("effect:second")
	if err != nil {
		t.Fatal(err)
	}
	var failures interactionEffectFailures
	failures.record(first, errors.New("sqlite: result transaction failed"))
	failures.record(first, errors.New("context canceled"))
	observed := failures.observations([]agent.EffectID{first, second})
	if observed[0].ID != first.String() || observed[0].Detail != "sqlite: result transaction failed" || observed[1].ID != second.String() || observed[1].Detail != "" {
		t.Fatalf("unknown evidence changed: %+v", observed)
	}
	observed[0].Detail = "mutated"
	if failures.observations([]agent.EffectID{first})[0].Detail != "sqlite: result transaction failed" {
		t.Fatal("observation changed owner")
	}
}

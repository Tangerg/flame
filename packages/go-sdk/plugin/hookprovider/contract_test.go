package hookprovider

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/hook"
)

func TestCommandOutcomeBoundsTheCompleteDecisionMaterial(t *testing.T) {
	decision := &hook.CommandDecision{Verdict: hook.CommandAllow, InjectContext: strings.Repeat("x", MaxCommandOutputBytes)}
	result := Result{Decision: decision}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	decision.Reason = "x"
	if result.Validate() == nil {
		t.Fatal("combined decision material exceeded the command output envelope")
	}
}

func TestCommandOutcomePreservesFailureWithoutInventingAnAllow(t *testing.T) {
	if (Result{}).Validate() == nil {
		t.Fatal("missing decision became success")
	}
	result := Result{ExitCode: -1, Failure: &Failure{Kind: TimedOut, Message: "deadline exceeded"}}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.Decision = &hook.CommandDecision{Verdict: "future"}
	if result.Validate() == nil {
		t.Fatal("unknown policy advice was admitted")
	}
}

package protocol

import (
	"errors"
	"strings"
	"testing"
	"time"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

func TestObservationCallIDsPreserveExecutorIdentityContract(t *testing.T) {
	values := []string{"", "model:root:19", "tool:root:1", "call~1", "调用", strings.Repeat("a", 256), strings.Repeat("a", 257)}
	for character := range 128 {
		values = append(values, "call"+string(rune(character)))
	}
	for _, value := range values {
		wantValid := runtimeidentity.ValidateEffect(value) == nil
		for _, observation := range []WireValidator{
			ModelInvocation{CallID: value, RunID: "run_1", SegmentID: "seg_1", State: ModelInvocationStarted, StartedAt: time.Unix(1, 0)},
			ToolAttempt{CallID: value, RunID: "run_1", SegmentID: "seg_1", ItemID: "item_1", State: ToolAttemptStarted, StartedAt: time.Unix(1, 0)},
		} {
			if err := observation.ValidateWire(); (err == nil) != wantValid {
				t.Errorf("%T callId %q: %v; executor accepts = %t", observation, value, err, wantValid)
			}
		}
	}
}

func TestPublicIdentityValidationUsesRuntimeSemantics(t *testing.T) {
	for name, validate := range map[string]func(string) error{
		"session": ValidateSessionID,
		"run":     ValidateRunID,
		"segment": ValidateSegmentID,
		"item":    ValidateItemID,
		"event":   ValidateRunEventID,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validate("opaque_一/2"); err != nil {
				t.Fatalf("valid identity: %v", err)
			}
			if err := validate("opaque identity"); err == nil {
				t.Fatal("identity containing whitespace was accepted")
			}
		})
	}

	if err := ValidateSessionID(strings.Repeat("界", MaximumResourceIdentityCharacters+1)); err == nil {
		t.Fatal("oversized resource identity was accepted")
	}
	if err := ValidateRunEventID(strings.Repeat("界", MaximumRunEventIDCharacters+1)); err == nil {
		t.Fatal("oversized event identity was accepted")
	}
}

func TestPublicModelIdentityValidationUsesRuntimeSemantics(t *testing.T) {
	if err := ValidateModelSelection("openai", "gpt-5", "high"); err != nil {
		t.Fatalf("valid model selection: %v", err)
	}
	if err := ValidateProviderIdentity("open ai"); err == nil {
		t.Fatal("provider identity containing whitespace was accepted")
	}
	if err := ValidateModelIdentity(""); err == nil {
		t.Fatal("empty model identity was accepted")
	}
	if err := ValidateReasoningEffortIdentity("very high"); err == nil {
		t.Fatal("reasoning effort containing whitespace was accepted")
	}
	if err := ValidateModelSelection("openai", "", ""); !errors.Is(err, ErrIncompleteModelSelection) {
		t.Fatalf("incomplete model selection error = %v", err)
	}
}

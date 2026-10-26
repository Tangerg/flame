package protocol

import (
	"strings"
	"testing"
	"time"

	runtimeidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
)

func TestObservationCallIDsPreserveExecutorIdentityContract(t *testing.T) {
	values := []string{"", "model:root:19", "tool:root:1", "call~1", "调用", strings.Repeat("a", 256), strings.Repeat("a", 257)}
	for character := range 128 {
		values = append(values, "call"+string(rune(character)))
	}
	for _, value := range values {
		wantValid := runtimeidentity.ValidateURISafeASCII(value, MaximumResourceIdentityCharacters) == nil
		for _, observation := range []wireValidator{
			ModelInvocation{CallID: value, RunID: "run_1", SegmentID: "seg_1", State: ModelInvocationStarted, StartedAt: time.Unix(1, 0)},
			ToolAttempt{CallID: value, RunID: "run_1", SegmentID: "seg_1", ItemID: "item_1", State: ToolAttemptStarted, StartedAt: time.Unix(1, 0)},
		} {
			if err := observation.validateWire(); (err == nil) != wantValid {
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

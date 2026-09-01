package protocol

import (
	"errors"
	"strings"
	"testing"
)

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

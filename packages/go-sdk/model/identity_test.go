package model

import (
	"errors"
	"testing"
)

func TestModelIdentityValidation(t *testing.T) {
	if err := ValidateSelection("openai", "gpt-5", "high"); err != nil {
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
	if err := ValidateSelection("openai", "", ""); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("incomplete model selection error = %v", err)
	}
}

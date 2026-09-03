package modelref

import (
	"errors"
	"testing"
)

func TestRoleOwnsProviderModelOnly(t *testing.T) {
	role, err := NewRole("openai", "gpt-5")
	if err != nil {
		t.Fatal(err)
	}
	if err := role.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !role.Configured() || role.Provider() != "openai" || role.Model() != "gpt-5" {
		t.Fatalf("role = %v", role)
	}
	if selection := role.Selection(); selection.Provider() != "openai" || selection.Model() != "gpt-5" || selection.ReasoningEffort() != "" {
		t.Fatalf("selection = %v", selection)
	}
	if err := (Role{}).Validate(); err != nil {
		t.Fatalf("zero role Validate() error = %v", err)
	}
}

func TestRoleRejectsExecutionOptions(t *testing.T) {
	selection, err := NewWithReasoningEffort("openai", "gpt-5", "high")
	if err != nil {
		t.Fatal(err)
	}
	if err := (Role{selection: selection}).Validate(); !errors.Is(err, errRoleReasoningEffort) {
		t.Fatalf("Validate() error = %v, want %v", err, errRoleReasoningEffort)
	}
}

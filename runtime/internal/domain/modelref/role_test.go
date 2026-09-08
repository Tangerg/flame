package modelref

import "testing"

func TestRoleOwnsProviderModelOnly(t *testing.T) {
	role, err := NewRole("openai", "gpt-5")
	if err != nil {
		t.Fatal(err)
	}
	if !role.Configured() || role.Provider() != "openai" || role.Model() != "gpt-5" {
		t.Fatalf("role = %v", role)
	}
	// A role stores the pair and nothing else: its constructor is the only
	// source, and it never carries the execution options a Selection may.
	if selection := role.Selection(); selection.Provider() != "openai" || selection.Model() != "gpt-5" || selection.ReasoningEffort() != "" {
		t.Fatalf("selection = %v", selection)
	}
	if zero := (Role{}); zero.Configured() || zero.Selection() != (Selection{}) {
		t.Fatalf("zero role = %v", zero)
	}
}

func TestNewRoleRejectsAnIncompletePair(t *testing.T) {
	if _, err := NewRole("openai", ""); err == nil {
		t.Fatal("NewRole accepted a provider without a model")
	}
	if _, err := NewRole("", "gpt-5"); err == nil {
		t.Fatal("NewRole accepted a model without a provider")
	}
}

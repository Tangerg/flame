package model

import (
	"errors"

	"testing"
)

func TestSelectionValidateExact(t *testing.T) {
	if err := (Selection{}).ValidateExact(); !errors.Is(err, errExactSelectionRequired) {
		t.Fatalf("zero selection ValidateExact() error = %v, want %v", err, errExactSelectionRequired)
	}
	selection, err := New("openai", "gpt-5")
	if err != nil {
		t.Fatal(err)
	}
	if err := selection.ValidateExact(); err != nil {
		t.Fatalf("exact selection ValidateExact() error = %v", err)
	}
}

func TestSelectionOwnsEqualityAndDiagnosticIdentity(t *testing.T) {
	selection, err := NewWithReasoningEffort("openai", "gpt-5", "high")
	if err != nil {
		t.Fatal(err)
	}
	same, err := NewWithReasoningEffort("openai", "gpt-5", "high")
	if err != nil {
		t.Fatal(err)
	}
	differentEffort, err := NewWithReasoningEffort("openai", "gpt-5", "low")
	if err != nil {
		t.Fatal(err)
	}
	if !selection.Equal(same) || selection.Equal(differentEffort) {
		t.Fatalf("Equal() did not compare the complete selection")
	}
	if got, want := selection.String(), "openai/gpt-5 (reasoning effort high)"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got, want := (Selection{}).String(), "<default>"; got != want {
		t.Fatalf("zero String() = %q, want %q", got, want)
	}
}

func TestPatchAppliesSelectionAtomically(t *testing.T) {
	current, err := NewWithReasoningEffort("openai", "gpt-5.6-sol", "high")
	if err != nil {
		t.Fatal(err)
	}
	medium := "medium"
	updated, err := (Patch{ReasoningEffort: &medium}).Apply(current)
	if err != nil || updated.Provider() != "openai" || updated.Model() != "gpt-5.6-sol" || updated.ReasoningEffort() != medium {
		t.Fatalf("effort-only patch = %+v, %v", updated, err)
	}

	provider, model := "anthropic", "claude-opus"
	changed, err := (Patch{Provider: &provider, Model: &model}).Apply(current)
	if err != nil || changed.Provider() != provider || changed.Model() != model || changed.ReasoningEffort() != "" {
		t.Fatalf("model patch = %+v, %v", changed, err)
	}

	if _, err := (Patch{Provider: &provider}).Apply(current); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("partial identity patch error = %v, want %v", err, ErrIncomplete)
	}
}

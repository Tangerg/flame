package modelref

import (
	"errors"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		wantSet  bool
		wantErr  error
	}{
		{name: "unset"},
		{name: "configured", provider: "openai", model: "gpt-5", wantSet: true},
		{name: "provider without model", provider: "openai", wantErr: ErrIncomplete},
		{name: "model without provider", model: "gpt-5", wantErr: ErrIncomplete},
		{name: "provider whitespace", provider: " openai", model: "gpt-5", wantErr: ErrProviderIdentity},
		{name: "model whitespace", provider: "openai", model: "gpt-5 ", wantErr: ErrModelIdentity},
		{name: "provider NUL", provider: "openai\x00shadow", model: "gpt-5", wantErr: ErrProviderIdentity},
		{name: "model newline", provider: "openai", model: "gpt-5\nshadow", wantErr: ErrModelIdentity},
		{name: "provider too long", provider: strings.Repeat("p", MaximumProviderIdentityCharacters+1), model: "gpt-5", wantErr: ErrProviderIdentity},
		{name: "model too long", provider: "openai", model: strings.Repeat("m", MaximumModelIdentityCharacters+1), wantErr: ErrModelIdentity},
		{name: "invalid UTF-8 provider", provider: string([]byte{0xff}), model: "gpt-5", wantErr: ErrProviderIdentity},
		{name: "zero-width model character", provider: "openai", model: "gpt-\u200b5", wantErr: ErrModelIdentity},
		{name: "unicode provider boundary", provider: strings.Repeat("模", MaximumProviderIdentityCharacters), model: "gpt-5", wantSet: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, err := New(tt.provider, tt.model)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("New() error = %v, want %v", err, tt.wantErr)
			}
			if selection.Configured() != tt.wantSet {
				t.Fatalf("Configured() = %t, want %t", selection.Configured(), tt.wantSet)
			}
			if selection.Provider() != tt.provider && err == nil {
				t.Fatalf("Provider() = %q, want %q", selection.Provider(), tt.provider)
			}
			if selection.Model() != tt.model && err == nil {
				t.Fatalf("Model() = %q, want %q", selection.Model(), tt.model)
			}
		})
	}
}

func TestNewWithReasoningEffort(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		effort   string
		wantErr  error
	}{
		{name: "provider default", provider: "openai", model: "gpt-5"},
		{name: "explicit", provider: "openai", model: "gpt-5", effort: "high"},
		{name: "effort without model", effort: "high", wantErr: ErrReasoningEffortWithoutModel},
		{name: "effort whitespace", provider: "openai", model: "gpt-5", effort: " high", wantErr: ErrReasoningEffortIdentity},
		{name: "effort control", provider: "openai", model: "gpt-5", effort: "high\t", wantErr: ErrReasoningEffortIdentity},
		{name: "effort too long", provider: "openai", model: "gpt-5", effort: strings.Repeat("e", MaximumReasoningEffortCharacters+1), wantErr: ErrReasoningEffortIdentity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, err := NewWithReasoningEffort(tt.provider, tt.model, tt.effort)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewWithReasoningEffort() error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && selection.ReasoningEffort() != tt.effort {
				t.Fatalf("ReasoningEffort() = %q, want %q", selection.ReasoningEffort(), tt.effort)
			}
		})
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

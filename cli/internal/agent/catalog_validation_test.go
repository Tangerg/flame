package agent

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func modelTokenPointer(value int64) *int64 { return &value }

func mustModelTokenLimits(t *testing.T, values ModelTokenLimitValues) ModelTokenLimits {
	t.Helper()
	limits, err := NewModelTokenLimits(values)
	if err != nil {
		t.Fatal(err)
	}
	return limits
}

func TestModelOwnsAndValidatesCompleteCatalogMetadata(t *testing.T) {
	t.Parallel()

	model := Model{
		ID: "reasoner", Provider: "provider", DisplayName: "Reasoner",
		TokenLimits: mustModelTokenLimits(t, ModelTokenLimitValues{
			ContextWindow: modelTokenPointer(200_000), MaxInputTokens: modelTokenPointer(180_000), MaxOutputTokens: modelTokenPointer(20_000),
		}),
		KnowledgeCutoff: "2026-01-31",
		Capabilities: &ModelCapabilities{
			Reasoning: true, ReasoningLevels: []string{"low", "high"}, ReasoningDefaultLevel: "high",
			Multimodal: true, InputModalities: []ModelModality{ModelModalityText, ModelModalityImage},
			OutputModalities: []ModelModality{ModelModalityText}, ToolUse: true, StructuredOutput: true,
		},
		Pricing: &ModelPricing{InputUSDPerMillionTokens: 0.2, OutputUSDPerMillionTokens: 0.8},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	cloned := model.Clone()
	cloned.Capabilities.ReasoningLevels[0] = "mutated"
	cloned.Capabilities.InputModalities[0] = ModelModalityAudio
	cloned.Pricing.InputUSDPerMillionTokens = 99
	if model.Capabilities.ReasoningLevels[0] != "low" ||
		model.Capabilities.InputModalities[0] != ModelModalityText ||
		model.Pricing.InputUSDPerMillionTokens != 0.2 {
		t.Fatal("model clone shares mutable catalog metadata")
	}
}

func TestModelTokenLimitsRejectNumericAbsenceAndImpossibleFacts(t *testing.T) {
	t.Parallel()

	if !(ModelTokenLimits{}).Unknown() {
		t.Fatal("zero value must mean explicitly unknown facts")
	}
	for _, values := range []ModelTokenLimitValues{
		{ContextWindow: modelTokenPointer(0)},
		{MaxOutputTokens: modelTokenPointer(-1)},
		{ContextWindow: modelTokenPointer(1_000), MaxInputTokens: modelTokenPointer(1_001)},
	} {
		if _, err := NewModelTokenLimits(values); !errors.Is(err, ErrInvalidModelTokenLimits) {
			t.Fatalf("NewModelTokenLimits(%+v) error = %v", values, err)
		}
	}
	if _, err := NewModelTokenLimits(ModelTokenLimitValues{
		ContextWindow: modelTokenPointer(16_384), MaxOutputTokens: modelTokenPointer(32_768),
	}); err != nil {
		t.Fatalf("independent streaming output facts rejected: %v", err)
	}
}

func TestModelRejectsContradictoryCatalogMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model Model
		want  string
	}{
		{
			name: "cutoff", want: "knowledge cutoff",
			model: Model{ID: "m", Provider: "p", KnowledgeCutoff: "yesterday"},
		},
		{
			name: "default reasoning", want: "not offered",
			model: Model{ID: "m", Provider: "p", Capabilities: &ModelCapabilities{
				Reasoning: true, ReasoningLevels: []string{"low"}, ReasoningDefaultLevel: "high",
			}},
		},
		{
			name: "duplicate modality", want: "duplicated",
			model: Model{ID: "m", Provider: "p", Capabilities: &ModelCapabilities{
				InputModalities: []ModelModality{ModelModalityText, ModelModalityText},
			}},
		},
		{
			name: "pricing", want: "finite and non-negative",
			model: Model{ID: "m", Provider: "p", Pricing: &ModelPricing{InputUSDPerMillionTokens: math.NaN()}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.model.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestApprovalRulesValidateDeletionAcknowledgement(t *testing.T) {
	rule := ApprovalRule{
		ID: "rule_1", Scope: RememberGlobal, Tool: "shell", Decision: ApprovalRuleAllow,
	}
	if err := ValidateApprovalRuleDeletion(nil, rule.ID); err != nil {
		t.Fatalf("deleted rule acknowledgement: %v", err)
	}
	if err := ValidateApprovalRuleDeletion([]ApprovalRule{rule}, rule.ID); err == nil {
		t.Fatal("accepted an approval rule that remained after deletion")
	}
}

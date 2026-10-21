package catalog

import (
	"errors"
	"testing"

	modelchoice "github.com/Tangerg/flame/runtime/model"
)

func TestSelectionsAdmitSelectionUsesExactCatalogReasoningVocabulary(t *testing.T) {
	supported, err := modelchoice.NewWithReasoningEffort("openai", "gpt-5.6-sol", "high")
	if err != nil {
		t.Fatal(err)
	}
	if err := (Selections{}).AdmitSelection(supported); err != nil {
		t.Fatalf("supported effort error = %v", err)
	}

	for _, selection := range []modelchoice.Selection{
		mustReasoningSelection(t, "openai", "gpt-5.6-sol", "turbo"),
		mustReasoningSelection(t, "alibaba", "qwen-mt-plus", "high"),
	} {
		if err := (Selections{}).AdmitSelection(selection); !errors.Is(err, ErrUnsupportedReasoningEffort) {
			t.Fatalf("selection %q/%q/%q error = %v", selection.Provider(), selection.Model(), selection.ReasoningEffort(), err)
		}
	}
}

func TestSelectionsAdmitSelectionAllowsPrivateCatalogMiss(t *testing.T) {
	selection := mustReasoningSelection(t, "openai-compatible", "private-reasoning-model", "custom")
	if err := (Selections{}).AdmitSelection(selection); err != nil {
		t.Fatalf("private selection error = %v", err)
	}
}

func mustReasoningSelection(t *testing.T, provider, model, effort string) modelchoice.Selection {
	t.Helper()
	selection, err := modelchoice.NewWithReasoningEffort(provider, model, effort)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

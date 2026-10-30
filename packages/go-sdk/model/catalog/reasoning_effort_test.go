package catalog

import (
	"errors"
	"testing"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/catalog"
)

func TestAdmissionUsesExactCatalogReasoningVocabulary(t *testing.T) {
	supported, err := modelchoice.NewWithReasoningEffort("openai", "gpt-5.6-sol", "high")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAdmission(supported, referenceModel(supported)); err != nil {
		t.Fatalf("supported effort error = %v", err)
	}

	for _, selection := range []modelchoice.Selection{
		mustReasoningSelection(t, "openai", "gpt-5.6-sol", "turbo"),
		mustReasoningSelection(t, "alibaba", "qwen-mt-plus", "high"),
	} {
		if _, err := NewAdmission(selection, referenceModel(selection)); !errors.Is(err, ErrUnsupportedReasoningEffort) {
			t.Fatalf("selection %q/%q/%q error = %v", selection.Provider(), selection.Model(), selection.ReasoningEffort(), err)
		}
	}
}

func TestAdmissionAllowsPrivateReasoningVocabularyWhenFactsAreUnknown(t *testing.T) {
	selection := mustReasoningSelection(t, "openai-compatible", "private-reasoning-model", "custom")
	if _, err := NewAdmission(selection, referenceModel(selection)); err != nil {
		t.Fatalf("private selection error = %v", err)
	}
}

func TestAdmissionFreezesSuppliedFactsAndRejectsAnotherModel(t *testing.T) {
	selection := mustReasoningSelection(t, "custom-provider", "custom-model", "custom-effort")
	metadata := &catalog.Model{ID: "custom-model", Reasoning: catalog.Reasoning{Supported: true, Levels: []string{"custom-effort"}}, Modalities: catalog.Modalities{Input: []catalog.Modality{catalog.ModalityText}}}
	admitted, err := NewAdmission(selection, metadata)
	if err != nil || admitted.Selection() != selection {
		t.Fatalf("provider-owned model facts were ignored: %+v %v", admitted, err)
	}
	unsupported := mustReasoningSelection(t, "custom-provider", "custom-model", "another-effort")
	if _, err := NewAdmission(unsupported, metadata); !errors.Is(err, ErrUnsupportedReasoningEffort) {
		t.Fatalf("supplied reasoning vocabulary was ignored: %v", err)
	}
	metadata.Modalities.Input[0] = catalog.ModalityImage
	if err := admitted.AdmitInput([]chat.Message{chat.NewUserMessage(chat.NewTextPart("question"))}); err != nil {
		t.Fatalf("captured facts changed through the input: %v", err)
	}
	metadata.ID = "another-model"
	if _, err := NewAdmission(selection, metadata); err == nil {
		t.Fatal("another model supplied admission facts")
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

func referenceModel(selection modelchoice.Selection) *catalog.Model {
	entry, found := catalog.Default.Lookup(selection.Provider(), selection.Model())
	if !found {
		return nil
	}
	return &entry
}

func referenceAdmission(t *testing.T, selection modelchoice.Selection) Admission {
	t.Helper()
	admitted, err := NewAdmission(selection, referenceModel(selection))
	if err != nil {
		t.Fatal(err)
	}
	return admitted
}

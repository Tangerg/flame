package transcript

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func TestModelOnlyOffloadPreservesStructuredResult(t *testing.T) {
	result, err := tool.ParseResult([]byte(`{"document":"report","revision":7}`))
	if err != nil {
		t.Fatal(err)
	}
	ref := ToolResultRef{ID: "BLOB234", ModelOnly: true}
	hydrated, err := ref.HydrateResult(&result, "full model-visible text")
	if err != nil || !hydrated.Equal(result) {
		t.Fatalf("structured result was replaced by model text: %s, %v", hydrated.Canonical(), err)
	}
	ref.ModelOnly = false
	if err := ref.ValidateResult(&result); err == nil {
		t.Fatal("structured result admitted as a plain text replacement")
	}
}

func TestToolResultOffloadRequiresItsResultAndBody(t *testing.T) {
	result, err := tool.NewResult("preview")
	if err != nil {
		t.Fatal(err)
	}
	for _, modelOnly := range []bool{false, true} {
		ref := ToolResultRef{ID: "BLOB234", ModelOnly: modelOnly}
		if err := ref.ValidateResult(nil); err == nil {
			t.Fatalf("modelOnly=%t admitted an absent result", modelOnly)
		}
		if _, err := ref.HydrateResult(&result, ""); err == nil {
			t.Fatalf("modelOnly=%t hid a missing blob", modelOnly)
		}
	}
}

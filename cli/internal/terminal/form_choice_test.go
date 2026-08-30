package terminal

import (
	"testing"

	"github.com/Tangerg/flame/cli/internal/modelconfig"
)

func TestFormChangeRejectsAnUninitializedSelection(t *testing.T) {
	for _, change := range []formChange{formChangeKeep, formChangeSet, formChangeClear} {
		if err := change.Validate(); err != nil {
			t.Fatalf("valid form change %d: %v", change, err)
		}
	}
	if err := (formChange(0)).Validate(); err == nil {
		t.Fatal("uninitialized form change was accepted")
	}
}

func TestFormChangeBuildsValidatedProviderChanges(t *testing.T) {
	set, err := valueChange(formChangeSet, "https://models.example")
	if err != nil || set == nil || set.Kind != modelconfig.SetValue || set.Value != "https://models.example" {
		t.Fatalf("set change = (%+v, %v)", set, err)
	}
	clear, err := valueChange(formChangeClear, "")
	if err != nil || clear == nil || clear.Kind != modelconfig.ClearValue {
		t.Fatalf("clear change = (%+v, %v)", clear, err)
	}
	keep, err := valueChange(formChangeKeep, "")
	if err != nil || keep != nil {
		t.Fatalf("keep change = (%+v, %v)", keep, err)
	}
	if _, err := valueChange(formChangeSet, ""); err == nil {
		t.Fatal("empty provider replacement was accepted")
	}
}

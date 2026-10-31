package run

import (
	"errors"
	"strings"
	"testing"
)

func TestRunCapabilitiesHasOneDurableRepresentation(t *testing.T) {
	canonical := Capabilities{
		ChildRuns:      true,
		InterruptTypes: []InterruptType{InterruptApproval, InterruptQuestion},
	}
	if err := canonical.Validate(); err != nil {
		t.Fatalf("Validate canonical capabilities: %v", err)
	}
	for name, capabilities := range map[string]Capabilities{
		"duplicate": {
			InterruptTypes: []InterruptType{InterruptApproval, InterruptApproval},
		},
		"unsorted": {
			InterruptTypes: []InterruptType{InterruptQuestion, InterruptApproval},
		},
		"unknown": {
			InterruptTypes: []InterruptType{InterruptType("invalid")},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := capabilities.Validate(); err == nil {
				t.Fatal("Validate accepted a non-canonical capability set")
			}
		})
	}

	spelledDifferently := Capabilities{
		ChildRuns:      true,
		InterruptTypes: []InterruptType{InterruptQuestion, InterruptApproval, InterruptQuestion},
	}
	if !canonical.Equal(spelledDifferently) {
		t.Fatal("Equal treated two spellings of the same capability set as different")
	}
	if canonical.Equal(Capabilities{InterruptTypes: canonical.InterruptTypes}) {
		t.Fatal("Equal ignored the child-Run capability")
	}
}

func TestRunCapabilitiesCloneOwnsInterruptTypes(t *testing.T) {
	original := Capabilities{InterruptTypes: []InterruptType{InterruptApproval}}
	cloned := original.Clone()
	cloned.InterruptTypes[0] = InterruptQuestion
	if original.InterruptTypes[0] != InterruptApproval {
		t.Fatal("Clone shares interrupt-kind storage with the source capabilities")
	}
}

func TestRunCapabilitiesReportsTheCompleteMissingSet(t *testing.T) {
	required := Capabilities{
		ChildRuns:      true,
		InterruptTypes: []InterruptType{InterruptApproval, InterruptQuestion},
	}
	caller := Capabilities{InterruptTypes: []InterruptType{InterruptApproval}}
	missing := required.MissingFrom(caller)
	if !missing.ChildRuns || len(missing.InterruptTypes) != 1 || missing.InterruptTypes[0] != InterruptQuestion {
		t.Fatalf("MissingFrom() = %v, want child runs and question interrupts", missing)
	}
	if text := missing.String(); strings.Contains(text, "interruptTypes") || text != "child runs, question interrupts" {
		t.Fatalf("String() = %q, want semantic capability names", text)
	}

	err := &InsufficientCapabilitiesError{Missing: missing}
	if !errors.Is(err, ErrInsufficientCapabilities) || !strings.Contains(err.Error(), `requires child runs, question interrupts`) {
		t.Fatalf("InsufficientCapabilitiesError = %v", err)
	}
}

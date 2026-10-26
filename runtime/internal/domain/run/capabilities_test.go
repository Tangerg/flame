package run

import (
	"errors"
	"strings"
	"testing"
)

func TestRunCapabilitiesHasOneDurableRepresentation(t *testing.T) {
	canonical := Capabilities{
		ChildRuns:      true,
		InterruptKinds: []InterruptKind{InterruptApproval, InterruptQuestion},
	}
	if err := canonical.Validate(); err != nil {
		t.Fatalf("Validate canonical capabilities: %v", err)
	}
	for name, capabilities := range map[string]Capabilities{
		"duplicate": {
			InterruptKinds: []InterruptKind{InterruptApproval, InterruptApproval},
		},
		"unsorted": {
			InterruptKinds: []InterruptKind{InterruptQuestion, InterruptApproval},
		},
		"unknown": {
			InterruptKinds: []InterruptKind{InterruptKind("invalid")},
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
		InterruptKinds: []InterruptKind{InterruptQuestion, InterruptApproval, InterruptQuestion},
	}
	if !canonical.Equal(spelledDifferently) {
		t.Fatal("Equal treated two spellings of the same capability set as different")
	}
	if canonical.Equal(Capabilities{InterruptKinds: canonical.InterruptKinds}) {
		t.Fatal("Equal ignored the child-Run capability")
	}
}

func TestRunCapabilitiesCloneOwnsInterruptKinds(t *testing.T) {
	original := Capabilities{InterruptKinds: []InterruptKind{InterruptApproval}}
	cloned := original.Clone()
	cloned.InterruptKinds[0] = InterruptQuestion
	if original.InterruptKinds[0] != InterruptApproval {
		t.Fatal("Clone shares interrupt-kind storage with the source capabilities")
	}
}

func TestRunCapabilitiesReportsTheCompleteMissingSet(t *testing.T) {
	required := Capabilities{
		ChildRuns:      true,
		InterruptKinds: []InterruptKind{InterruptApproval, InterruptQuestion},
	}
	caller := Capabilities{InterruptKinds: []InterruptKind{InterruptApproval}}
	missing := required.MissingFrom(caller)
	if !missing.ChildRuns || len(missing.InterruptKinds) != 1 || missing.InterruptKinds[0] != InterruptQuestion {
		t.Fatalf("MissingFrom() = %v, want child runs and question interrupts", missing)
	}
	if text := missing.String(); strings.Contains(text, "interruptTypes") || text != "child runs, question interrupts" {
		t.Fatalf("String() = %q, want semantic capability names", text)
	}

	err := &InsufficientCapabilitiesError{RunID: "run_1", Missing: missing}
	if !errors.Is(err, ErrInsufficientCapabilities) || !strings.Contains(err.Error(), `run "run_1" requires child runs, question interrupts`) {
		t.Fatalf("InsufficientCapabilitiesError = %v", err)
	}
}

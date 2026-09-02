package agentexec

import (
	"math"
	"reflect"
	"testing"
)

func TestAddInteractionCallCountsRejectsOverflowWithoutMutatingInput(t *testing.T) {
	current := map[string]int{"model": math.MaxInt}
	before := map[string]int{"model": math.MaxInt}
	if _, err := addInteractionCallCounts(current, map[string]int{"model": 1}); err == nil {
		t.Fatal("overflowing call count was accepted")
	}
	if !reflect.DeepEqual(current, before) {
		t.Fatalf("rejected addition mutated input: got %v, want %v", current, before)
	}
}

func TestAddInteractionCallCountsBuildsIndependentCandidate(t *testing.T) {
	current := map[string]int{"alpha": 1}
	next, err := addInteractionCallCounts(current, map[string]int{"alpha": 2, "beta": 3})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(next, map[string]int{"alpha": 3, "beta": 3}) {
		t.Fatalf("merged calls = %v", next)
	}
	next["alpha"] = 9
	if current["alpha"] != 1 {
		t.Fatalf("candidate mutation changed source: %v", current)
	}
}

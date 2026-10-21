package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/exactint"
)

func TestPolicyOwnsExactMatchingAndDetachedRevisions(t *testing.T) {
	phrases := []string{"Patient@example.test", "private-凭证"}
	policy, err := NewPolicy().Replace(1, phrases)
	if err != nil {
		t.Fatal(err)
	}
	phrases[0] = "forged"
	policy.Snapshot().Phrases[1] = "forged"
	for _, text := range []string{"Contact Patient@example.test.", "private-凭证"} {
		if err := policy.Admit(text); !errors.Is(err, ErrProtectedContent) || strings.Contains(err.Error(), text) {
			t.Fatalf("protected content was allowed or disclosed: %v", err)
		}
	}
	if err := policy.Admit("patient@example.test"); err != nil {
		t.Fatalf("exact matching changed case: %v", err)
	}
	if _, err := policy.Replace(1, []string{}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale edit was accepted: %v", err)
	}
	cleared, err := policy.Replace(2, []string{})
	if err != nil || cleared.Summary() != (Summary{Revision: 3}) || cleared.Admit("private-凭证") != nil {
		t.Fatalf("explicit policy clear: %+v %v", cleared.Summary(), err)
	}
}

func TestPolicyRefusesMissingDuplicatedOrUnboundedFacts(t *testing.T) {
	for _, phrases := range [][]string{nil, {" "}, {"same", "same"}, {string([]byte{0xff})}, {strings.Repeat("x", 513)}, slices.Repeat([]string{"term"}, 33)} {
		if _, err := Restore(Snapshot{Revision: 1, Phrases: phrases}); !errors.Is(err, ErrInvalidPolicy) {
			t.Fatalf("invalid policy admitted: %v", err)
		}
	}
	policy, err := Restore(Snapshot{Revision: exactint.Maximum, Phrases: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Replace(exactint.Maximum, []string{}); !errors.Is(err, ErrRevisionLimit) {
		t.Fatalf("revision wrapped: %v", err)
	}
}

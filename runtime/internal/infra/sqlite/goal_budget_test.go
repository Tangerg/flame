package sqlite

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/goal"
)

func TestGoalBudgetCodecUsesExplicitCurrentShape(t *testing.T) {
	unlimited, err := encodeGoalBudget(goal.UnlimitedBudget())
	if err != nil {
		t.Fatal(err)
	}
	if unlimited != `{"type":"unlimited"}` {
		t.Fatalf("unlimited budget = %s", unlimited)
	}
	if decoded, err := decodeGoalBudget(unlimited); err != nil || !decoded.Unlimited() {
		t.Fatalf("decode unlimited = %+v, %v", decoded, err)
	}

	runs, cost := 3, 0.25
	limited, err := goal.NewBudget(goal.BudgetLimits{MaxRuns: &runs, MaxCostUSD: &cost})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeGoalBudget(limited)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeGoalBudget(encoded)
	if err != nil || decoded != limited {
		t.Fatalf("limited round trip = %+v, %v; encoded %s", decoded, err, encoded)
	}
}

func TestGoalBudgetCodecRejectsOldAmbiguousAndMalformedShapes(t *testing.T) {
	for name, encoded := range map[string]string{
		"old zero fields":    `{"max_runs":0,"max_cost_usd":0,"max_steps":0}`,
		"limited zero":       `{"type":"limited","max_runs":0}`,
		"limited empty":      `{"type":"limited"}`,
		"unlimited with cap": `{"type":"unlimited","max_runs":1}`,
		"unknown type":       `{"type":"sometimes"}`,
		"unknown field":      `{"type":"unlimited","legacy":true}`,
		"trailing value":     `{"type":"unlimited"} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeGoalBudget(encoded); err == nil {
				t.Fatalf("decodeGoalBudget(%s) succeeded", encoded)
			}
		})
	}
	if encoded, err := encodeGoalBudget(goal.Budget{}); err == nil || !strings.Contains(err.Error(), "constructed explicitly") || encoded != "" {
		t.Fatalf("encode zero Budget = %q, %v", encoded, err)
	}
}

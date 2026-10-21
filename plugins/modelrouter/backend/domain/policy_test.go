package domain

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/exactint"
	"github.com/Tangerg/flame/runtime/model"
)

func TestPolicyRoutesCapturedUsesAndBudgetsWithoutChangingTheProposal(t *testing.T) {
	proposed, _ := model.New("anthropic", "large")
	cheap, _ := model.NewWithReasoningEffort("openai", "small", "low")
	ceiling := model.SelectionLimits{InputKind: model.TextBytes, MaxInputBytes: 4096, MaxOutputTokens: 128}
	policy, err := NewPolicy().Replace(1, []Rule{{Use: model.Utility, Selection: cheap, Ceiling: &ceiling}})
	if err != nil {
		t.Fatal(err)
	}
	ceiling.MaxInputBytes = 1
	projected := policy.Snapshot()
	projected.Rules[0].Ceiling.MaxOutputTokens = 1
	for _, tc := range []struct {
		name   string
		use    model.Use
		limits *model.SelectionLimits
		want   model.Selection
	}{
		{"within", model.Utility, &model.SelectionLimits{InputKind: model.TextBytes, MaxInputBytes: 4096, MaxOutputTokens: 128}, cheap},
		{"input exceeds", model.Utility, &model.SelectionLimits{InputKind: model.TextBytes, MaxInputBytes: 4097, MaxOutputTokens: 128}, proposed},
		{"output exceeds", model.Utility, &model.SelectionLimits{InputKind: model.TextBytes, MaxInputBytes: 4096, MaxOutputTokens: 129}, proposed},
		{"different use", model.Independent, &model.SelectionLimits{InputKind: model.RequestBytes, MaxInputBytes: 4096, MaxOutputTokens: 128}, proposed},
		{"interaction", model.Interaction, nil, proposed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			context := model.SelectionContext{Use: tc.use, Limits: tc.limits}
			selected, err := policy.Select(context, proposed)
			if err != nil || selected != tc.want {
				t.Fatalf("route: %+v, %v", selected, err)
			}
		})
	}
	if _, err := policy.Select(model.SelectionContext{Use: model.Utility}, proposed); err == nil {
		t.Fatal("missing caller envelope was inferred")
	}
	if _, err := policy.Replace(1, nil); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale replacement: %v", err)
	}
}

func TestPolicyRefusesAmbiguousRulesInvalidUnitsAndRevisionExhaustion(t *testing.T) {
	selected, _ := model.New("openai", "small")
	for _, rules := range [][]Rule{
		{{Use: model.Utility, Selection: selected}, {Use: model.Utility, Selection: selected}},
		{{Use: "invented", Selection: selected}},
		{{Use: model.Utility}},
		{{Use: model.Utility, Selection: selected, Ceiling: &model.SelectionLimits{InputKind: model.RequestBytes, MaxInputBytes: 1, MaxOutputTokens: 1}}},
		{{Use: model.Interaction, Selection: selected, Ceiling: &model.SelectionLimits{InputKind: model.TextBytes, MaxInputBytes: 1, MaxOutputTokens: 1}}},
	} {
		if _, err := NewPolicy().Replace(1, rules); !errors.Is(err, ErrInvalidPolicy) {
			t.Fatalf("invalid policy admitted: %+v, %v", rules, err)
		}
	}
	if _, err := Restore(Snapshot{Revision: 1}); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("missing rule list became a pass-through policy: %v", err)
	}
	policy, err := Restore(Snapshot{Revision: exactint.Maximum, Rules: []Rule{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Replace(exactint.Maximum, nil); !errors.Is(err, ErrRevisionLimit) {
		t.Fatalf("revision wrapped: %v", err)
	}
}

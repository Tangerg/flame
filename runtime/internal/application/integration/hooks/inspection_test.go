package hooks

import (
	"slices"
	"testing"

	domain "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
)

func TestInspectionAcceptsResolvedGlobalToProjectCascade(t *testing.T) {
	inspection := validInspection()
	if err := inspection.ValidateFor("/repo/pkg"); err != nil {
		t.Fatalf("ValidateFor: %v", err)
	}
}

func TestInspectionRejectsBrokenResolvedCascades(t *testing.T) {
	for name, mutate := range map[string]func(*Inspection){
		"unrelated project root": func(value *Inspection) { value.ProjectRoot = "/other" },
		"non-canonical root":     func(value *Inspection) { value.ProjectRoot = "/repo/.." },
		"trusted without root":   func(value *Inspection) { value.ProjectRoot = "" },
		"invalid hook": func(value *Inspection) {
			value.Hooks[0].Inject = ""
		},
		"unknown scope": func(value *Inspection) {
			value.Hooks[0].Scope = domain.Scope("other")
		},
		"relative source": func(value *Inspection) {
			value.Hooks[0].Source = ".flame/hooks.json"
		},
		"project source outside root": func(value *Inspection) {
			value.Hooks[1].Source = "/other/.flame/hooks.json"
		},
		"global after project": func(value *Inspection) {
			value.Hooks[0], value.Hooks[1] = value.Hooks[1], value.Hooks[0]
		},
	} {
		t.Run(name, func(t *testing.T) {
			inspection := validInspection()
			inspection.Hooks = slices.Clone(inspection.Hooks)
			mutate(&inspection)
			if err := inspection.ValidateFor("/repo/pkg"); err == nil {
				t.Fatal("ValidateFor accepted a broken Hook inspection")
			}
		})
	}

	t.Run("oversized cascade", func(t *testing.T) {
		inspection := validInspection()
		inspection.Hooks = make([]domain.Hook, domain.MaxHooksPerCascade+1)
		for index := range inspection.Hooks {
			inspection.Hooks[index] = domain.Hook{
				Event: domain.Stop, Inject: "context", Scope: domain.ScopeGlobal,
				Source: "/home/.flame/hooks.json",
			}
		}
		if err := inspection.ValidateFor("/repo/pkg"); err == nil {
			t.Fatal("ValidateFor accepted an oversized Hook cascade")
		}
	})
}

func validInspection() Inspection {
	return Inspection{
		ProjectRoot:    "/repo",
		ProjectTrusted: true,
		Hooks: []domain.Hook{
			{Event: domain.Stop, Inject: "global", Scope: domain.ScopeGlobal, Source: "/home/.flame/hooks.json"},
			{Event: domain.PreToolUse, Command: "check", Scope: domain.ScopeProject, Source: "/repo/pkg/.flame/hooks.json"},
		},
	}
}

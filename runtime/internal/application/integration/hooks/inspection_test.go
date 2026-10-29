package hooks

import (
	"slices"
	"testing"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
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
			value.Hooks[0].Scope = hookdata.Scope("other")
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
		inspection.Hooks = make([]hookdata.Hook, hookdata.MaxHooksPerCascade+1)
		for index := range inspection.Hooks {
			inspection.Hooks[index] = hookdata.Hook{
				Event: hookdata.Stop, Inject: "context", Scope: hookdata.ScopeGlobal,
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
		Hooks: []hookdata.Hook{
			{Event: hookdata.Stop, Inject: "global", Scope: hookdata.ScopeGlobal, Source: "/home/.flame/hooks.json"},
			{Event: hookdata.PreToolUse, Command: "check", Scope: hookdata.ScopeProject, Source: "/repo/pkg/.flame/hooks.json"},
		},
	}
}

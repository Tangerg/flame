package testsupport

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/skill"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
)

// FixtureSkillSelection models an installed provider in reader/confinement
// tests. Provider registration and recovery use the real release in Bootstrap.
func FixtureSkillSelection[T any](_ context.Context, input skilllearning.Candidates, _ *T) (skilllearning.Catalog, error) {
	byName := map[string][]skilllearning.Candidate{}
	for _, item := range input.Items {
		byName[item.Name] = append(byName[item.Name], item)
	}
	result := skilllearning.Catalog{}
	for name, candidates := range byName {
		best := ""
		for _, scope := range []skill.Scope{skill.Project, skill.User, skill.Installation} {
			found := 0
			for _, candidate := range candidates {
				if candidate.Scope == scope {
					found++
					best = candidate.Source
				}
			}
			if found > 1 {
				best = ""
			}
			if found != 0 {
				break
			}
		}
		result.Entries = append(result.Entries, skilllearning.Choice{Name: name, Source: best})
	}
	return result, nil
}

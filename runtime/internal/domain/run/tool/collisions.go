package tool

import (
	"slices"
	"strings"
)

// NameConflicts identifies remote tools excluded from model manifests. Built-in
// names stay reserved even when that built-in is unavailable in a particular Run.
// Each value names the other identities competing for the same model name.
func NameConflicts(refs []Ref) map[Ref][]Ref {
	byName := make(map[string][]Ref)
	for _, ref := range refs {
		name := ref.ModelName()
		if !slices.Contains(byName[name], ref) {
			byName[name] = append(byName[name], ref)
		}
	}
	conflicts := make(map[Ref][]Ref)
	for name, candidates := range byName {
		if reserved, err := BuiltIn(BuiltInName(name)); err == nil && !slices.Contains(candidates, reserved) {
			candidates = append(candidates, reserved)
		}
		slices.SortFunc(candidates, func(a, b Ref) int { return strings.Compare(a.String(), b.String()) })
		for _, ref := range candidates {
			if ref.Kind() == BuiltInKind {
				continue
			}
			for _, other := range candidates {
				if other != ref {
					conflicts[ref] = append(conflicts[ref], other)
				}
			}
		}
	}
	return conflicts
}

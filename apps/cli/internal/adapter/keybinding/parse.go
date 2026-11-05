package keybinding

import (
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/oolong/ui/core/input"
)

type Binding struct {
	Action settings.Action
	Keys   input.Keys
}

func Parse(actions map[settings.Action][]string) ([]Binding, error) {
	var bindings []Binding
	owners := make(map[string]settings.Action)
	for _, action := range slices.Sorted(maps.Keys(actions)) {
		for _, encoded := range actions[action] {
			sequence, ok := input.ParseKeys(encoded)
			if !ok {
				return nil, fmt.Errorf("session settings: keys.%s contains invalid binding %q", action, encoded)
			}
			identity := sequence.String()
			if owner, exists := owners[identity]; exists && owner != action {
				return nil, fmt.Errorf("session settings: key %q is bound to both %s and %s", identity, owner, action)
			}
			owners[identity] = action
			bindings = append(bindings, Binding{Action: action, Keys: sequence})
		}
	}
	return bindings, nil
}

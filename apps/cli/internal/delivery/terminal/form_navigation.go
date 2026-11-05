package terminal

import (
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// Form cancellation belongs to a pending submission. Leaving an editable
// product step is a separate intent, reached only if the form did not consume it.
func formNavigation(body scene.Element, leave func()) scene.Element {
	group := scene.NewGroup(scene.GroupConfig{
		Items: []scene.Item{{Element: body, Size: layout.Flex(1)}},
		Element: scene.ElementConfig{
			Handle: func(event scene.Event) scene.EventResult {
				key, ok := event.Input().(input.Key)
				if !ok || !key.Down() || !key.Is(input.Esc, 0) {
					return scene.EventResult{}
				}
				leave()
				return scene.EventResult{Consumed: true}
			},
		},
	})
	return group.Element()
}

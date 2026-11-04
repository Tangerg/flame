package terminal

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// passiveElement admits a measured, noninteractive drawing into the scene.
// Interactive controls expose their own Element and never enter this boundary.
func passiveElement(content interface {
	grid.Drawable
	HeightForWidth(int) int
}) scene.Element {
	return scene.NewElement(scene.ElementConfig{
		Paint: func(frame scene.Frame) { content.Draw(frame.View) },
		Measure: func(axis layout.Axis, width int) int {
			if axis == layout.Down {
				return content.HeightForWidth(width)
			}
			return 0
		},
	})
}

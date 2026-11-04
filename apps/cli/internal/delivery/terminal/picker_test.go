package terminal

import (
	"fmt"
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestPickerClickCommitsOnlyAfterMatchingRelease(t *testing.T) {
	picked := ""
	picker := newPicker(kit.Dark(), kit.Unicode(), "search",
		func(value string) string { return value },
		func(string) string { return "" },
		func(value string) { picked = value },
	)
	picker.SetItems([]string{"first", "second", "third"})
	root := newSceneTestRoot(t, picker.Element())
	surface := grid.NewSurface(40, 7)
	root.Draw(surface.View())
	second := pickerPoint(t, picker, surface, 1)

	if !root.Handle(input.Mouse{Pos: second, Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("picker press was not handled")
	}
	if picked != "" || picker.items.Selected() != 1 {
		t.Fatalf("press picked %q with selection %d", picked, picker.items.Selected())
	}
	root.Handle(input.Mouse{Action: input.MouseLeave})
	if !root.Handle(input.Mouse{Pos: second, Action: input.MouseUp, Button: input.ButtonLeft}) {
		t.Fatal("picker release was not handled")
	}
	if picked != "second" {
		t.Fatalf("matching release picked %q", picked)
	}
}

func TestPickerDragChangesSelectionWithoutActivation(t *testing.T) {
	picked := ""
	picker := newPicker(kit.Dark(), kit.Unicode(), "search",
		func(value string) string { return value },
		func(string) string { return "" },
		func(value string) { picked = value },
	)
	picker.SetItems([]string{"first", "second", "third"})
	root := newSceneTestRoot(t, picker.Element())
	surface := grid.NewSurface(40, 7)
	root.Draw(surface.View())
	first, third := pickerPoint(t, picker, surface, 0), pickerPoint(t, picker, surface, 2)

	root.Handle(input.Mouse{Pos: first, Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: third, Action: input.MouseDrag, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: third, Action: input.MouseUp, Button: input.ButtonLeft})
	if picked != "" {
		t.Fatalf("drag activated %q", picked)
	}
	if got := picker.items.Selected(); got != 2 {
		t.Fatalf("drag selection = %d, want 2", got)
	}
}

func TestPickerReleaseOutsideTheListDoesNotActivateThePressedRow(t *testing.T) {
	for _, x := range []int{-1, 40} {
		t.Run(fmt.Sprint(x), func(t *testing.T) {
			picked := ""
			picker := newPicker(kit.Dark(), kit.Unicode(), "search",
				func(value string) string { return value },
				func(string) string { return "" },
				func(value string) { picked = value },
			)
			picker.SetItems([]string{"first", "second", "third"})
			root := newSceneTestRoot(t, picker.Element())
			surface := grid.NewSurface(40, 7)
			root.Draw(surface.View())
			second := pickerPoint(t, picker, surface, 1)

			root.Handle(input.Mouse{Pos: second, Action: input.MouseDown, Button: input.ButtonLeft})
			root.Handle(input.Mouse{Pos: image.Pt(x, second.Y), Action: input.MouseUp, Button: input.ButtonLeft})
			if picked != "" {
				t.Fatalf("release outside the list activated %q", picked)
			}
		})
	}
}

func TestPickerCancelsAStalePointerGesture(t *testing.T) {
	tests := []struct {
		name      string
		interrupt func(*picker[string], *sceneTestRoot, image.Point)
	}{
		{
			name: "capture cancellation",
			interrupt: func(_ *picker[string], root *sceneTestRoot, point image.Point) {
				root.Handle(input.Mouse{Pos: point, Action: input.MouseCancel})
			},
		},
		{
			name: "different button press",
			interrupt: func(_ *picker[string], root *sceneTestRoot, point image.Point) {
				root.Handle(input.Mouse{Pos: point, Action: input.MouseDown, Button: input.ButtonRight})
			},
		},
		{
			name: "focus loss",
			interrupt: func(picker *picker[string], _ *sceneTestRoot, _ image.Point) {
				_ = picker.query.Element().RequestFocusFirst()

			},
		},
		{
			name: "catalog replacement",
			interrupt: func(picker *picker[string], _ *sceneTestRoot, _ image.Point) {
				picker.SetItems([]string{"replacement first", "replacement second", "replacement third"})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			picked := ""
			picker := newPicker(kit.Dark(), kit.Unicode(), "search",
				func(value string) string { return value },
				func(string) string { return "" },
				func(value string) { picked = value },
			)
			picker.SetItems([]string{"first", "second", "third"})

			root := newSceneTestRoot(t, picker.Element())
			surface := grid.NewSurface(40, 7)
			root.Draw(surface.View())
			second := pickerPoint(t, picker, surface, 1)

			root.Handle(input.Mouse{Pos: second, Action: input.MouseDown, Button: input.ButtonLeft})
			test.interrupt(picker, root, second)
			root.Handle(input.Mouse{Pos: second, Action: input.MouseUp, Button: input.ButtonLeft})
			if picked != "" {
				t.Fatalf("stale pointer gesture picked %q", picked)
			}
		})
	}
}

func TestPickerForeignButtonReleasePreservesTheNativeCapture(t *testing.T) {
	picked := ""
	picker := newPicker(kit.Dark(), kit.Unicode(), "search",
		func(value string) string { return value },
		func(string) string { return "" },
		func(value string) { picked = value },
	)
	picker.SetItems([]string{"first", "second"})
	root := newSceneTestRoot(t, picker.Element())
	surface := grid.NewSurface(40, 7)
	root.Draw(surface.View())
	second := pickerPoint(t, picker, surface, 1)
	root.Handle(input.Mouse{Pos: second, Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: second, Action: input.MouseUp, Button: input.ButtonRight})
	if picked != "" {
		t.Fatal("an unrelated release committed the captured left-button press")
	}
	root.Handle(input.Mouse{Pos: second, Action: input.MouseUp, Button: input.ButtonLeft})
	if picked != "second" {
		t.Fatalf("the matching release selected %q, want second", picked)
	}
}

func TestPickerDoesNotCommitAPointerGestureInterruptedByNavigation(t *testing.T) {
	tests := []struct {
		name      string
		interrupt func(*sceneTestRoot, image.Point)
	}{
		{
			name: "keyboard",
			interrupt: func(root *sceneTestRoot, _ image.Point) {
				root.Handle(input.Key{Code: input.Down})
				root.Handle(input.Key{Code: input.Up})
			},
		},
		{
			name: "wheel",
			interrupt: func(root *sceneTestRoot, point image.Point) {
				root.Handle(input.Mouse{Pos: point, Action: input.WheelDown})
				root.Handle(input.Mouse{Pos: point, Action: input.WheelUp})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			picked := ""
			picker := newPicker(kit.Dark(), kit.Unicode(), "search",
				func(value string) string { return value },
				func(string) string { return "" },
				func(value string) { picked = value },
			)
			picker.SetItems([]string{"first", "second", "third", "fourth", "fifth", "sixth", "seventh"})
			root := newSceneTestRoot(t, picker.Element())
			surface := grid.NewSurface(40, 7)
			root.Draw(surface.View())
			second := pickerPoint(t, picker, surface, 1)

			root.Handle(input.Mouse{Pos: second, Action: input.MouseDown, Button: input.ButtonLeft})
			test.interrupt(root, second)
			root.Handle(input.Mouse{Pos: second, Action: input.MouseUp, Button: input.ButtonLeft})
			if picked != "" {
				t.Fatalf("interrupted pointer gesture picked %q", picked)
			}
		})
	}
}

func pickerPoint[T any](t testing.TB, picker *picker[T], surface *grid.Surface, sourceIndex int) image.Point {
	t.Helper()
	return drawnTextOrigin(t, surface, picker.label(picker.items.Items()[sourceIndex]))
}

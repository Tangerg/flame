package terminal

import (
	"image"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestConfirmationContentPanePointerFollowsDrawnRegions(t *testing.T) {
	pane, confirmed := confirmationContentTestPane()
	root := newSceneTestRoot(t, pane.Element())
	for _, size := range []image.Point{{X: 76, Y: 24}, {X: 46, Y: 12}} {
		surface := grid.NewSurface(size.X, size.Y)
		root.Draw(surface.View())
		body := drawnTextOrigin(t, surface, "Reading must not choose approval.")
		root.Handle(input.Mouse{Pos: body, Action: input.MouseDown, Button: input.ButtonLeft})
		root.Handle(input.Mouse{Pos: body, Action: input.MouseUp, Button: input.ButtonLeft})
		if confirmed() {
			t.Fatalf("body click chose approval at %v", size)
		}

		approve := drawnTextOrigin(t, surface, "Approve")
		if !root.Handle(input.Mouse{Pos: approve, Action: input.MouseDown, Button: input.ButtonLeft}) {
			t.Fatalf("visible approval choice declined its click at %v", size)
		}
		root.Handle(input.Mouse{Pos: approve, Action: input.MouseUp, Button: input.ButtonLeft})
		if !confirmed() {
			t.Fatalf("visible approval choice did not select approval at %v", size)
		}

		cancel := drawnTextOrigin(t, surface, "Cancel")
		root.Handle(input.Mouse{Pos: cancel, Action: input.MouseDown, Button: input.ButtonLeft})
		root.Handle(input.Mouse{Pos: cancel, Action: input.MouseUp, Button: input.ButtonLeft})
		if confirmed() {
			t.Fatalf("visible cancel choice did not clear approval at %v", size)
		}
	}
}

func TestConfirmationContentPaneWheelReadsBody(t *testing.T) {
	pane, confirmed := confirmationContentTestPane()
	root := newSceneTestRoot(t, pane.Element())
	surface := grid.NewSurface(76, 24)
	root.Draw(surface.View())
	before := surface.Rows()[0]
	body := drawnTextOrigin(t, surface, "Reading must not choose approval.")
	if !root.Handle(input.Mouse{Pos: body, Action: input.WheelDown}) {
		t.Fatal("body declined wheel input")
	}
	surface = grid.NewSurface(76, 24)
	root.Draw(surface.View())
	if after := surface.Rows()[0]; after == before {
		t.Fatal("wheel over the body did not reveal later instructions")
	}
	if confirmed() {
		t.Fatal("reading instructions selected approval")
	}
	drawnTextOrigin(t, surface, "Approve")
	drawnTextOrigin(t, surface, "Cancel")
}

func TestConfirmationContentPaneKeepsKeyboardFocusOnDecisions(t *testing.T) {
	pane, confirmed := confirmationContentTestPane()
	root := newSceneTestRoot(t, pane.Element())
	surface := grid.NewSurface(76, 24)
	root.Draw(surface.View())
	if err := pane.Element().RequestFocusFirst(); err != nil {
		t.Fatal(err)
	}
	root.Handle(input.Key{Code: input.Down})
	if !confirmed() {
		t.Fatal("the reviewed document intercepted the decision's arrow key")
	}
	root.Handle(input.Key{Code: input.Up})
	if confirmed() {
		t.Fatal("the decision could not return to its original choice")
	}
}

func confirmationContentTestPane() (*confirmationContentPane, func() bool) {
	choice := base.NewSelect(base.SelectConfig[bool]{
		Same: base.Equal[bool], Label: "Decide this exact revision", Initial: false, Rows: 2,
	})
	choice.SetOptions([]base.Option[bool]{
		{Label: "Cancel", Value: false},
		{Label: "Approve", Value: true},
	})
	theme := kit.Dark()
	content := "Revision: exact reviewed revision\n\nReading must not choose approval.\n" + strings.Repeat("Read every instruction.\n", 80)
	return &confirmationContentPane{
		viewport: base.NewViewport(base.ViewportConfig{Content: passiveElement(kit.NewParagraph(content, theme.Text))}),
		form: kit.NewForm(kit.FormConfig{
			Theme: theme, Glyphs: kit.Unicode(), Fields: []base.Field{choice},
		}),
	}, func() bool { value, _ := choice.Chosen(); return value.Value }
}

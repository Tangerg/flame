package terminal

import (
	"image"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func TestConfirmationContentPanePointerFollowsDrawnRegions(t *testing.T) {
	pane, confirmed := confirmationContentTestPane()
	root := headless.NewRoot(pane)
	pane.Focus(true)
	for _, size := range []image.Point{{X: 76, Y: 24}, {X: 46, Y: 12}} {
		surface := grid.NewSurface(size.X, size.Y)
		root.Draw(surface.View())
		body := drawnTextOrigin(t, surface, "Reading must not choose approval.")
		root.Handle(input.Mouse{Pos: body, Action: input.MouseDown, Button: input.ButtonLeft})
		root.Handle(input.Mouse{Pos: body, Action: input.MouseUp, Button: input.ButtonLeft})
		if *confirmed {
			t.Fatalf("body click chose approval at %v", size)
		}

		approve := drawnTextOrigin(t, surface, "Approve")
		if !root.Handle(input.Mouse{Pos: approve, Action: input.MouseDown, Button: input.ButtonLeft}) {
			t.Fatalf("visible approval choice declined its click at %v", size)
		}
		root.Handle(input.Mouse{Pos: approve, Action: input.MouseUp, Button: input.ButtonLeft})
		if !*confirmed {
			t.Fatalf("visible approval choice did not select approval at %v", size)
		}

		cancel := drawnTextOrigin(t, surface, "Cancel")
		root.Handle(input.Mouse{Pos: cancel, Action: input.MouseDown, Button: input.ButtonLeft})
		root.Handle(input.Mouse{Pos: cancel, Action: input.MouseUp, Button: input.ButtonLeft})
		if *confirmed {
			t.Fatalf("visible cancel choice did not clear approval at %v", size)
		}
	}
}

func TestConfirmationContentPaneWheelReadsBody(t *testing.T) {
	pane, confirmed := confirmationContentTestPane()
	root := headless.NewRoot(pane)
	pane.Focus(true)
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
	if *confirmed {
		t.Fatal("reading instructions selected approval")
	}
	drawnTextOrigin(t, surface, "Approve")
	drawnTextOrigin(t, surface, "Cancel")
}

func confirmationContentTestPane() (*confirmationContentPane, *bool) {
	confirmed := new(bool)
	choice := &headless.Select[bool]{
		Same: headless.Equal[bool], Label: "Decide this exact revision", Value: headless.Bind(confirmed), Rows: 2,
	}
	choice.SetOptions([]headless.Option[bool]{
		{Label: "Cancel", Value: false},
		{Label: "Approve", Value: true},
	})
	theme := kit.Dark()
	content := "Revision: exact reviewed revision\n\nReading must not choose approval.\n" + strings.Repeat("Read every instruction.\n", 80)
	return &confirmationContentPane{
		viewport: headless.NewViewport(headless.Static{Of: kit.NewParagraph(content, theme.Text)}),
		form: kit.NewForm(kit.FormConfig{
			Theme: theme, Glyphs: kit.Unicode(), Controller: headless.NewForm(choice),
		}),
	}, confirmed
}

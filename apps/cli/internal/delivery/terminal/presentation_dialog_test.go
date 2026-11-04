package terminal

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestPresentationDialogRenewPermanentlyRetiresPriorIdentity(t *testing.T) {
	var stack workspaceScene
	dialog := newPresentationDialog(&stack, kit.DialogConfig{Theme: kit.Dark(), Glyphs: kit.Unicode(), Body: scene.NewElement(scene.ElementConfig{})})
	dialog.Show()
	prior := dialog.Identity()
	dialog.Show()
	if dialog.Identity().current(prior) || prior.current(prior) {
		t.Fatal("presentation renewal reactivated its prior identity")
	}
}

type presentationProbe struct {
	handled int
	focused bool
}

func (p *presentationProbe) Element() scene.Element {
	return scene.NewElement(scene.ElementConfig{
		Focusable:    func() bool { return true },
		FocusChanged: func(focused bool) { p.focused = focused },
		Handle: func(event scene.Event) scene.EventResult {
			key, ok := event.Input().(input.Key)
			if !ok || !key.Down() || key.Code != input.Enter {
				return scene.EventResult{}
			}
			p.handled++
			return scene.EventResult{Consumed: true}
		},
	})
}

func TestPresentationDialogRejectsInputUntilTheCurrentOpeningIsVisible(t *testing.T) {
	var stack workspaceScene
	probe := &presentationProbe{}
	dialog := newPresentationDialog(&stack, kit.DialogConfig{Theme: kit.Dark(), Glyphs: kit.Unicode(), Title: "Probe", Body: probe.Element()})
	root := newSceneTestRoot(t, stack.Element())
	surface := grid.NewSurface(40, 8)
	enter := input.Key{Code: input.Enter}

	dialog.Show()
	root.Draw(surface.View())
	if !probe.focused || !root.Handle(enter) || probe.handled != 1 {
		t.Fatalf("visible opening = focused %t, handled %d", probe.focused, probe.handled)
	}

	dialog.Dismiss()
	root.Handle(enter)
	if probe.focused || probe.handled != 1 {
		t.Fatalf("dismissed opening accepted input: focused %t, handled %d", probe.focused, probe.handled)
	}

	dialog.Show()
	root.Handle(enter)
	if !dialog.Open() || probe.handled != 1 {
		t.Fatalf("undrawn replacement accepted input: open %t, handled %d", dialog.Open(), probe.handled)
	}
	root.Draw(surface.View())
	if !root.Handle(enter) || probe.handled != 2 {
		t.Fatalf("visible replacement handled %d inputs, want 2", probe.handled)
	}
}

func TestPresentationDialogShowInvalidatesAnOpenPresentation(t *testing.T) {
	var stack workspaceScene
	probe := &presentationProbe{}
	dialog := newPresentationDialog(&stack, kit.DialogConfig{Theme: kit.Dark(), Glyphs: kit.Unicode(), Title: "Probe", Body: probe.Element()})
	root := newSceneTestRoot(t, stack.Element())
	surface := grid.NewSurface(40, 8)
	enter := input.Key{Code: input.Enter}

	dialog.Show()
	root.Draw(surface.View())
	dialog.Show()
	root.Handle(enter)
	if probe.handled != 0 {
		t.Fatalf("refreshed but undrawn presentation handled %d inputs", probe.handled)
	}
	root.Draw(surface.View())
	if !root.Handle(enter) || probe.handled != 1 {
		t.Fatalf("drawn refresh handled %d inputs, want 1", probe.handled)
	}
}

func TestPresentationDialogKeepsItsKeyboardRouteWhileResizeAwaitsPainting(t *testing.T) {
	var stack workspaceScene
	probe := &presentationProbe{}
	dialog := newPresentationDialog(&stack, kit.DialogConfig{Body: probe.Element()})
	root := newSceneTestRoot(t, stack.Element())
	dialog.Show()
	root.Draw(grid.NewSurface(40, 8).View())
	root.InvalidateGeometry()
	if !root.Handle(input.Key{Code: input.Enter}) || probe.handled != 1 {
		t.Fatal("an unchanged presentation lost keyboard input while resize was pending")
	}
}

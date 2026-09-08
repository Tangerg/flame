package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func TestSessionCenterRejectsActionsAfterItsControllerCloses(t *testing.T) {
	favorites := 0
	center := newSessionCenterPane(kit.Dark(), kit.Unicode(), func(protocol.Session) {})
	center.toggleFavorite = func(protocol.Session) { favorites++ }
	if err := center.SetPage(protocol.Page[protocol.Session]{Data: []protocol.Session{{ID: "ses_1", Title: "One"}}}, false); err != nil {
		t.Fatal(err)
	}
	center.Focus(true)
	var stack headless.Stack
	dialog := newPresentationDialog(kit.DialogConfig{Stack: &stack, Theme: kit.Dark(), Glyphs: kit.Unicode(), Body: center})
	root := headless.NewRoot(&stack)
	surface := grid.NewSurface(80, 20)
	dialog.Show()
	root.Draw(surface.View())

	dialog.Dismiss()
	root.Handle(input.Key{Code: input.Character, Rune: 'f', Mods: input.Alt})
	if favorites != 0 {
		t.Fatalf("closed center executed %d favorite actions", favorites)
	}
	dialog.Show()
	root.Handle(input.Key{Code: input.Character, Rune: 'f', Mods: input.Alt})
	if favorites != 0 {
		t.Fatalf("undrawn replacement executed %d favorite actions", favorites)
	}
	root.Draw(surface.View())
	root.Handle(input.Key{Code: input.Character, Rune: 'f', Mods: input.Alt})
	if favorites != 1 {
		t.Fatalf("visible replacement executed %d favorite actions", favorites)
	}
}

func TestSessionCenterRejectsCursorCyclesAcrossUserLoadedPages(t *testing.T) {
	t.Parallel()
	center := newSessionCenterPane(kit.Theme{}, kit.Glyphs{}, func(protocol.Session) {})
	center.Reset()
	for _, page := range []protocol.Page[protocol.Session]{{NextCursor: "first"}, {NextCursor: "second"}} {
		if err := center.SetPage(page, center.Cursor() != ""); err != nil {
			t.Fatalf("SetPage(%q): %v", page.NextCursor, err)
		}
	}
	err := center.SetPage(protocol.Page[protocol.Session]{NextCursor: "first"}, true)
	if err == nil || !strings.Contains(err.Error(), "cyclic continuation cursor") {
		t.Fatalf("SetPage cycle error = %v", err)
	}
}

func TestSessionCenterCommandInterruptsAPendingPickerClick(t *testing.T) {
	opened, loaded := 0, 0
	center := newSessionCenterPane(kit.Dark(), kit.Unicode(), func(protocol.Session) { opened++ })
	center.loadMore = func() { loaded++ }
	if err := center.SetPage(protocol.Page[protocol.Session]{
		Data:       []protocol.Session{{ID: "one"}, {ID: "two"}},
		NextCursor: "next",
	}, false); err != nil {
		t.Fatal(err)
	}
	center.Focus(true)
	root := headless.NewRoot(center)
	root.Draw(grid.NewSurface(80, 20).View())
	second := pickerPoint(center.picker, 1)

	root.Handle(input.Mouse{Pos: second, Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Key{Code: input.Character, Rune: 'l', Mods: input.Alt})
	root.Handle(input.Mouse{Pos: second, Action: input.MouseUp, Button: input.ButtonLeft})

	if loaded != 1 {
		t.Fatalf("load-more command ran %d times, want 1", loaded)
	}
	if opened != 0 {
		t.Fatalf("release after load-more opened %d sessions", opened)
	}
}

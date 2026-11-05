package terminal

import (
	"image"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/uitest"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// sceneTestRoot admits and confirms each test drawing through the same two
// boundaries as the terminal writer. Embedding Root leaves Prepare/Abort and
// delayed Wrote available to tests of failed or stale presentation.
type sceneTestRoot struct {
	*scene.Root
	testing  testing.TB
	sequence uint64
}

func newSceneTestRoot(t testing.TB, element scene.Element) *sceneTestRoot {
	t.Helper()
	root, err := scene.NewRoot(scene.RootConfig{Element: element})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.Close)
	return &sceneTestRoot{Root: root, testing: t}
}

func (r *sceneTestRoot) Draw(view grid.View) {
	r.testing.Helper()
	r.Root.FlushNotifications()
	prepared, err := r.Root.Prepare(view)
	if err != nil {
		r.testing.Fatal(err)
	}
	r.sequence++
	prepared.Commit(r.sequence)
	r.Root.Wrote(r.sequence)
	r.Root.FlushNotifications()
}

func repaint(t testing.TB, host *uitest.Host) bool {
	t.Helper()
	_, err := host.Repaint(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return true
}

func drawRoot(t *testing.T, widget interface{ Element() scene.Element }, width, height int) string {
	t.Helper()
	surface := grid.NewSurface(width, height)
	root := newSceneTestRoot(t, widget.Element())
	defer root.Close()
	root.Draw(surface.View())
	return strings.Join(surface.Rows(), "\n")
}

func awaitValue[T any](t *testing.T, values <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for " + what)
		return *new(T)
	}
}

// drawnTextOrigin finds a cell a click can be aimed at. A pointer test aims at
// what the user sees, so it reads the drawn screen instead of asking the drawer
// to publish a rectangle whose one consumer would be this assertion.
func drawnTextOrigin(t testing.TB, surface *grid.Surface, drawn string) image.Point {
	t.Helper()
	rows := surface.Rows()
	for y, row := range rows {
		if index := strings.Index(row, drawn); index >= 0 {
			return image.Pt(utf8.RuneCountInString(row[:index]), y)
		}
	}
	t.Fatalf("%q was not drawn:\n%s", drawn, strings.Join(rows, "\n"))
	return image.Point{}
}

type staticDrawer interface {
	Draw(grid.View)
}

func drawStatic(t *testing.T, widget staticDrawer, width, height int) string {
	t.Helper()
	surface := grid.NewSurface(width, height)
	widget.Draw(surface.View())
	return strings.Join(surface.Rows(), "\n")
}

package terminal

import (
	"slices"

	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// workspaceScene composes the workbench and its currently presented dialogs.
// Attachment, focus restoration and historical input remain owned by scene.
type workspaceScene struct {
	group   *scene.Group
	preview func(scene.Event) scene.EventResult
}

func (w *workspaceScene) Element() scene.Element {
	if w.group == nil {
		w.group = scene.NewGroup(scene.GroupConfig{Overlay: true})
	}
	return w.group.Element()
}

func (w *workspaceScene) setWorkbench(element scene.Element) {
	w.Element()
	items := w.group.Items()
	item := scene.Item{Key: "workspace", Element: element, Size: layout.Flex(1)}
	if len(items) == 0 || items[0].Key != "workspace" {
		items = append([]scene.Item{item}, items...)
	} else {
		items[0] = item
	}
	if err := w.group.Set(items...); err != nil {
		panic(err)
	}
}

func (w *workspaceScene) hasModal() bool {
	w.Element()
	for _, item := range w.group.Items() {
		if item.Key != "workspace" {
			return true
		}
	}
	return false
}

func (w *workspaceScene) showDialog(element scene.Element) {
	w.Element()
	items := slices.DeleteFunc(w.group.Items(), func(item scene.Item) bool { return item.Element == element })
	items = append(items, scene.Item{Element: element, Size: layout.Flex(1)})
	if err := w.group.Set(items...); err != nil {
		panic(err)
	}
}

func (w *workspaceScene) dismissDialog(element scene.Element) {
	items := slices.DeleteFunc(w.group.Items(), func(item scene.Item) bool { return item.Element == element })
	if err := w.group.Set(items...); err != nil {
		panic(err)
	}
}

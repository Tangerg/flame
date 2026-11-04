package terminal

import (
	"errors"
	"image"
	"slices"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/kit"
)

const queueDrawerVisibleRows = 3

type queueTargetKind uint8

const (
	queueTargetNone queueTargetKind = iota
	queueTargetRow
	queueTargetSend
	queueTargetEdit
	queueTargetRemove
)

type queueTarget struct {
	kind queueTargetKind
	id   authoring.QueueEntryID
}

type queueHit struct {
	area   image.Rectangle
	target queueTarget
}

type queuePresentation struct {
	hits []queueHit
}

type queuePointerGesture struct {
	target  queueTarget
	active  bool
	dragged bool
}

func (q *queuePointerGesture) begin(target queueTarget) {
	*q = queuePointerGesture{target: target, active: true}
}

func (q *queuePointerGesture) drag(target queueTarget) bool {
	if !q.active {
		return false
	}
	if target != q.target {
		q.dragged = true
	}
	return true
}

func (q *queuePointerGesture) release(target queueTarget) bool {
	commit := q.active && !q.dragged && target.kind != queueTargetNone && target == q.target
	q.cancel()
	return commit
}

func (q *queuePointerGesture) cancel() { *q = queuePointerGesture{} }

type queueDrawerActions struct {
	BeginEdit  func(authoring.QueueEntry) error
	SaveEdit   func(authoring.QueueEntry, authoring.Message, bool) error
	CancelEdit func(authoring.QueueEntry) error
	Remove     func(authoring.QueueEntryID) error
	Move       func(authoring.QueueEntryID, int) error
	SendNow    func(authoring.QueueEntryID) error
	Dismiss    func()
}

// queueDrawer owns only the transient state of the bottom queue drawer. Queue
// entries and ordering remain in the application store, and every mutation is
// delegated to the app so runtime lifecycle rules stay outside the terminal view.
type queueDrawer struct {
	theme   kit.Theme
	glyphs  kit.Glyphs
	keys    *keymap.Map
	actions queueDrawerActions

	entries  []authoring.QueueEntry
	selected int
	scroll   base.Scroll

	editingEntry   *authoring.QueueEntry
	editingMessage authoring.Message
	editor         *kit.Composer

	hovered        queueTarget
	pointerGesture queuePointerGesture
	notice         string

	presentation scene.Snapshot[queuePresentation]
	group        *scene.Group
}

func newQueueDrawer(theme kit.Theme, glyphs kit.Glyphs, keys *keymap.Map, clipboard base.Clipboard) *queueDrawer {
	drawer := &queueDrawer{theme: theme, glyphs: glyphs, keys: keys}
	drawer.editor = kit.NewComposer(kit.ComposerConfig{Theme: theme, Prompt: glyphs.Marker + " ", MaxRows: 4})
	drawer.editor.Controller().Keys = keys
	drawer.editor.Controller().Clipboard = clipboard
	drawer.editor.Controller().Placeholder = "Edit queued prompt"
	return drawer
}

func (q *queueDrawer) Element() scene.Element {
	if q.group == nil {
		q.group = scene.NewGroup(scene.GroupConfig{
			Element: scene.ElementConfig{
				Focusable: func() bool { return !q.Editing() },
				Preview: func(event scene.Event) scene.EventResult {
					if _, ok := event.Input().(input.Mouse); ok {
						return scene.EventResult{}
					}
					return scene.EventResult{Consumed: q.Handle(event.Input())}
				},
				Handle: func(event scene.Event) scene.EventResult {
					mouse, ok := event.Input().(input.Mouse)
					if !ok {
						return scene.EventResult{}
					}
					if mouse.Action != input.MouseCancel && !event.CurrentLayout() {
						q.pointerGesture.cancel()
						return scene.EventResult{Consumed: true}
					}
					consumed := q.handleMouse(mouse)
					return scene.EventResult{Consumed: consumed, Capture: consumed && mouse.Action == input.MouseDown, Focus: consumed && mouse.Action == input.MouseDown}
				},
				Paint: q.Draw,
				FocusChanged: func(has bool) {
					if !has {
						q.hovered = queueTarget{}
						q.pointerGesture.cancel()
					}
				},
				Detached: func() { q.pointerGesture.cancel() },
			},
			Layout: func(space image.Point, items []scene.Item) []image.Rectangle {
				if len(items) == 0 {
					return nil
				}
				box := kit.Box{Theme: q.theme, Glyphs: q.glyphs, Padding: layout.Symmetric(0, 1)}
				inner := box.InnerRect(space)
				return []image.Rectangle{image.Rect(inner.Min.X, min(inner.Min.Y+1, inner.Max.Y), inner.Max.X, inner.Max.Y)}
			},
		})
		q.syncElements()
	}
	return q.group.Element()
}
func (q *queueDrawer) syncElements() {
	if q.group == nil {
		return
	}
	var items []scene.Item
	if q.Editing() {
		items = []scene.Item{{Element: q.editor.Element(), Size: layout.Flex(1)}}
	}
	if err := q.group.Set(items...); err != nil {
		panic(err)
	}
}

func (q *queueDrawer) SetActions(actions queueDrawerActions) { q.actions = actions }

func (q *queueDrawer) Set(snapshot authoring.QueueSnapshot) {
	q.hovered = queueTarget{}
	q.pointerGesture.cancel()
	selected, hasSelected := q.selectedEntry()
	entries := snapshot.Entries
	editingIndex := -1
	if q.Editing() {
		editingIndex = queueEntryIndex(entries, q.editingEntry.ID)
		if editingIndex >= 0 && entries[editingIndex].SessionID != q.editingEntry.SessionID {
			editingIndex = -1
		}
	}
	retainsEditor := editingIndex >= 0 && entries[editingIndex].Held &&
		entries[editingIndex].CommandID == q.editingEntry.CommandID
	// A sibling's transition does not replace the held editor or its input lease.
	if !retainsEditor {
		q.Element().Invalidate()
	}
	q.entries = entries
	if q.Editing() && editingIndex < 0 {
		if err := q.releaseEdit(); err != nil && !errors.Is(err, authoring.ErrEntryNotFound) {
			q.notice = err.Error()
		}
	}
	if len(entries) == 0 {
		q.selected = 0
		q.scroll.ToTop()
		return
	}
	q.selected = min(q.selected, len(entries)-1)
	if hasSelected {
		if index := queueEntryIndex(entries, selected.ID); index >= 0 {
			q.selected = index
		}
	}
}

func (q *queueDrawer) ResetNotice() { q.notice = "" }

func (q *queueDrawer) Editing() bool { return q.editingEntry != nil }

func (q *queueDrawer) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() {
		q.pointerGesture.cancel()
		return q.handleKey(key)
	}
	if mouse, ok := event.(input.Mouse); ok {
		return q.handleMouse(mouse)
	}
	if q.Editing() {
		return false
	}
	return false
}

func (q *queueDrawer) handleKey(key input.Key) bool {
	if q.Editing() {
		return q.handleEditKey(key)
	}
	if key.Is(input.Enter, input.Ctrl) {
		q.sendSelected()
		return true
	}
	if key.Code == input.Character {
		return q.handleCommandKey(key)
	}
	switch key.Code {
	case input.Up:
		q.moveSelection(-1)
		return true
	case input.Down:
		q.moveSelection(1)
		return true
	case input.Home:
		q.selectIndex(0)
		return true
	case input.End:
		q.selectIndex(len(q.entries) - 1)
		return true
	case input.Delete, input.Backspace:
		q.removeSelected()
		return true
	case input.Enter:
		q.beginEdit()
		return true
	default:
		return false
	}
}

func (q *queueDrawer) handleCommandKey(key input.Key) bool {
	if key.Mods != 0 && key.Mods != input.Shift {
		return false
	}
	switch key.Rune {
	case 'q':
		q.dismiss()
	case 'j':
		q.moveSelection(1)
	case 'k':
		q.moveSelection(-1)
	case 'J':
		q.moveSelected(1)
	case 'K':
		q.moveSelected(-1)
	case 'e':
		q.beginEdit()
	case 'x':
		q.removeSelected()
	case 's':
		q.sendSelected()
	default:
		return false
	}
	return true
}

func (q *queueDrawer) handleEditKey(key input.Key) bool {
	if key.Code == input.Esc {
		q.cancelEdit()
		return true
	}
	action, _ := q.keys.Action(key.Chord())
	if action == cancelRun && q.editor.Controller().Empty() {
		q.cancelEdit()
		return true
	}
	if key.Code == input.Enter {
		switch {
		case key.Mods == input.Ctrl:
			q.saveEdit(true)
		case action == insertNewline || key.Mods == input.Shift || key.Mods == input.Alt:
			q.editor.Controller().Insert("\n")
		default:
			q.saveEdit(false)
		}
		return true
	}
	return false
}

func (q *queueDrawer) handleMouse(mouse input.Mouse) bool {
	if q.Editing() {
		q.pointerGesture.cancel()
		return false
	}
	switch mouse.Action {
	case input.MouseCancel:
		q.pointerGesture.cancel()
		return true
	case input.MouseLeave:
		q.hovered = queueTarget{}
		return false
	case input.WheelUp:
		q.pointerGesture.cancel()
		q.moveSelection(-1)
		return true
	case input.WheelDown:
		q.pointerGesture.cancel()
		q.moveSelection(1)
		return true
	}
	target := q.hitAt(mouse.Pos)
	switch mouse.Action {
	case input.MouseMove:
		q.hovered = target
	case input.MouseDown:
		q.pointerGesture.cancel()
		if mouse.Button != input.ButtonLeft {
			return false
		}
		q.hovered = target
		if target.kind != queueTargetNone {
			q.pointerGesture.begin(target)
		}
	case input.MouseDrag:
		if mouse.Button != input.ButtonLeft || !q.pointerGesture.drag(target) {
			q.pointerGesture.cancel()
			return false
		}
		q.hovered = target
	case input.MouseUp:
		if mouse.Button != input.ButtonLeft {
			q.pointerGesture.cancel()
			return false
		}
		commit := q.pointerGesture.release(target)
		q.hovered = target
		if commit {
			q.activate(target)
		}
	}
	return true
}

func (q *queueDrawer) Closed() {
	_ = q.releaseEdit()
	q.hovered = queueTarget{}
	q.pointerGesture.cancel()
}

func (q *queueDrawer) selectedEntry() (authoring.QueueEntry, bool) {
	if q.selected < 0 || q.selected >= len(q.entries) {
		return authoring.QueueEntry{}, false
	}
	return q.entries[q.selected], true
}

func (q *queueDrawer) selectIndex(index int) {
	if len(q.entries) == 0 {
		return
	}
	q.selected = min(max(index, 0), len(q.entries)-1)
	q.hovered = queueTarget{}
}

func (q *queueDrawer) moveSelection(delta int) { q.selectIndex(q.selected + delta) }

func (q *queueDrawer) beginEdit() {
	entry, ok := q.selectedEntry()
	if !ok {
		return
	}
	if q.actions.BeginEdit != nil {
		if err := q.actions.BeginEdit(entry); err != nil {
			q.notice = err.Error()
			return
		}
	}
	q.editingEntry = new(entry)
	q.editingMessage = entry.Message.Clone()
	q.editor.Controller().SetText(entry.Message.Text)
	q.syncElements()
	_ = q.editor.Element().RequestFocusFirst()
	q.notice = ""
}

func (q *queueDrawer) cancelEdit() {
	if err := q.releaseEdit(); err != nil {
		q.notice = err.Error()
		return
	}
	q.notice = "edit discarded"
}

func (q *queueDrawer) releaseEdit() error {
	if !q.Editing() {
		return nil
	}
	entry := *q.editingEntry
	q.cancelEditState()
	if q.actions.CancelEdit == nil {
		return nil
	}
	return q.actions.CancelEdit(entry)
}

func (q *queueDrawer) cancelEditState() {
	q.editingEntry = nil
	q.editingMessage = authoring.Message{}
	q.editor.Controller().Clear()
	q.syncElements()
	_ = q.Element().RequestFocus()
}

func (q *queueDrawer) saveEdit(sendNow bool) {
	if !q.Editing() || q.actions.SaveEdit == nil {
		return
	}
	entry := *q.editingEntry
	message := q.editingMessage.Clone()
	message.Text = q.editor.Controller().Text()
	if err := q.actions.SaveEdit(entry, message, sendNow); err != nil {
		q.notice = err.Error()
		return
	}
	q.cancelEditState()
	q.notice = "queued prompt updated"
	if sendNow {
		q.notice = "queued prompt promoted for immediate send"
	}
}

func (q *queueDrawer) removeSelected() {
	entry, ok := q.selectedEntry()
	if !ok || q.actions.Remove == nil {
		return
	}
	if err := q.actions.Remove(entry.ID); err != nil {
		q.notice = err.Error()
		return
	}
	q.notice = "queued prompt removed"
}

func (q *queueDrawer) moveSelected(offset int) {
	entry, ok := q.selectedEntry()
	if !ok || q.actions.Move == nil {
		return
	}
	if err := q.actions.Move(entry.ID, offset); err != nil {
		if !errors.Is(err, authoring.ErrMoveUnavailable) {
			q.notice = err.Error()
		}
		return
	}
	q.notice = "queued prompt reordered"
}

func (q *queueDrawer) sendSelected() {
	entry, ok := q.selectedEntry()
	if ok {
		q.send(entry.ID)
	}
}

func (q *queueDrawer) send(id authoring.QueueEntryID) {
	if q.actions.SendNow == nil {
		return
	}
	if err := q.actions.SendNow(id); err != nil {
		q.notice = err.Error()
		return
	}
	q.notice = "queued prompt promoted for immediate send"
}

func (q *queueDrawer) dismiss() {
	if q.actions.Dismiss != nil {
		q.actions.Dismiss()
	}
}

func (q *queueDrawer) activate(target queueTarget) {
	if queueEntryIndex(q.entries, target.id) < 0 {
		return
	}
	q.selectID(target.id)
	switch target.kind {
	case queueTargetRow:
	case queueTargetSend:
		q.sendSelected()
	case queueTargetEdit:
		q.beginEdit()
	case queueTargetRemove:
		q.removeSelected()
	}
}

func (q *queueDrawer) selectID(id authoring.QueueEntryID) {
	if index := queueEntryIndex(q.entries, id); index >= 0 {
		q.selectIndex(index)
	}
}

func (q *queueDrawer) hitAt(point image.Point) queueTarget {
	hits := q.presentation.Value().hits
	for _, hit := range slices.Backward(hits) {
		if point.In(hit.area) {
			return hit.target
		}
	}
	return queueTarget{}
}

func queueEntryIndex(entries []authoring.QueueEntry, id authoring.QueueEntryID) int {
	return slices.IndexFunc(entries, func(entry authoring.QueueEntry) bool { return entry.ID == id })
}

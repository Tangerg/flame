package terminal

import (
	"errors"
	"image"
	"slices"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/queue"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
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
	id   queue.EntryID
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
	BeginEdit  func(queue.Entry) error
	SaveEdit   func(queue.Entry, prompt.Message, bool) error
	CancelEdit func(queue.Entry) error
	Remove     func(queue.EntryID) error
	Move       func(queue.EntryID, int) error
	SendNow    func(queue.EntryID) error
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

	entries  []queue.Entry
	selected int
	scroll   headless.Scroll

	editingEntry   *queue.Entry
	editingMessage prompt.Message
	editor         kit.Composer
	focused        bool

	hovered        queueTarget
	pointerGesture queuePointerGesture
	notice         string

	lifecycle    presentationLease
	presentation headless.Snapshot[queuePresentation]
	editorRegion headless.PointerRegion
}

func newQueueDrawer(theme kit.Theme, glyphs kit.Glyphs, keys *keymap.Map, clipboard headless.Clipboard) *queueDrawer {
	drawer := &queueDrawer{theme: theme, glyphs: glyphs, keys: keys}
	drawer.editor = kit.Composer{Theme: theme, Prompt: glyphs.Marker + " ", MaxRows: 4}
	drawer.editor.Editor().Keys = keys
	drawer.editor.Editor().Clipboard = clipboard
	drawer.editor.Editor().Placeholder = "Edit queued prompt"
	return drawer
}

func (q *queueDrawer) SetActions(actions queueDrawerActions) { q.actions = actions }

func (q *queueDrawer) Set(snapshot queue.Snapshot) {
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
		q.lifecycle.renew()
	}
	q.entries = entries
	if q.Editing() && editingIndex < 0 {
		if err := q.releaseEdit(); err != nil && !errors.Is(err, queue.ErrEntryNotFound) {
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
	if !q.lifecycle.acceptsInput() {
		return true
	}
	if key, ok := event.(input.Key); ok && key.Down() {
		q.pointerGesture.cancel()
		return q.handleKey(key)
	}
	if mouse, ok := event.(input.Mouse); ok {
		return q.handleMouse(mouse)
	}
	if q.Editing() {
		return q.editor.Handle(event)
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
	if action == cancelRun && q.editor.Editor().Empty() {
		q.cancelEdit()
		return true
	}
	if key.Code == input.Enter {
		switch {
		case key.Mods == input.Ctrl:
			q.saveEdit(true)
		case action == insertNewline || key.Mods == input.Shift || key.Mods == input.Alt:
			q.editor.Editor().Insert("\n")
		default:
			q.saveEdit(false)
		}
		return true
	}
	return q.editor.Handle(key)
}

func (q *queueDrawer) handleMouse(mouse input.Mouse) bool {
	if q.Editing() {
		q.pointerGesture.cancel()
		handled, delivered := q.editorRegion.Handle(mouse)
		return handled || delivered
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

func (q *queueDrawer) Focus(has bool) {
	q.focused = has
	if !has {
		q.hovered = queueTarget{}
		q.pointerGesture.cancel()
	}
	q.editor.Focus(has && q.Editing())
}

func (q *queueDrawer) Closed() {
	q.focused = false
	_ = q.releaseEdit()
	q.hovered = queueTarget{}
	q.pointerGesture.cancel()
}

func (q *queueDrawer) selectedEntry() (queue.Entry, bool) {
	if q.selected < 0 || q.selected >= len(q.entries) {
		return queue.Entry{}, false
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
	q.editor.Editor().SetText(entry.Message.Text)
	q.editor.Focus(q.focused)
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
	q.editingMessage = prompt.Message{}
	q.editor.Editor().Clear()
	q.editor.Focus(false)
}

func (q *queueDrawer) saveEdit(sendNow bool) {
	if !q.Editing() || q.actions.SaveEdit == nil {
		return
	}
	entry := *q.editingEntry
	message := q.editingMessage.Clone()
	message.Text = q.editor.Editor().Text()
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
		if !errors.Is(err, queue.ErrMoveUnavailable) {
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

func (q *queueDrawer) send(id queue.EntryID) {
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

func (q *queueDrawer) selectID(id queue.EntryID) {
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

func queueEntryIndex(entries []queue.Entry, id queue.EntryID) int {
	return slices.IndexFunc(entries, func(entry queue.Entry) bool { return entry.ID == id })
}

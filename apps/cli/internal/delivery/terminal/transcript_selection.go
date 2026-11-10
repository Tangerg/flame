package terminal

import (
	"strings"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

type transcriptSelection struct {
	Present    bool
	Readable   bool
	Expandable bool
	Expanded   bool
}

type transcriptPointerGesture struct {
	target  transcriptEntryID
	active  bool
	header  bool
	dragged bool
}

func (t *transcriptPointerGesture) begin(target transcriptEntryID, header bool) {
	*t = transcriptPointerGesture{target: target, active: true, header: header}
}

func (t *transcriptPointerGesture) drag() { t.dragged = true }

func (t *transcriptPointerGesture) release(
	selected transcriptEntryID,
	selectedPresent bool,
) (click, activate bool) {
	click = t.active && !t.dragged
	activate = click && selectedPresent && t.header && t.target == selected
	t.cancel()
	return click, activate
}

func (t *transcriptPointerGesture) cancel() { *t = transcriptPointerGesture{} }

func (t *transcriptView) handleMouse(mouse input.Mouse) {
	if mouse.Action == input.MouseCancel {
		t.pointerGesture.cancel()
		return
	}
	if mouse.Action == input.MouseLeave {
		return
	}
	if mouse.Button != input.ButtonLeft {
		t.cancelPointerGesture(mouse)
		return
	}
	switch mouse.Action {
	case input.MouseDown:
		point, _ := t.selection.Range()
		id, ok := t.nativeEntries[point.Content]
		offset := point.Position.Line
		if !ok {
			t.pointerGesture.cancel()
			return
		}
		if !t.selectPointerEntry(id) {
			t.pointerGesture.cancel()
			return
		}
		t.pointerGesture.begin(id, offset == 0 && t.tool(id) != nil)
	case input.MouseDrag:
		t.pointerGesture.drag()
	case input.MouseUp:
		click, activate := t.pointerGesture.release(t.selected, t.hasSelected)
		if click {
			t.selection.Clear()
		}
		if activate {
			t.toggleSelected()
		} else if !click {
			t.copySelection()
		}
	}
}

func (t *transcriptView) cancelPointerGesture(mouse input.Mouse) {
	switch mouse.Action {
	case input.MouseDown, input.MouseUp, input.MouseDrag, input.MouseCancel:
		t.pointerGesture.cancel()
	}
}

func (t *transcriptView) ensureSelection() {
	first := t.firstEntry()
	if t.hasSelected && t.selected >= first && t.selected < first+blockOffset(t.content.Len()) {
		return
	}
	if t.content.Len() == 0 {
		t.hasSelected = false
		return
	}
	t.selected = first + blockOffset(t.content.Len()-1)
	t.hasSelected = true
}

func (t *transcriptView) moveSelection(delta int) bool {
	if t.content.Len() == 0 || delta == 0 {
		return false
	}
	t.ensureSelection()
	first := t.firstEntry()
	last := first + blockOffset(t.content.Len()-1)
	next := t.selected
	switch {
	case delta < 0 && next > first:
		next--
	case delta > 0 && next < last:
		next++
	default:
		return true
	}
	t.selectEntry(next, true)
	return true
}

func (t *transcriptView) selectEdge(last bool) bool {
	if t.content.Len() == 0 {
		return false
	}
	id := t.firstEntry()
	if last {
		id += blockOffset(t.content.Len() - 1)
	}
	t.selectEntry(id, true)
	return true
}

func (t *transcriptView) selectEntry(id transcriptEntryID, reveal bool) {
	t.setSelectedEntry(id, reveal, true)
}

func (t *transcriptView) selectPointerEntry(id transcriptEntryID) bool {
	if _, ok := t.entries[id]; !ok {
		return false
	}
	t.setSelectedEntry(id, false, false)
	return true
}

func (t *transcriptView) setSelectedEntry(id transcriptEntryID, reveal, clearTextSelection bool) {
	if _, ok := t.entries[id]; !ok {
		return
	}
	t.selected, t.hasSelected = id, true
	if clearTextSelection {
		t.selectionIntent = nil
		t.selection.Clear()
	}
	t.syncSelectedEntry()
	if reveal {
		t.revealSelected()
	}
	t.announceSelection()
}

func (t *transcriptView) syncSelectedEntry() {
	t.Element().Invalidate()
	for id, entry := range t.entries {
		entry.selected = t.hasSelected && id == t.selected
		entry.focused = entry.selected && t.focused
	}
}

func (t *transcriptView) revealSelected() {
	if !t.hasSelected {
		return
	}
	top, width := 0, max(t.projected.Value().Width(), 1)
	for _, id := range t.order {
		body := t.entries[id].passive()
		if body == nil {
			continue
		}
		height := body.HeightForWidth(width)
		if id == t.selected {
			t.scroll.Reveal(top, top+max(height-1, 0))
			return
		}
		top += height
	}
}

func (t *transcriptView) tool(id transcriptEntryID) toolDisclosure {
	for _, tracked := range t.toolViews {
		if tracked.id == id {
			return tracked.block
		}
	}
	return nil
}

func (t *transcriptView) toggleSelected() bool {
	return t.mutateSelectedDisclosure(func(tool toolDisclosure) {
		tool.ToggleExpanded()
	})
}

func (t *transcriptView) setSelectedExpanded(expanded bool) bool {
	return t.mutateSelectedDisclosure(func(tool toolDisclosure) {
		tool.SetExpanded(expanded)
	})
}

// mutateSelectedDisclosure owns the layout invariant for keyboard-operated tool
// details. Both expansion and collapse can move the selected entry relative to
// the viewport, so every actual height change remeasures and reveals the same
// stable block identity before another command can target it.
func (t *transcriptView) mutateSelectedDisclosure(mutate func(toolDisclosure)) bool {
	tool := t.tool(t.selected)
	if !t.hasSelected || tool == nil || !tool.Expandable() {
		return true
	}
	before := tool.Expanded()
	mutate(tool)
	if tool.Expanded() != before {
		t.projectEntry(t.selected)
		t.revealSelected()
		t.refreshSearch()
	}
	t.announceSelection()
	return true
}

func (t *transcriptView) copySelected() bool {
	if !t.hasSelected {
		return true
	}
	layout := t.content.Layout()
	top, height, ok := layout.Extent(t.entries[t.selected].nativeID)
	if !ok {
		return true
	}
	t.copy(copyableRowsText(layout.Rows(top, height)))
	return true
}

func (t *transcriptView) copySelection() {
	t.restoreSelection()
	value, err := t.selection.Text(t.content.Layout())
	if err == nil {
		t.copy(value)
	}
}

func (t *transcriptView) copy(value string) {
	if value == "" {
		return
	}
	if t.clipboard == nil || !t.clipboard.Copy(value) {
		return
	}
	if t.onCopy != nil {
		t.onCopy(value)
	}
}

func (t *transcriptView) announceSelection() {
	if t.onSelection == nil {
		return
	}
	_, readable := t.readerTargetForSelected()
	selection := transcriptSelection{Present: t.hasSelected, Readable: readable}
	if tool := t.tool(t.selected); tool != nil && tool.Expandable() {
		selection.Expandable = true
		selection.Expanded = tool.Expanded()
	}
	t.onSelection(selection)
}

func (t *transcriptView) selectedReaderTarget() (readerTarget, bool) {
	if !t.hasSelected {
		t.ensureSelection()
	}
	return t.readerTargetForSelected()
}

func (t *transcriptView) readerTargetForSelected() (readerTarget, bool) {
	if !t.hasSelected {
		return readerTarget{}, false
	}
	entry := t.entries[t.selected]
	if entry == nil || entry.content == nil {
		return readerTarget{}, false
	}
	switch tool := entry.content.(type) {
	case *toolBlock:
		return readerTarget{document: tool.readerDocument(), source: tool}, true
	case *toolGroupBlock:
		return readerTarget{document: tool.readerDocument(), source: tool}, true
	}
	copyable, ok := entry.content.(text.Projector)
	if !ok {
		return readerTarget{}, false
	}
	width := max(t.content.Layout().Width()-transcriptEntryInset, 40)
	value := copyableRowsText(copyable.Rows(width))
	if strings.TrimSpace(value) == "" {
		return readerTarget{}, false
	}
	title := "Transcript entry"
	switch block := entry.content.(type) {
	case *markdownBlock:
		title = block.speaker
	case *userMessageBlock:
		if strings.TrimSpace(block.message.Label) != "" {
			title = block.message.Label
		}
	case *kit.Entry:
		if strings.TrimSpace(block.Label) != "" {
			title = block.Label
		}
	}
	return readerTarget{document: readerDocument{
		Title:    title,
		Sections: []toolSection{{Style: toolSectionCode, Text: value}},
	}}, true
}

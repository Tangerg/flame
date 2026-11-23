package terminal

import (
	"strings"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

type entryDisclosure uint8

const (
	disclosureFixed entryDisclosure = iota
	disclosureCollapsed
	disclosureExpanded
)

func disclosureOf(tool toolDisclosure) entryDisclosure {
	switch {
	case tool == nil || !tool.Expandable():
		return disclosureFixed
	case tool.Expanded():
		return disclosureExpanded
	default:
		return disclosureCollapsed
	}
}

// transcriptSelection is what prompt help shows about the selected entry; a
// nil selection means no entry is selected.
type transcriptSelection struct {
	disclosure entryDisclosure
	readable   bool
}

// A press on the first row of a tool entry toggles its details on click.
type transcriptPressTarget struct {
	entry  transcriptEntryID
	header bool
}

func (t *transcriptView) handleMouse(mouse input.Mouse) {
	if mouse.Action == input.MouseCancel {
		t.press.cancel()
		return
	}
	if mouse.Action == input.MouseLeave {
		return
	}
	if mouse.Button != input.ButtonLeft {
		t.cancelPress(mouse)
		return
	}
	switch mouse.Action {
	case input.MouseDown:
		point, _ := t.selection.Range()
		id, ok := t.nativeEntries[point.Content]
		offset := point.Position.Line
		if !ok || !t.selectPointerEntry(id) {
			t.press.cancel()
			return
		}
		t.press.begin(transcriptPressTarget{entry: id, header: offset == 0 && t.tool(id) != nil})
	case input.MouseDrag:
		t.press.drag()
	case input.MouseUp:
		pressed, click := t.press.release()
		if click {
			t.selection.Clear()
		}
		if click && pressed.header && t.isSelected(pressed.entry) {
			t.toggleSelected()
		} else if !click {
			t.copySelection()
		}
	}
}

func (t *transcriptView) cancelPress(mouse input.Mouse) {
	switch mouse.Action {
	case input.MouseDown, input.MouseUp, input.MouseDrag, input.MouseCancel:
		t.press.cancel()
	}
}

func (t *transcriptView) ensureSelection() {
	first := t.firstEntry()
	if t.selected != nil && *t.selected >= first && *t.selected < first+blockOffset(t.content.Len()) {
		return
	}
	if t.content.Len() == 0 {
		t.selected = nil
		return
	}
	t.selected = new(first + blockOffset(t.content.Len()-1))
}

func (t *transcriptView) isSelected(id transcriptEntryID) bool {
	return t.selected != nil && *t.selected == id
}

func (t *transcriptView) moveSelection(delta int) bool {
	if t.content.Len() == 0 || delta == 0 {
		return false
	}
	t.ensureSelection()
	first := t.firstEntry()
	last := first + blockOffset(t.content.Len()-1)
	next := *t.selected
	switch {
	case delta < 0 && next > first:
		next--
	case delta > 0 && next < last:
		next++
	default:
		return true
	}
	t.selectEntry(next)
	return true
}

func (t *transcriptView) selectFirst() bool { return t.selectAt(0) }

func (t *transcriptView) selectLast() bool { return t.selectAt(t.content.Len() - 1) }

func (t *transcriptView) selectAt(index int) bool {
	if t.content.Len() == 0 {
		return false
	}
	t.selectEntry(t.firstEntry() + blockOffset(index))
	return true
}

// selectionSource separates keyboard navigation, which reveals the entry and
// drops any text selection, from a pointer press that keeps both in place.
type selectionSource uint8

const (
	keyboardSelection selectionSource = iota + 1
	pointerSelection
)

func (t *transcriptView) selectEntry(id transcriptEntryID) {
	t.setSelectedEntry(id, keyboardSelection)
}

func (t *transcriptView) selectPointerEntry(id transcriptEntryID) bool {
	if _, ok := t.entries[id]; !ok {
		return false
	}
	t.setSelectedEntry(id, pointerSelection)
	return true
}

func (t *transcriptView) setSelectedEntry(id transcriptEntryID, source selectionSource) {
	if _, ok := t.entries[id]; !ok {
		return
	}
	t.selected = &id
	if source == keyboardSelection {
		t.selectionIntent = nil
		t.selection.Clear()
	}
	t.syncSelectedEntry()
	if source == keyboardSelection {
		t.revealSelected()
	}
	t.announceSelection()
}

func (t *transcriptView) syncSelectedEntry() {
	t.Element().Invalidate()
	for id, entry := range t.entries {
		switch {
		case !t.isSelected(id):
			entry.rail = railHidden
		case t.focused:
			entry.rail = railFocused
		default:
			entry.rail = railSelected
		}
	}
}

func (t *transcriptView) revealSelected() {
	if t.selected == nil {
		return
	}
	top, width := 0, max(t.projected.Value().Width(), 1)
	for _, id := range t.order {
		body := t.entries[id].passive()
		if body == nil {
			continue
		}
		height := body.HeightForWidth(width)
		if id == *t.selected {
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
	if t.selected == nil {
		return true
	}
	tool := t.tool(*t.selected)
	if tool == nil || !tool.Expandable() {
		return true
	}
	before := tool.Expanded()
	mutate(tool)
	if tool.Expanded() != before {
		t.projectEntry(*t.selected)
		t.revealSelected()
		t.refreshSearch()
	}
	t.announceSelection()
	return true
}

func (t *transcriptView) copySelected() bool {
	if t.selected == nil {
		return true
	}
	layout := t.content.Layout()
	top, height, ok := layout.Extent(t.entries[*t.selected].nativeID)
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
	if t.clipboard == nil || t.clipboard.Copy(value) != nil {
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
	if t.selected == nil {
		t.onSelection(nil)
		return
	}
	_, readable := t.readerTargetForSelected()
	t.onSelection(&transcriptSelection{disclosure: disclosureOf(t.tool(*t.selected)), readable: readable})
}

func (t *transcriptView) selectedReaderTarget() (readerTarget, bool) {
	if t.selected == nil {
		t.ensureSelection()
	}
	return t.readerTargetForSelected()
}

func (t *transcriptView) readerTargetForSelected() (readerTarget, bool) {
	if t.selected == nil {
		return readerTarget{}, false
	}
	entry := t.entries[*t.selected]
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

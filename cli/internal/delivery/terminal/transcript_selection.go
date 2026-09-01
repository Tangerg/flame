package terminal

import (
	"sort"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

type transcriptSelection struct {
	Present    bool
	Readable   bool
	Expandable bool
	Expanded   bool
}

type transcriptBlockPlacement struct {
	blockID     headless.BlockID
	top, height int
}

type transcriptBlockPresentation struct {
	lease  *transcriptContentLease
	blocks []transcriptBlockPlacement
}

type transcriptContentLease struct {
	retired bool
}

func newTranscriptContentLease() *transcriptContentLease {
	return &transcriptContentLease{}
}

func (l *transcriptContentLease) retire() {
	if l != nil {
		l.retired = true
	}
}

func (l *transcriptContentLease) current(candidate *transcriptContentLease) bool {
	return l != nil && l == candidate && !l.retired
}

type transcriptPointerGesture struct {
	target  headless.BlockID
	header  bool
	dragged bool
}

func (t *transcriptPointerGesture) begin(target headless.BlockID, header bool) {
	*t = transcriptPointerGesture{target: target, header: header}
}

func (t *transcriptPointerGesture) drag() { t.dragged = true }

func (t *transcriptPointerGesture) release(
	selected headless.BlockID,
	selectedPresent bool,
	selectionCollapsed bool,
) (click, activate bool) {
	click = !t.dragged && selectionCollapsed
	activate = click && selectedPresent && t.header && t.target == selected
	t.cancel()
	return click, activate
}

func (t *transcriptPointerGesture) cancel() { *t = transcriptPointerGesture{} }

func (t *transcriptView) handleMouse(mouse input.Mouse) {
	if mouse.Button != input.ButtonLeft {
		t.cancelPointerGesture(mouse)
		return
	}
	switch mouse.Action {
	case input.MouseDown:
		point, _ := t.selection.Range()
		presented := t.presentedBlocks.Value()
		if !t.contentLease.current(presented.lease) {
			// The transcript widget has already translated the press through its last
			// complete frame. Cancel the resulting selection when that frame belonged
			// to content Reset has replaced; BlockIDs restart from zero and must not
			// transfer a gesture to their new owners.
			t.selection.Clear()
			t.pointerGesture.cancel()
			return
		}
		id, offset, ok := presentedBlockAt(presented.blocks, point.Row)
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
		start, end := t.selection.Range()
		click, activate := t.pointerGesture.release(t.selected, t.hasSelected, start == end)
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
	case input.MouseDown, input.MouseUp, input.MouseDrag:
		t.pointerGesture.cancel()
	}
}

// projectBlockPlacements records stable block identities alongside the exact row
// layout being drawn. Semantic transcript content may change before the next frame;
// pointer input must still target what the last complete frame showed, not whatever
// now happens to occupy the same row number.
func (t *transcriptView) projectBlockPlacements(width int) []transcriptBlockPlacement {
	placements := make([]transcriptBlockPlacement, 0, t.content.Len())
	top := t.content.StartRow()
	first := t.content.FirstBlock()
	for index := range t.content.Len() {
		id := first + blockOffset(index)
		height := 0
		if width == t.content.Width() {
			_, height, _ = t.content.Extent(id)
		} else if entry := t.entries[id]; entry != nil && width > 0 {
			height = max(entry.Measure(width), 0)
		}
		placements = append(placements, transcriptBlockPlacement{blockID: id, top: top, height: height})
		top = layout.Sum(top, height)
	}
	return placements
}

func presentedBlockAt(placements []transcriptBlockPlacement, row int) (headless.BlockID, int, bool) {
	index := sort.Search(len(placements), func(index int) bool {
		return layout.Sum(placements[index].top, placements[index].height) > row
	})
	if index >= len(placements) || row < placements[index].top || placements[index].height <= 0 {
		return 0, 0, false
	}
	return placements[index].blockID, row - placements[index].top, true
}

func (t *transcriptView) ensureSelection() {
	first := t.content.FirstBlock()
	if t.hasSelected && t.selected >= first && t.selected < first+blockOffset(t.content.Len()) {
		return
	}
	if t.content.Len() == 0 {
		t.hasSelected = false
		return
	}
	t.selected = first + blockOffset(t.content.Len()-1)
	t.hasSelected = true
	t.revealSelected()
}

func (t *transcriptView) moveSelection(delta int) bool {
	if t.content.Len() == 0 || delta == 0 {
		return false
	}
	t.ensureSelection()
	first := t.content.FirstBlock()
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
	id := t.content.FirstBlock()
	if last {
		id += blockOffset(t.content.Len() - 1)
	}
	t.selectEntry(id, true)
	return true
}

func (t *transcriptView) selectEntry(id headless.BlockID, reveal bool) {
	t.setSelectedEntry(id, reveal, true)
}

func (t *transcriptView) selectPointerEntry(id headless.BlockID) bool {
	if _, ok := t.entries[id]; !ok {
		return false
	}
	t.setSelectedEntry(id, false, false)
	return true
}

func (t *transcriptView) setSelectedEntry(id headless.BlockID, reveal, clearTextSelection bool) {
	if _, ok := t.entries[id]; !ok {
		return
	}
	t.selected, t.hasSelected = id, true
	if clearTextSelection {
		t.selection.Clear()
	}
	t.syncSelectedEntry()
	if reveal {
		t.revealSelected()
	}
	t.announceSelection()
}

func (t *transcriptView) syncSelectedEntry() {
	for id, entry := range t.entries {
		entry.selected = t.hasSelected && id == t.selected
		entry.focused = entry.selected && t.focused
	}
}

func (t *transcriptView) revealSelected() {
	if !t.hasSelected {
		return
	}
	if top, height, ok := t.content.Extent(t.selected); ok {
		start := t.content.StartRow()
		t.scroll.Reveal(top-start, top-start+height-1)
	}
}

func (t *transcriptView) tool(id headless.BlockID) toolDisclosure {
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
		t.content.Changed(t.selected)
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
	top, height, ok := t.content.Extent(t.selected)
	if !ok {
		return true
	}
	t.copy(copyableRowsText(t.content.Rows(top, height)))
	return true
}

func (t *transcriptView) copySelection() {
	t.copy(t.selection.Text(&t.content))
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
	copyable, ok := entry.content.(headless.TextProjector)
	if !ok {
		return readerTarget{}, false
	}
	width := max(t.content.Width()-transcriptEntryInset, 40)
	value := copyableRowsText(copyable.Rows(width))
	if strings.TrimSpace(value) == "" {
		return readerTarget{}, false
	}
	title := "Transcript entry"
	switch block := entry.content.(type) {
	case *markdownBlock:
		title = block.speaker
	case *userMessageBlock:
		title = "you"
	case *kit.Entry:
		if strings.TrimSpace(block.Label) != "" {
			title = block.Label
		}
	}
	return readerTarget{document: readerDocument{
		Title:    title,
		Sections: []ToolSection{{Style: toolSectionCode, Language: "text", Text: value}},
	}}, true
}

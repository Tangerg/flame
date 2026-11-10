package terminal

import (
	"image"
	"slices"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

type transcriptReadingIntent struct {
	anchor                              transcriptEntryID
	anchorOffset, offset, width, height int
	follow                              bool
}

type transcriptSelectionPoint struct {
	entry    transcriptEntryID
	body     *transcriptEntry
	position text.Position
}

type transcriptSelectionIntent struct{ start, end transcriptSelectionPoint }

func snapshotTranscriptEntry(entry *transcriptEntry) *transcriptEntry {
	return newTranscriptEntry(entry.theme, entry.glyphs, snapshotTranscriptBlock(entry.content))
}

// Product presenters may keep observing Runtime facts. A reading window receives
// only detached passive values; a completed tool can therefore change disclosure
// without revising an immutable Oolong item behind its owner's back.
func snapshotTranscriptBlock(block base.Block) base.Block {
	switch value := block.(type) {
	case *toolBlock:
		copied := *value
		copied.call = value.call.Clone()
		copied.body = slices.Clone(value.body)
		copied.observers = readerDocumentObservers{}
		return &copied
	case *toolGroupBlock:
		copied := *value
		copied.observers = readerDocumentObservers{}
		copied.tools = make([]*toolBlock, len(value.tools))
		for index, tool := range value.tools {
			copied.tools[index] = snapshotTranscriptBlock(tool).(*toolBlock)
		}
		return &copied
	case *questionBlock:
		return newQuestionBlock(value.theme, value.glyphs, value.question)
	case *markdownBlock:
		copied := *value
		copied.doc.SetBlocks(value.doc.Blocks())
		copied.cancel, copied.images = nil, nil
		return &copied
	case *terminalImageBlock:
		projection := value.view()
		return &projection
	default:
		return block
	}
}

func (entry *transcriptEntry) passive() *transcriptEntry {
	if entry.document != nil {
		body, _ := entry.document.Current().Body.(*transcriptEntry)
		return body
	}
	return entry.projection
}

func (t *transcriptView) retainReadingIntent(preserve bool) transcriptReadingIntent {
	intent := transcriptReadingIntent{follow: true}
	if preserve && t.windowIntent != nil {
		intent = *t.windowIntent
	}
	if t.content != nil {
		previous := t.content.Layout()
		if preserve && previous.Width() > 0 {
			intent.width, intent.height = previous.Width(), t.viewport.Value()
			intent.follow, intent.offset = t.scroll.FollowingEnd(), t.scroll.Offset()
			if native, local, ok := previous.At(previous.StartRow() + intent.offset); ok {
				intent.anchor, intent.anchorOffset = t.nativeEntries[native], local
			}
			if t.selectionIntent == nil && t.selection.Active() {
				start, end := t.selection.Range()
				first, firstOK := previous.Block(start.Content).(*transcriptEntry)
				last, lastOK := previous.Block(end.Content).(*transcriptEntry)
				if firstOK && lastOK {
					t.selectionIntent = &transcriptSelectionIntent{
						start: transcriptSelectionPoint{entry: t.nativeEntries[start.Content], body: first, position: start.Position},
						end:   transcriptSelectionPoint{entry: t.nativeEntries[end.Content], body: last, position: end.Position},
					}
				}
			}
		}
	}
	return intent
}

func (t *transcriptView) resetWindow(preserve bool) {
	intent := t.retainReadingIntent(preserve)
	if t.content != nil {
		t.content.Close()
	}

	view, err := kit.NewTranscript(kit.TranscriptConfig{
		Behavior: base.TranscriptConfig{Key: "conversation-window", MaxItems: max(len(t.order), 1)}, Theme: t.theme, Glyphs: t.glyphs,
	})
	if err != nil {
		panic(err)
	}
	t.view, t.content = view, view.Controller()
	t.scroll, t.selection = t.content.Scroll(), t.content.Selection()
	t.scroll.Wheel(t.wheel)
	t.nativeEntries = make(map[base.BlockID]transcriptEntryID, len(t.order))
	for _, id := range t.order {
		entry := t.entries[id]
		var item *base.TranscriptItem
		if entry.document != nil {
			item, err = t.content.AddDocument(entry.document)
		} else {
			item, err = t.content.Add(entry.projection)
		}
		if err != nil {
			panic(err)
		}
		entry.nativeID = item.ID()
		t.nativeEntries[item.ID()] = id
	}
	t.restoreReadingIntent(intent)

	if !preserve {
		t.selectionIntent = nil
	}
	if t.selectionIntent != nil {
		intent := t.selectionIntent
		first, last := t.entries[intent.start.entry], t.entries[intent.end.entry]
		if first == nil || last == nil || first.passive() != intent.start.body || last.passive() != intent.end.body {
			t.selectionIntent = nil
		}
	}
	if err := view.Element().SetPaint(t.Draw); err != nil {
		panic(err)
	}
	if err := view.Element().SetFocusable(func() bool { return false }); err != nil {
		panic(err)
	}
	if err := t.group.Set(scene.Item{Element: view.Element()}); err != nil {
		panic(err)
	}
}

func (t *transcriptView) restoreReadingIntent(intent transcriptReadingIntent) {
	t.windowIntent = &intent
	if intent.follow {
		t.scroll.ToBottom()
	} else {
		t.scroll.ToTop()
		offset := intent.offset
		if intent.anchor != 0 {
			offset = 0
			for _, id := range t.order {
				if id == intent.anchor {
					offset += intent.anchorOffset
					break
				}
				if id > intent.anchor {
					break
				}
				if body := t.entries[id].passive(); body != nil {
					offset += body.HeightForWidth(intent.width)
				}
			}
		}
		t.scroll.Reveal(offset, offset+max(intent.height, 1)-1)
	}
}

func (t *transcriptView) projectSelection(layout base.TranscriptLayout, intent *transcriptSelectionIntent) (base.TextRange, bool) {
	project := func(saved transcriptSelectionPoint) (base.TextPoint, bool) {
		entry := t.entries[saved.entry]
		if entry == nil || entry.passive() != saved.body {
			return base.TextPoint{}, false
		}
		top, height, exists := layout.Extent(entry.nativeID)
		if !exists {
			return base.TextPoint{}, false
		}
		rows := layout.Rows(top, height)
		for index, row := range rows {
			selected, valid := layout.Hit(base.Point{Row: top + index, Col: row.Offset})
			if valid {
				selected.Start.Position = saved.position
				_, valid = layout.Caret(selected.Start)
				return selected.Start, valid
			}
		}
		return base.TextPoint{}, false
	}
	first, firstOK := project(intent.start)
	last, lastOK := project(intent.end)
	return base.TextRange{Start: first, End: last}, firstOK && lastOK
}

func (t *transcriptView) restoreSelection() {
	layout := t.content.Layout()
	if t.selectionIntent == nil || layout.Width() == 0 {
		return
	}
	if selected, valid := t.projectSelection(layout, t.selectionIntent); valid {
		t.selection.Set(selected)
	}
	t.selectionIntent = nil
}

func paintReadingSelection(view grid.View, from int, layout base.TranscriptLayout, selection *base.Selection, style grid.Style) {
	rectangles, err := selection.Rects(layout)
	if err != nil {
		return
	}
	for _, area := range rectangles {
		area = area.Sub(image.Pt(0, from)).Intersect(view.Visible())
		for y := area.Min.Y; y < area.Max.Y; y++ {
			for x := area.Min.X; x < area.Max.X; x++ {
				view.MergeStyle(x, y, style)
			}
		}
	}
}

func paintReadingMatches(view grid.View, from int, matches []base.Match, current int, theme kit.Theme) {
	for index, match := range matches {
		style := theme.Selection
		if index == current {
			style = theme.Accent
		}
		for row, span := range match.Spans {
			area := grid.Area(span.Col, match.Row+row-from, span.Width, 1).Intersect(view.Visible())
			for x := area.Min.X; x < area.Max.X; x++ {
				view.MergeStyle(x, area.Min.Y, style)
			}
		}
	}
}

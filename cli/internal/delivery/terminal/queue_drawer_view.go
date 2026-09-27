package terminal

import (
	"fmt"
	"image"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/queue"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

func (q *queueDrawer) Place(space image.Point) layout.Placement {
	if space.X <= 0 || space.Y <= 0 {
		return layout.Placement{}
	}
	innerRows := min(max(len(q.entries), 1), queueDrawerVisibleRows)
	if q.Editing() {
		innerRows = 6
	} else if entry, ok := q.selectedEntry(); ok {
		lines := strings.Split(entry.Message.Text, "\n")
		if len(lines) > 1 {
			innerRows += min(len(lines), 3) + 1
		}
	}
	return layout.Placement{Anchor: layout.Bottom, Height: min(innerRows+2, space.Y)}
}

func (q *queueDrawer) Draw(frame headless.Frame) {
	q.lifecycle.stage(frame)
	width, height := frame.Size()
	if width <= 0 || height <= 0 {
		q.presentation.Stage(frame, queuePresentation{})
		q.editorRegion.Stage(frame, image.Rectangle{}, nil)
		return
	}
	box := kit.Box{
		Theme: q.theme, Glyphs: q.glyphs, Padding: layout.Symmetric(0, 1),
		Title: q.title(), Footer: q.footer(), FooterAlign: layout.End,
	}
	inner := box.InnerRect(frame.Bounds().Size())
	box.Draw(frame.View)
	if q.Editing() {
		q.drawEditor(frame, inner)
		q.presentation.Stage(frame, queuePresentation{})
		return
	}
	q.editorRegion.Stage(frame, image.Rectangle{}, nil)
	hits := q.drawEntries(frame.Sub(inner))
	for index := range hits {
		hits[index].area = hits[index].area.Add(inner.Min)
	}
	q.presentation.Stage(frame, queuePresentation{hits: hits})
}

func (q *queueDrawer) title() string {
	return "Queue · " + countedNoun(len(q.entries), "prompt")
}

func (q *queueDrawer) footer() string {
	if q.notice != "" {
		return q.notice
	}
	if q.Editing() {
		return "enter save · shift/alt+enter newline · ctrl+enter send now · esc discard"
	}
	return "j/k move · enter edit · x remove · J/K reorder · s send now · esc close"
}

func (q *queueDrawer) drawEditor(frame headless.Frame, inner image.Rectangle) {
	if !q.Editing() || inner.Empty() {
		q.editorRegion.Stage(frame, image.Rectangle{}, nil)
		return
	}
	entry := *q.editingEntry
	rows := (layout.Flow{Axis: layout.Down}).Rects(inner.Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Flex(1)},
	})
	header, field := rows[0].Add(inner.Min), rows[1].Add(inner.Min)
	label := "Editing queued prompt"
	if attachments := len(entry.Message.Attachments); attachments > 0 {
		label += " · keeps " + countedNoun(attachments, "attachment")
	}
	frame.Text(header.Min.X, header.Min.Y, text.Truncate(label, header.Dx(), q.glyphs.Ellipsis), q.theme.Heading)
	q.editorRegion.Stage(frame, field, &q.editor)
	q.editor.Draw(frame.Sub(field))
}

func (q *queueDrawer) drawEntries(frame headless.Frame) []queueHit {
	width, height := frame.Size()
	if width <= 0 || height <= 0 {
		return nil
	}
	if len(q.entries) == 0 {
		frame.Text(0, 0, "No queued prompts.", q.theme.Muted)
		return nil
	}
	hits := make([]queueHit, 0, queueDrawerVisibleRows*4)
	y := q.drawSelectedPreview(frame.View)
	rows := min(queueDrawerVisibleRows, max(height-y, 0))
	// Queue actions must stay visible with their selected entry after a resize.
	// Refine Oolong's frame-local scroll instead of advancing selection in Draw.
	scroll := q.scroll.Stage(frame, len(q.entries), rows)
	scroll.Reveal(q.selected, q.selected)
	for row := range rows {
		index := scroll.Offset() + row
		if index >= len(q.entries) {
			break
		}
		area := grid.Area(0, y+row, width, 1)
		for _, hit := range q.drawEntry(frame.View.Sub(area), q.entries[index], index, index == q.selected) {
			hit.area = hit.area.Add(area.Min)
			hits = append(hits, hit)
		}
	}
	return hits
}

func (q *queueDrawer) drawSelectedPreview(view grid.View) int {
	width, height := view.Size()
	if selected, ok := q.selectedEntry(); ok {
		lines := strings.Split(selected.Message.Text, "\n")
		if len(lines) > 1 && height >= 4 {
			view.Text(0, 0, "Preview · full queued prompt", q.theme.Subtle.Merge(grid.Style{Attr: grid.Bold}))
			y := 1
			for _, line := range lines[:min(len(lines), 3)] {
				if y >= height-1 {
					break
				}
				view.Text(2, y, text.Truncate(cleanQueueText(line), max(width-2, 1), q.glyphs.Ellipsis), q.theme.Accent)
				y++
			}
			return y
		}
	}
	return 0
}

func (q *queueDrawer) drawEntry(view grid.View, entry queue.Entry, index int, selected bool) []queueHit {
	width, _ := view.Size()
	row := grid.Area(0, 0, width, 1)
	rowTarget := queueTarget{kind: queueTargetRow, id: entry.ID}
	style := q.theme.Text
	if selected || q.hovered.id == entry.ID {
		style = style.Merge(q.theme.Selection)
		view.Fill(row, q.theme.Selection)
	}
	if q.pointerGesture.target == rowTarget {
		style = style.Merge(grid.Style{Attr: grid.Bold | grid.Reverse})
	}
	hits := []queueHit{{area: row, target: rowTarget}}
	marker := " "
	if selected {
		marker = q.glyphs.Marker
	}
	prefix := fmt.Sprintf("%s %d. ", marker, index+1)
	right := width
	if width >= 40 && (selected || q.hovered.id == entry.ID || q.pointerGesture.target.id == entry.ID) {
		right = q.drawActions(view, row, entry.ID, right, style, &hits)
	}
	view.Text(0, 0, prefix, style.Merge(q.theme.Muted))
	left := text.Width(prefix)
	view.Text(left, 0, text.Truncate(queueEntryLabel(entry), max(right-left, 1), q.glyphs.Ellipsis), style)
	return hits
}

func (q *queueDrawer) drawActions(view grid.View, row image.Rectangle, id queue.EntryID, right int, rowStyle grid.Style, hits *[]queueHit) int {
	buttons := []struct {
		label  string
		target queueTarget
		style  grid.Style
	}{
		{label: "[remove]", target: queueTarget{kind: queueTargetRemove, id: id}, style: q.theme.Danger},
		{label: "[edit]", target: queueTarget{kind: queueTargetEdit, id: id}, style: q.theme.Accent},
		{label: "[send now]", target: queueTarget{kind: queueTargetSend, id: id}, style: q.theme.Success},
	}
	for _, button := range buttons {
		buttonWidth := text.Width(button.label)
		x := right - buttonWidth
		if x < 8 {
			continue
		}
		style := rowStyle.Merge(q.theme.Muted)
		if q.hovered == button.target {
			style = style.Merge(button.style)
		}
		if q.pointerGesture.target == button.target {
			style = style.Merge(grid.Style{Attr: grid.Bold | grid.Reverse})
		}
		view.Text(x, row.Min.Y, button.label, style)
		area := grid.Area(x, row.Min.Y, buttonWidth, 1)
		*hits = append(*hits, queueHit{area: area, target: button.target})
		right = x
	}
	return right
}

func cleanQueueText(value string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return ' '
		}
		return r
	}, value)
}

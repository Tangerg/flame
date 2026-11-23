package terminal

import (
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

type transcriptEntryID uint64

const transcriptEntryInset = 2

type selectionRail uint8

const (
	railHidden selectionRail = iota
	railSelected
	railFocused
)

type transcriptEntry struct {
	theme      kit.Theme
	glyphs     kit.Glyphs
	content    base.Block
	rail       selectionRail
	settled    bool
	projection *transcriptEntry
	document   *content.StreamDocument
	nativeID   base.BlockID
}

var (
	_ base.Block     = (*transcriptEntry)(nil)
	_ text.Projector = (*transcriptEntry)(nil)
)

func newTranscriptEntry(theme kit.Theme, glyphs kit.Glyphs, content base.Block) *transcriptEntry {
	return &transcriptEntry{theme: theme, glyphs: glyphs, content: content}
}

func (t *transcriptEntry) HeightForWidth(width int) int {
	contentWidth, _ := t.geometry(width)
	return t.content.HeightForWidth(contentWidth)
}

func (t *transcriptEntry) Draw(view grid.View) {
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	contentWidth, inset := t.geometry(width)
	if inset == 0 {
		t.content.Draw(view)
		return
	}
	t.content.Draw(view.Sub(grid.Area(inset, 0, contentWidth, height)))
	if t.rail == railHidden {
		return
	}
	style := t.theme.Subtle
	if t.rail == railFocused {
		style = t.theme.Accent
	}
	for row := range height {
		view.Text(0, row, t.glyphs.Vertical.String(), style)
	}
}

func (t *transcriptEntry) Rows(width int) []text.Row {
	height := t.HeightForWidth(width)
	rows := make([]text.Row, height)
	copyable, ok := t.content.(text.Projector)
	if !ok {
		return rows
	}
	contentWidth, inset := t.geometry(width)
	copied := copyable.Rows(contentWidth)
	for index := range min(height, len(copied)) {
		rows[index] = copied[index]
		rows[index].Offset = layout.Sum(rows[index].Offset, inset)
	}
	return rows
}

func (t *transcriptEntry) geometry(width int) (contentWidth, inset int) {
	if width <= transcriptEntryInset {
		return max(width, 1), 0
	}
	return width - transcriptEntryInset, transcriptEntryInset
}

func copyableRowsText(rows []text.Row) string {
	var copied strings.Builder
	for index, row := range rows {
		if index > 0 {
			copied.WriteString(row.Separator())
		}
		copied.WriteString(strings.TrimRight(row.Content.String(), " "))
	}
	return strings.TrimRight(copied.String(), "\n")
}

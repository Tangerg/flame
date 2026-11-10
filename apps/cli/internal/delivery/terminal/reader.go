package terminal

import (
	"context"
	"fmt"
	"github.com/Tangerg/oolong/ui/core/scene"
	"image"
	"strings"

	"github.com/Tangerg/oolong/extensions/highlight"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	readerFind     keymap.Action = "find"
	readerNext     keymap.Action = "next match"
	readerPrevious keymap.Action = "previous match"
	readerCopy     keymap.Action = "copy"
	readerClose    keymap.Action = "close"
)

type readerDocument struct {
	Title    string
	Detail   string
	Sections []toolSection
}

type readerDocumentSource interface {
	Observe(func(readerDocument)) func()
}

type readerTarget struct {
	document readerDocument
	source   readerDocumentSource
}

type readerSelectionGesture struct {
	active bool
}

func (r *readerSelectionGesture) begin() { r.active = true }

func (r *readerSelectionGesture) release() bool {
	owned := r.active
	r.cancel()
	return owned
}

func (r *readerSelectionGesture) cancel() { r.active = false }

// readerPane owns one modal reading session: full semantic content, search,
// selection, scrolling, and an optional live tool subscription.
type readerPane struct {
	theme     kit.Theme
	glyphs    kit.Glyphs
	syntax    highlight.Renderer
	wheel     input.Wheel
	clipboard base.Clipboard
	keys      *keymap.Map

	content          *base.Transcript
	scroll           *base.Scroll
	selection        *base.Selection
	view             *kit.Transcript
	group            *scene.Group
	body             *scene.Group
	blocks           []base.Block
	viewport         scene.Snapshot[image.Point]
	selectionGesture readerSelectionGesture
	search           *readingSearch
	matches          []base.Match
	current          int
	query            string
	problem          string
	refreshFailure   string
	title            string
	detail           string

	dismiss         func()
	openSearch      func()
	onCopied        func()
	releaseSource   func()
	observingSource bool
}

func newReaderPane(theme kit.Theme, glyphs kit.Glyphs, syntax highlight.Renderer, wheel input.Wheel, clipboard base.Clipboard) *readerPane {
	keys := base.DefaultScrollKeys()
	keys.Bind(readerFind, input.Ctrl.Rune('f'))
	keys.Bind(readerNext, input.Chord{Code: input.F3})
	keys.Bind(readerPrevious, input.Shift.With(input.F3))
	keys.Bind(readerCopy, input.Alt.Rune('c'))
	keys.Bind(readerClose, input.Chord{Code: input.Esc})
	r := &readerPane{
		theme: theme, glyphs: glyphs, syntax: syntax, wheel: wheel, clipboard: clipboard,
		keys: keys, search: newReadingSearch(), current: -1,
	}
	r.body = scene.NewGroup(scene.GroupConfig{Overlay: true, Element: scene.ElementConfig{Key: "reader-body", Focusable: func() bool { return true }, Preview: func(event scene.Event) scene.EventResult {
		if !event.CurrentLayout() {
			return scene.EventResult{}
		}
		handled := r.handleBody(event.Input())
		mouse, pointer := event.Input().(input.Mouse)
		return scene.EventResult{Consumed: handled, Capture: handled && pointer && mouse.Action == input.MouseDown, Focus: handled && pointer && mouse.Action == input.MouseDown}
	}, Handle: func(event scene.Event) scene.EventResult {
		if mouse, ok := event.Input().(input.Mouse); ok && mouse.Action == input.MouseCancel {
			r.handleBody(mouse)
		}
		return scene.EventResult{}
	}, FocusChanged: func(focused bool) {
		if !focused {
			r.selectionGesture.cancel()
		}
	}}})
	r.group = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Element: scene.ElementConfig{Key: "reader", Preview: func(event scene.Event) scene.EventResult {
		return scene.EventResult{Consumed: r.handleShortcut(event.Input())}
	}}, Items: []scene.Item{
		{Size: layout.Fixed(1), Element: scene.NewElement(scene.ElementConfig{Paint: r.drawHeading})},
		{Size: layout.Flex(1), Element: r.body.Element()},
	}})
	r.resetContent(nil, false, false)
	return r
}

func (r *readerPane) Open(target readerTarget) {
	r.CloseDocument()
	r.query, r.problem, r.matches, r.current = "", "", nil, -1
	r.search.Submit(nil, "", 0)
	if target.source != nil {
		r.observingSource = true
		initial := true
		r.releaseSource = target.source.Observe(func(document readerDocument) {
			follow := r.scroll.FollowingEnd()
			r.replace(document, !initial, follow)
			initial = false
		})
		return
	}
	r.replace(target.document, false, false)
}

func (r *readerPane) replace(document readerDocument, preserveScroll, follow bool) {
	r.refreshFailure = ""
	r.title = strings.TrimSpace(document.Title)
	if r.title == "" {
		r.title = "Transcript entry"
	}
	r.detail = strings.TrimSpace(document.Detail)
	var blocks []base.Block
	presentation := blockPresentation{Theme: r.theme, Glyphs: r.glyphs, Syntax: r.syntax}
	for _, section := range document.Sections {
		rendered := renderToolSections(presentation, []toolSection{section}, false)
		for _, block := range rendered {
			blocks = append(blocks, newReaderSectionBlock(r.theme, section.Title, block))
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, &kit.Entry{Theme: r.theme, Body: "No content is available for this entry."})
	}
	r.resetContent(blocks, preserveScroll, follow)
	if follow {
		r.scroll.ToBottom()
	}
	r.selection.Clear()
	if r.query != "" {
		r.search.Submit(r.searchRows(), r.query, 0)
	}
}

func (r *readerPane) resetContent(blocks []base.Block, preserveScroll, follow bool) {
	offset := 0
	if r.content != nil {
		r.interruptSelectionGesture()
		if preserveScroll {
			offset = r.scroll.Offset()
		}
		r.content.Close()
	}
	view, err := kit.NewTranscript(kit.TranscriptConfig{Behavior: base.TranscriptConfig{Key: "reader-window", MaxItems: max(len(blocks), 1), Keys: r.keys}, Theme: r.theme, Glyphs: r.glyphs})
	if err != nil {
		panic(err)
	}
	r.view, r.content, r.blocks = view, view.Controller(), blocks
	r.scroll, r.selection = r.content.Scroll(), r.content.Selection()
	r.scroll.Wheel(r.wheel)
	for _, block := range blocks {
		if _, err := r.content.Add(block); err != nil {
			panic(err)
		}
	}
	r.scroll.ToTop()
	if follow {
		r.scroll.ToBottom()
	} else if preserveScroll {
		r.scroll.Reveal(offset, offset+max(r.viewport.Value().Y, 1)-1)
	}
	if err := view.Element().SetPaint(r.drawBody); err != nil {
		panic(err)
	}
	if err := view.Element().SetFocusable(func() bool { return false }); err != nil {
		panic(err)
	}
	if err := r.body.Set(scene.Item{Element: view.Element()}); err != nil {
		panic(err)
	}
}

func (r *readerPane) Element() scene.Element { return r.group.Element() }

func (r *readerPane) drawHeading(frame scene.Frame) {
	kit.Label{Text: r.title, Style: r.theme.Strong, Ellipsis: r.glyphs.Ellipsis}.Draw(frame.View)
	if r.detail != "" {
		kit.Label{Text: r.detail, Style: r.theme.Subtle, Align: layout.End, Ellipsis: r.glyphs.Ellipsis}.Draw(frame.View)
	}
}

func (r *readerPane) drawBody(frame scene.Frame) {
	width, height := frame.Size()
	body := frame.Sub(grid.Area(0, 0, width, max(height-1, 0)))
	r.viewport.Stage(frame, body.Bounds().Size())
	window := r.content.StageWindow(body)
	window.Content.Draw(window.Body, window.BodyFrom)
	paintReadingSelection(window.Body, window.BodyFrom, window.Content, r.selection, r.theme.Selection)
	result, err := base.Scan(context.Background(), base.ScanConfig{Query: r.query, Rows: window.Content.Rows(window.Content.StartRow(), window.Content.Height()), StartRow: window.Content.StartRow()})
	if err == nil {
		paintReadingMatches(window.Body, window.BodyFrom, result.Matches, r.current, r.theme)
	}
	footer := frame.Sub(grid.Area(0, max(height-1, 0), width, min(height, 1)))
	kit.Label{Text: r.footerAt(window.Content.Height(), window.BodyFrom-window.Content.StartRow(), max(height-1, 1)), Style: r.theme.Subtle, Align: layout.End, Ellipsis: r.glyphs.Ellipsis}.Draw(footer.View)
}

func (r *readerPane) searchRows() []text.Row {
	width := max(r.viewport.Value().X, 1)
	var rows []text.Row
	for _, block := range r.blocks {
		if body, ok := block.(text.Projector); ok {
			rows = append(rows, body.Rows(width)...)
		} else {
			rows = append(rows, make([]text.Row, block.HeightForWidth(width))...)
		}
	}
	return rows
}

func (r *readerPane) footerAt(total, offset, window int) string {
	if r.refreshFailure != "" {
		return r.refreshFailure
	}
	if r.problem != "" {
		return r.problem
	}
	if total <= 0 {
		return "esc close"
	}
	first := min(offset+1, total)
	last := min(offset+window, total)
	percent := min((last*100)/total, 100)
	search := ""
	if r.query != "" {
		search = fmt.Sprintf(" · %d matches", len(r.matches))
	}
	return fmt.Sprintf("rows %d-%d/%d · %d%%%s · ctrl+f search · alt+c copy · esc close", first, last, total, percent, search)
}

func (r *readerPane) RefreshFailed(message string) {
	r.refreshFailure = message
}

func (r *readerPane) Handle(event input.Event) bool {
	if r.handleShortcut(event) {
		return true
	}
	if mouse, ok := event.(input.Mouse); ok {
		mouse.Pos.Y--
		return r.handleBody(mouse)
	}
	return r.handleBody(event)
}

func (r *readerPane) handleBody(event input.Event) bool {
	switch event.(type) {
	case input.Paste, input.FocusOut:
		r.interruptSelectionGesture()
	}
	handled := r.content.Handle(event)
	if mouse, ok := event.(input.Mouse); ok {
		bounds := grid.Area(0, 0, r.viewport.Value().X, r.viewport.Value().Y)
		delivered := !bounds.Empty()
		if mouse.Action == input.MouseDown {
			delivered = mouse.Pos.In(bounds)
		}
		r.handleMouse(mouse, handled, delivered)
	}
	return handled
}

func (r *readerPane) handleShortcut(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	r.interruptSelectionGesture()
	action, _ := r.keys.Action(key.Chord())
	switch action {
	case readerFind:
		if r.openSearch != nil {
			r.openSearch()
		}
		return true
	case readerNext:
		r.StepMatch(1)
		return true
	case readerPrevious:
		r.StepMatch(-1)
		return true
	case readerCopy:
		r.copy()
		return true
	case readerClose:
		if r.dismiss != nil {
			r.dismiss()
		}
		return true
	default:
		return false
	}
}

func (r *readerPane) handleMouse(mouse input.Mouse, handled, delivered bool) {
	switch mouse.Action {
	case input.MouseCancel:
		r.interruptSelectionGesture()
	case input.MouseDown:
		if mouse.Button == input.ButtonLeft && handled && delivered {
			r.selectionGesture.begin()
		} else {
			r.interruptSelectionGesture()
		}
	case input.MouseDrag:
		if mouse.Button != input.ButtonLeft || !delivered {
			r.interruptSelectionGesture()
		}
	case input.MouseUp:
		owned := mouse.Button == input.ButtonLeft && r.selectionGesture.release()
		if mouse.Button != input.ButtonLeft {
			r.interruptSelectionGesture()
		}
		if owned && delivered && r.selection.Active() {
			r.copy()
		}
	case input.WheelUp, input.WheelDown:
		r.interruptSelectionGesture()
	}
}

func (r *readerPane) interruptSelectionGesture() {
	r.selectionGesture.cancel()
	r.selection.Done()
}

func (r *readerPane) Closed() { r.CloseDocument() }

func (r *readerPane) Find(query string) {
	r.query = strings.TrimSpace(query)
	r.problem, r.matches, r.current = "", nil, -1
	r.search.Submit(r.searchRows(), r.query, 0)
}

func (r *readerPane) AcceptSearch(result readingSearchResult) bool {
	if !r.search.Current(result) {
		return false
	}
	if result.Err != nil {
		r.problem = "search: " + result.Err.Error()
		r.matches, r.current = nil, -1
		return true
	}
	r.problem = ""
	navigate := r.current < 0
	r.matches = result.Matches
	r.current = min(r.current, len(r.matches)-1)
	if len(r.matches) > 0 {
		r.current = max(r.current, 0)
	}
	if navigate {
		r.revealMatch()
	}
	return true
}

func (r *readerPane) StepMatch(delta int) bool {
	if len(r.matches) == 0 {
		return false
	}
	r.current = (r.current + delta) % len(r.matches)
	if r.current < 0 {
		r.current += len(r.matches)
	}
	return r.revealMatch()
}

func (r *readerPane) revealMatch() bool {
	if r.current < 0 || r.current >= len(r.matches) {
		return false
	}
	match := r.matches[r.current]
	r.scroll.Reveal(match.Row, match.Row+max(len(match.Spans)-1, 0))
	return true
}

func (r *readerPane) SearchResults() <-chan readingSearchResult { return r.search.Results() }

func (r *readerPane) ObservingSource() bool {
	return r != nil && r.observingSource
}

func (r *readerPane) copy() {
	layout := r.content.Layout()
	value, err := r.selection.Text(layout)
	if err != nil {
		value = ""
	}
	if value == "" {
		value = copyableRowsText(layout.Rows(layout.StartRow(), layout.Height()))
	}
	if value == "" || r.clipboard == nil || !r.clipboard.Copy(value) {
		return
	}
	if r.onCopied != nil {
		r.onCopied()
	}
}

func (r *readerPane) CloseDocument() {
	r.interruptSelectionGesture()
	release := r.releaseSource
	r.releaseSource = nil
	r.observingSource = false
	if release != nil {
		release()
	}
}

func (r *readerPane) Shutdown() {
	r.CloseDocument()
	r.search.Close()
	r.content.Close()
}

type readerSectionBlock struct {
	theme   kit.Theme
	title   string
	content base.Block
}

var _ text.Projector = (*readerSectionBlock)(nil)

func newReaderSectionBlock(theme kit.Theme, title string, content base.Block) *readerSectionBlock {
	return &readerSectionBlock{theme: theme, title: strings.TrimSpace(title), content: content}
}

func (r *readerSectionBlock) HeightForWidth(width int) int {
	return layout.Sum(r.headingRows(), r.content.HeightForWidth(width), 1)
}

func (r *readerSectionBlock) Draw(view grid.View) {
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	y := r.headingRows()
	if y > 0 {
		view.Text(0, 0, r.title, r.theme.Strong)
	}
	r.content.Draw(view.Sub(grid.Area(0, y, width, max(height-y-1, 0))))
}

func (r *readerSectionBlock) Rows(width int) []text.Row {
	rows := make([]text.Row, 0, r.HeightForWidth(width))
	if r.title != "" {
		rows = append(rows, text.Row{Text: r.title})
	}
	if copyable, ok := r.content.(text.Projector); ok {
		rows = append(rows, copyable.Rows(width)...)
	} else {
		rows = append(rows, make([]text.Row, r.content.HeightForWidth(width))...)
	}
	return append(rows, text.Row{})
}

func (r *readerSectionBlock) headingRows() int {
	if r.title == "" {
		return 0
	}
	return 1
}

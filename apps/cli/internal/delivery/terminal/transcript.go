package terminal

import (
	"errors"
	"fmt"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/scene"
	"io"
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/extensions/highlight"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/kit"
)

type transcriptView struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	wheel  input.Wheel
	look   markdown.Look
	syntax highlight.Renderer

	content          *base.Transcript
	scroll           *base.Scroll
	selection        *base.Selection
	view             *kit.Transcript
	group            *scene.Group
	order            []transcriptEntryID
	nextEntry        transcriptEntryID
	selectionIntent  *transcriptSelectionIntent
	nativeEntries    map[base.BlockID]transcriptEntryID
	projected        scene.Snapshot[base.TranscriptLayout]
	viewport         scene.Snapshot[int]
	windowIntent     *transcriptReadingIntent
	search           transcriptSearch
	retain           int
	details          bool
	clipboard        base.Clipboard
	entrance         grid.Drawable
	focused          bool
	selected         transcriptEntryID
	hasSelected      bool
	entries          map[transcriptEntryID]*transcriptEntry
	pointerGesture   transcriptPointerGesture
	onFocusChange    func(bool)
	onSelection      func(transcriptSelection)
	onCopy           func(string)
	keys             *keymap.Map
	matcher          keymap.Matcher
	tools            map[string]liveTool
	textStreams      map[string]*liveText
	pendingQuestions map[string]trackedQuestion
	toolViews        []trackedToolView
	history          transcriptHistory
	activeToolGroup  *trackedToolGroup
	images           *terminalImagePresenter
	media            *markdownMedia
	onRenderError    func(error)
	releaseErr       error
}

const transcriptPrompt keymap.Action = "prompt"

type liveTool struct {
	runID  string
	ids    []transcriptEntryID
	blocks []trackedTool
	group  *trackedToolGroup
}

type liveText struct {
	runID    string
	kind     conversation.BlockKind
	document *content.StreamDocument
	block    *markdownBlock
	id       transcriptEntryID
}

type trackedTool struct {
	id    transcriptEntryID
	block mutableToolBlock
}

type trackedToolView struct {
	id    transcriptEntryID
	block toolDisclosure
}

type trackedToolGroup struct {
	id    transcriptEntryID
	runID string
	block *toolGroupBlock
}

func (t *transcriptView) ToggleDetails() {
	first := t.firstEntry()
	// #nosec G115 -- Transcript.Len is non-negative and cannot exceed the
	// addressable in-memory slice backing the transcript.
	end := first + transcriptEntryID(t.content.Len())
	expand, hasTool := false, false
	for _, tracked := range t.toolViews {
		if tracked.id < first || tracked.id >= end || !tracked.block.Expandable() {
			continue
		}
		hasTool = true
		if !tracked.block.Expanded() {
			expand = true
			break
		}
	}
	if !hasTool {
		expand = !t.details
	}
	t.details = expand
	selectedChanged := false
	for _, tracked := range t.toolViews {
		if tracked.id < first || tracked.id >= end || !tracked.block.Expandable() {
			continue
		}
		before := tracked.block.Expanded()
		tracked.block.SetExpanded(t.details)
		if tracked.block.Expanded() == before {
			continue
		}
		selectedChanged = selectedChanged || t.focused && tracked.id == t.selected
		t.projectEntry(tracked.id)
	}
	if selectedChanged {
		t.revealSelected()
	}
	t.refreshSearch()
	t.announceSelection()
}

func (t *transcriptView) DetailsLabel() string {
	if t.details {
		return "tool details expanded"
	}
	return "tool details collapsed"
}

func newTranscriptView(
	theme kit.Theme,
	glyphs kit.Glyphs,
	locale string,
	wheel input.Wheel,
	syntax highlight.Renderer,
	retain int,
	details bool,
	clipboard base.Clipboard,
) *transcriptView {
	c := &transcriptView{
		theme: theme, glyphs: glyphs, wheel: wheel,
		look: markdownLook(theme, glyphs, locale, syntax), syntax: syntax,
		search: newTranscriptSearch(), retain: max(retain, 4), details: details,
		clipboard: clipboard, entries: make(map[transcriptEntryID]*transcriptEntry),
		tools: make(map[string]liveTool), textStreams: make(map[string]*liveText),
		pendingQuestions: make(map[string]trackedQuestion),
		history:          newTranscriptHistory(),
		keys:             transcriptKeys(),
	}
	c.group = scene.NewGroup(scene.GroupConfig{Overlay: true, Element: scene.ElementConfig{
		Key: "conversation", Focusable: func() bool { return true },
		FocusChanged: c.Focus,
		Preview: func(event scene.Event) scene.EventResult {
			if _, mouse := event.Input().(input.Mouse); mouse && !event.CurrentLayout() {
				return scene.EventResult{}
			}
			consumed := c.Handle(event.Input())
			mouse, pointer := event.Input().(input.Mouse)
			return scene.EventResult{Consumed: consumed, Capture: consumed && pointer && mouse.Action == input.MouseDown, Focus: consumed && pointer && mouse.Action == input.MouseDown}
		},
		Handle: func(event scene.Event) scene.EventResult {
			if mouse, ok := event.Input().(input.Mouse); ok && mouse.Action == input.MouseCancel {
				c.Handle(mouse)
			}
			return scene.EventResult{}
		},
	}})
	c.resetWindow(false)

	return c
}

func (t *transcriptView) Element() scene.Element { return t.group.Element() }

func (t *transcriptView) Draw(frame scene.Frame) {
	window := t.content.StageWindow(frame)
	_, viewportHeight := frame.Size()
	t.viewport.Stage(frame, viewportHeight)
	t.projected.Stage(frame, window.Content)
	window.Content.Draw(window.Body, window.BodyFrom)
	selection := t.selection
	if intent := t.selectionIntent; intent != nil {
		if restored, ok := t.projectSelection(window.Content, intent); ok {
			projected := &base.Selection{}
			projected.Set(restored)
			selection = projected
		}
	}
	paintReadingSelection(window.Body, window.BodyFrom, window.Content, selection, t.theme.Selection)
	matches, current := t.search.presentationFor(window.Content)
	paintReadingMatches(window.Body, window.BodyFrom, matches, current, t.theme)
	if t.hasSelected {
		if entry := t.entries[t.selected]; entry != nil {
			top, height, ok := window.Content.Extent(entry.nativeID)
			if ok && window.Content.Width() > transcriptEntryInset {
				style := t.theme.Subtle
				if t.focused {
					style = t.theme.Accent
				}
				visible := window.Body.Visible()
				for row := max(top-window.BodyFrom, visible.Min.Y); row < min(top+height-window.BodyFrom, visible.Max.Y); row++ {
					window.Body.Text(0, row, t.glyphs.Vertical, style)
				}
			}
		}
	}
	if t.content.Len() == 0 && t.entrance != nil {
		t.entrance.Draw(frame.View)
	}
}

// SetEntrance installs a presentation-only projection that is consumed by the
// first transcript block or reset. It is not part of retained transcript state.
func (t *transcriptView) SetEntrance(entrance grid.Drawable) {
	t.entrance = entrance
	t.Element().Invalidate()
}

func (t *transcriptView) Handle(event input.Event) bool {
	if t.handleKey(event) {
		return true
	}
	t.restoreSelection()
	handled := t.content.Handle(event)
	mouse, ok := event.(input.Mouse)
	if !ok {
		return handled
	}
	if !handled {
		t.cancelPointerGesture(mouse)
		return false
	}
	t.handleMouse(mouse)
	return true
}

func (t *transcriptView) handleKey(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	t.pointerGesture.cancel()
	if !t.focused {
		return false
	}
	if key.Code == input.Esc && (t.selection.Active() || t.selectionIntent != nil) {
		t.selectionIntent = nil
		t.selection.Clear()
		return true
	}
	_, handled := t.matcher.Handle(t.keys, key, func() bool { return t.focused }, t.Do)
	return handled
}

func (t *transcriptView) Focus(has bool) {
	if t.focused == has {
		return
	}
	t.focused = has
	if !has {
		t.matcher.Clear()
		t.pointerGesture.cancel()
	}
	if has {
		t.ensureSelection()
	}
	t.syncSelectedEntry()
	if t.onFocusChange != nil {
		t.onFocusChange(has)
	}
	t.announceSelection()
}

func (t *transcriptView) Focused() bool { return t.focused }

func (t *transcriptView) OnFocusChange(change func(bool)) { t.onFocusChange = change }

func (t *transcriptView) OnSelection(change func(transcriptSelection)) { t.onSelection = change }

func (t *transcriptView) OnCopy(copied func(string)) { t.onCopy = copied }

func (t *transcriptView) Keys() *keymap.Map { return t.keys }

func (t *transcriptView) action(event input.Event) keymap.Action {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return ""
	}
	action, _ := t.keys.Action(key.Chord())
	return action
}

func (t *transcriptView) Do(action keymap.Action) bool {
	switch action {
	case base.SelectPrev:
		return t.moveSelection(-1)
	case base.SelectNext:
		return t.moveSelection(1)
	case base.SelectFirst:
		return t.selectEdge(false)
	case base.SelectLast:
		return t.selectEdge(true)
	case base.Collapse:
		return t.setSelectedExpanded(false)
	case base.Expand:
		return t.setSelectedExpanded(true)
	case toggleDetails:
		return t.toggleSelected()
	case base.Copy:
		return t.copySelected()
	}
	return false
}

func transcriptKeys() *keymap.Map {
	keys := &keymap.Map{}
	keys.Bind(base.SelectPrev, input.Chord{Code: input.Up})
	keys.Bind(base.SelectNext, input.Chord{Code: input.Down})
	keys.Bind(base.SelectFirst, input.Chord{Code: input.Home})
	keys.Bind(base.SelectLast, input.Chord{Code: input.End})
	keys.Bind(base.Expand, input.Chord{Code: input.Right})
	keys.Bind(base.Collapse, input.Chord{Code: input.Left})
	keys.Bind(toggleDetails, input.Chord{Code: input.Enter})
	keys.Bind(base.Copy, input.Alt.Rune('c'))
	keys.Bind(openReader, input.Chord{Code: input.Character, Rune: 'v'})
	keys.Bind(transcriptPrompt, input.Chord{Code: input.Tab})
	keys.Bind(transcriptPrompt, input.Chord{Code: input.Character, Rune: ' '})
	keys.Bind(commandPalette, input.Chord{Code: input.Character, Rune: '?'})
	return keys
}

func (t *transcriptView) Follow() { t.scroll.ToBottom() }

func (t *transcriptView) Scroll(action keymap.Action) bool { return t.scroll.Do(action) }

func (t *transcriptView) Close() {
	if t != nil {
		if t.media != nil {
			t.media.workers.Close()
		}
		t.Reset()
		t.content.Close()
		t.search.Close()
	}
}

func (t *transcriptView) releaseEntry(entry *transcriptEntry) {
	if closer, ok := entry.content.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			err = fmt.Errorf("release transcript content: %w", err)
			t.releaseErr = errors.Join(t.releaseErr, err)
			if t.onRenderError != nil {
				t.onRenderError(err)
			}
		}
	}
}

func (t *transcriptView) finishMarkdown(id transcriptEntryID, block *markdownBlock) {
	if entry := t.entries[id]; entry != nil {
		entry.content, entry.settled = block, false
	}
	if t.media != nil && t.media.start(block, func() {
		t.projectEntry(id)
		t.settleEntry(id)
		t.refreshSearch()
	}) {
		t.projectEntry(id)
		return
	}
	t.projectEntry(id)
	t.settleEntry(id)
}

func (t *transcriptView) mutateTrackedTool(tracked trackedTool, mutate func(mutableToolBlock)) bool {
	before := tracked.block.Expanded()
	mutate(tracked.block)
	t.projectEntry(tracked.id)
	return t.focused && tracked.id == t.selected && before && !tracked.block.Expanded()
}

func (t *transcriptView) appendCompleted(block conversation.Block) error {
	rendered, err := t.present(block)
	if err != nil {
		return err
	}
	if block.Kind == conversation.BlockTool {
		if tool, grouped := groupedTool(rendered); grouped {
			t.addGroupedTool(block.RunID, tool)
			t.refreshSearch()
			return nil
		}
	}
	t.sealToolGroup()
	for _, item := range rendered {
		mutable, isMutable := item.(mutableToolBlock)
		if isMutable {
			mutable.SetExpanded(t.details)
		}
		question, isPendingQuestion := item.(*questionBlock)
		isPendingQuestion = isPendingQuestion && !question.answered()
		key := ""
		if isPendingQuestion {
			key = conversation.BlockKey(block.RunID, block.ID)
			if _, exists := t.pendingQuestions[key]; exists {
				return fmt.Errorf("terminal transcript: question block %s completed twice", block.ID)
			}
		}
		id := t.place(item, !isPendingQuestion)
		t.history.Append(block.RunID, id)
		if isPendingQuestion {
			t.pendingQuestions[key] = trackedQuestion{runID: block.RunID, id: id, block: question}
		}
		if isMutable {
			t.toolViews = append(t.toolViews, trackedToolView{id: id, block: mutable})
		}
	}
	t.refreshSearch()
	return nil
}

func (t *transcriptView) beginTool(block conversation.Block) error {
	key := conversation.BlockKey(block.RunID, block.ID)
	if _, exists := t.tools[key]; exists {
		return fmt.Errorf("terminal transcript: tool block %s started twice", block.ID)
	}
	if _, exists := t.textStreams[key]; exists {
		return fmt.Errorf("terminal transcript: block %s is already a live text block", block.ID)
	}
	rendered, err := t.present(block)
	if err != nil {
		return err
	}
	if tool, grouped := groupedTool(rendered); grouped {
		group := t.addGroupedTool(block.RunID, tool)
		tracked := trackedTool{id: group.id, block: tool}
		t.tools[key] = liveTool{runID: block.RunID, blocks: []trackedTool{tracked}, group: group}
		t.refreshSearch()
		return nil
	}
	t.sealToolGroup()
	live := liveTool{runID: block.RunID}
	for _, item := range rendered {
		mutable, isMutable := item.(mutableToolBlock)
		if isMutable {
			mutable.SetExpanded(t.details)
		}
		id := t.place(item, false)
		t.history.Append(block.RunID, id)
		live.ids = append(live.ids, id)
		if isMutable {
			tracked := trackedTool{id: id, block: mutable}
			live.blocks = append(live.blocks, tracked)
			t.toolViews = append(t.toolViews, trackedToolView{id: id, block: mutable})
		}
	}
	t.tools[key] = live
	t.refreshSearch()
	return nil
}

func groupedTool(rendered []base.Block) (*toolBlock, bool) {
	if len(rendered) != 1 {
		return nil, false
	}
	tool, ok := rendered[0].(*toolBlock)
	return tool, ok && groupableTool(tool.call)
}

func (t *transcriptView) addGroupedTool(runID string, tool *toolBlock) *trackedToolGroup {
	group := t.activeToolGroup
	if group == nil || group.runID != runID {
		t.sealToolGroup()
		block := newToolGroupBlock(t.theme, t.glyphs, t.details)
		block.Add(tool)
		group = &trackedToolGroup{runID: runID, block: block}
		group.id = t.place(block, false)
		t.history.Append(runID, group.id)
		t.toolViews = append(t.toolViews, trackedToolView{id: group.id, block: block})
		t.activeToolGroup = group
		return group
	}
	group.block.Add(tool)
	t.projectEntry(group.id)
	return group
}

func (t *transcriptView) sealToolGroup() {
	group := t.activeToolGroup
	if group == nil {
		return
	}
	group.block.Seal()
	t.projectEntry(group.id)
	t.activeToolGroup = nil
	t.finishToolGroupIfReady(group)
}

func (t *transcriptView) finishToolGroupIfReady(group *trackedToolGroup) {
	if group != nil && group.block.ReadyToFinish() {
		t.settleEntry(group.id)
	}
}

// SealToolGroups closes the trailing adjacency window after a cold snapshot.
// A live event stream closes it naturally on the next semantic boundary.
func (t *transcriptView) SealToolGroups() { t.sealToolGroup() }

func (t *transcriptView) present(block conversation.Block) ([]base.Block, error) {
	var present func(blockPresentation, conversation.Block) []base.Block
	switch block.Kind {
	case conversation.BlockUser:
		present = presentUser
	case conversation.BlockAssistant:
		present = presentMarkdown("flame")
	case conversation.BlockReasoning:
		present = presentMarkdown("thinking")
	case conversation.BlockQuestion:
		present = presentQuestion
	case conversation.BlockTool:
		present = presentTool
	case conversation.BlockNotice:
		present = presentNotice
	case conversation.BlockError:
		present = presentFailure
	default:
		return nil, fmt.Errorf("terminal transcript: no presenter for block kind %q", block.Kind)
	}
	return presentSafely(present, blockPresentation{
		Theme: t.theme, Glyphs: t.glyphs, Look: t.look, Syntax: t.syntax,
		Tools: defaultToolPresenters(), Speaker: t.history.Speaker(block), Image: t.presentImage,
	}, block)
}

func (t *transcriptView) presentImage(image conversation.InlineImage) base.Block {
	if t.images != nil {
		return t.images.Present(t.theme, image)
	}
	return fallbackInlineImage(t.theme, image)
}

func presentSafely(presenter func(blockPresentation, conversation.Block) []base.Block, presentation blockPresentation, block conversation.Block) (rendered []base.Block, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("terminal transcript: presenter for %q panicked: %v", block.Kind, recovered)
		}
	}()
	return presenter(presentation, block), nil
}

func (t *transcriptView) Append(block base.Block) {
	t.sealToolGroup()
	t.append(block)
}

func (t *transcriptView) append(block base.Block) transcriptEntryID {
	id := t.place(block, true)
	t.refreshSearch()
	return id
}

func (t *transcriptView) place(block base.Block, finished bool) transcriptEntryID {
	t.entrance = nil
	if t.nextEntry == ^transcriptEntryID(0) {
		panic("terminal: transcript entry identity exhausted")
	}
	t.nextEntry++
	id := t.nextEntry
	entry := newTranscriptEntry(t.theme, t.glyphs, block)
	t.entries[id] = entry
	t.order = append(t.order, id)
	entry.settled = finished
	entry.projection = snapshotTranscriptEntry(entry)
	t.resetWindow(true)
	if image, ok := block.(*terminalImageBlock); ok {
		t.prepareImage(id, image)
	}
	if finished {
		if message, ok := block.(*markdownBlock); ok {
			t.finishMarkdown(id, message)
		}
	}
	return id
}

func (t *transcriptView) entrySettled(id transcriptEntryID) bool {
	entry := t.entries[id]
	return entry != nil && entry.settled
}

func (t *transcriptView) settleEntry(id transcriptEntryID) {
	if entry := t.entries[id]; entry != nil {
		entry.settled = true
	}
}

func (t *transcriptView) projectEntry(id transcriptEntryID) {
	if entry := t.entries[id]; entry != nil {
		entry.projection = snapshotTranscriptEntry(entry)
		entry.document = nil
		t.resetWindow(true)
	}
}

func (t *transcriptView) DiscardExcess() {
	finished := 0
	for _, id := range t.order {
		if !t.entrySettled(id) {
			break
		}
		finished++
	}
	excess := finished - t.retain
	if excess <= 0 {
		return
	}
	discarded := slices.Clone(t.order[:excess])
	t.order = slices.Clone(t.order[excess:])
	// The new window releases every old native reference before semantic resources retire.
	t.resetWindow(true)
	for _, id := range discarded {
		t.releaseEntry(t.entries[id])
		delete(t.entries, id)
	}
	first := t.firstEntry()
	t.toolViews = slices.DeleteFunc(t.toolViews, func(item trackedToolView) bool { return item.id < first })
	for key, question := range t.pendingQuestions {
		if question.id < first {
			delete(t.pendingQuestions, key)
		}
	}
	t.history.DiscardBefore(first)
	if t.hasSelected && t.selected < first {
		t.hasSelected = false
		t.ensureSelection()
		t.syncSelectedEntry()
		t.announceSelection()
	}
	t.refreshSearch()
}

func (t *transcriptView) Reset() {
	t.entrance = nil
	old := t.entries
	t.entries = make(map[transcriptEntryID]*transcriptEntry)
	t.order = nil
	t.hasSelected = false
	t.selectionIntent = nil
	t.resetWindow(false)
	for _, entry := range old {
		t.releaseEntry(entry)
	}
	clear(t.textStreams)
	t.search.Reset()
	clear(t.tools)
	clear(t.pendingQuestions)
	t.history.Reset()
	t.activeToolGroup = nil
	t.pointerGesture.cancel()
	t.toolViews = nil
}

func (t *transcriptView) firstEntry() transcriptEntryID {
	if len(t.order) == 0 {
		return t.nextEntry + 1
	}
	return t.order[0]
}

func (t *transcriptView) SetRuns(runs []conversation.Run) {
	t.history.ReplaceRuns(runs)
}

func (t *transcriptView) JumpToRun(runID string) bool {
	first := t.firstEntry()
	last := first + blockOffset(t.content.Len())
	id, ok := t.history.FirstRetained(runID, first, last)
	if !ok {
		return false
	}
	t.selectEntry(id, true)
	return true
}

func blockOffset(index int) transcriptEntryID {
	if index < 0 {
		panic("terminal: negative transcript block offset")
	}
	return transcriptEntryID(index) // #nosec G115 -- validated nonnegative and int cannot exceed uint64.
}

func (t *transcriptView) lookFor(kind conversation.BlockKind) markdown.Look {
	look := t.look
	if kind == conversation.BlockReasoning {
		look.Text, look.Strong, look.Code = t.theme.Muted, t.theme.Subtle, t.theme.Info
	}
	return look
}

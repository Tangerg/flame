package terminal

import (
	"fmt"
	"slices"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/markdown"

	"github.com/Tangerg/flame/cli/internal/application/extensions"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type transcriptView struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	wheel  input.Wheel
	look   markdown.Look
	syntax highlight.Renderer

	content          headless.Transcript
	scroll           headless.Scroll
	selection        headless.Selection
	view             kit.Transcript
	search           transcriptSearch
	retain           int
	details          bool
	clipboard        headless.Clipboard
	entrance         grid.Drawable
	focused          bool
	selected         headless.BlockID
	hasSelected      bool
	entries          map[headless.BlockID]*transcriptEntry
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
	contentLease     *transcriptContentLease
	presentedBlocks  headless.Snapshot[transcriptBlockPresentation]
}

const transcriptPrompt keymap.Action = "prompt"

type liveTool struct {
	runID  string
	ids    []headless.BlockID
	blocks []trackedTool
	group  *trackedToolGroup
}

type liveText struct {
	runID  string
	kind   agent.BlockKind
	text   agent.StreamedText
	stream markdown.Stream
	stable []markdown.Block
	block  *markdownBlock
	id     headless.BlockID
}

type trackedTool struct {
	id    headless.BlockID
	block mutableToolBlock
}

type trackedToolView struct {
	id    headless.BlockID
	block toolDisclosure
}

type trackedToolGroup struct {
	id    headless.BlockID
	runID string
	block *toolGroupBlock
}

func (t *transcriptView) ToggleDetails() {
	first := t.content.FirstBlock()
	// #nosec G115 -- Transcript.Len is non-negative and cannot exceed the
	// addressable in-memory slice backing the transcript.
	end := first + headless.BlockID(t.content.Len())
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
		t.content.Changed(tracked.id)
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
	wheel input.Wheel,
	syntax highlight.Renderer,
	retain int,
	details bool,
	clipboard headless.Clipboard,
) *transcriptView {
	c := &transcriptView{
		theme: theme, glyphs: glyphs, wheel: wheel,
		look: markdownLook(theme, glyphs, syntax), syntax: syntax,
		search: newTranscriptSearch(), retain: max(retain, 4), details: details,
		clipboard: clipboard, entries: make(map[headless.BlockID]*transcriptEntry),
		tools: make(map[string]liveTool), textStreams: make(map[string]*liveText),
		pendingQuestions: make(map[string]trackedQuestion),
		history:          newTranscriptHistory(),
		keys:             transcriptKeys(),
		contentLease:     newTranscriptContentLease(),
	}
	c.scroll.Wheel(wheel)
	c.scroll.ToBottom()
	c.view = kit.Transcript{
		Content: &c.content, Scroll: &c.scroll, Selection: &c.selection,
		Theme: theme, Glyphs: glyphs, Current: -1,
	}
	return c
}

func (t *transcriptView) Draw(frame headless.Frame) {
	width, _ := frame.Size()
	t.presentedBlocks.Stage(frame, transcriptBlockPresentation{
		lease: t.contentLease, blocks: t.projectBlockPlacements(width),
	})
	t.view.Matches, t.view.Current = t.search.presentation()
	t.view.Draw(frame)
	if t.content.Len() == 0 && t.entrance != nil {
		t.entrance.Draw(frame.View)
	}
}

// SetEntrance installs a presentation-only projection that is consumed by the
// first transcript block or reset. It is not part of retained transcript state.
func (t *transcriptView) SetEntrance(entrance grid.Drawable) { t.entrance = entrance }

func (t *transcriptView) Handle(event input.Event) bool {
	if t.handleKey(event) {
		return true
	}
	handled := t.view.Handle(event)
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
	if key.Code == input.Esc && t.selection.Active() {
		t.selection.Clear()
		return true
	}
	_, handled := t.matcher.Handle(t.keys, key, t.Do)
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
	case headless.SelectPrev:
		return t.moveSelection(-1)
	case headless.SelectNext:
		return t.moveSelection(1)
	case headless.SelectFirst:
		return t.selectEdge(false)
	case headless.SelectLast:
		return t.selectEdge(true)
	case headless.Collapse:
		return t.setSelectedExpanded(false)
	case headless.Expand:
		return t.setSelectedExpanded(true)
	case toggleDetails:
		return t.toggleSelected()
	case headless.Copy:
		return t.copySelected()
	}
	return false
}

func transcriptKeys() *keymap.Map {
	keys := &keymap.Map{}
	keys.Bind(headless.SelectPrev, input.Chord{Code: input.Up})
	keys.Bind(headless.SelectNext, input.Chord{Code: input.Down})
	keys.Bind(headless.SelectFirst, input.Chord{Code: input.Home})
	keys.Bind(headless.SelectLast, input.Chord{Code: input.End})
	keys.Bind(headless.Expand, input.Chord{Code: input.Right})
	keys.Bind(headless.Collapse, input.Chord{Code: input.Left})
	keys.Bind(toggleDetails, input.Chord{Code: input.Enter})
	keys.Bind(headless.Copy, input.Alt.Rune('c'))
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
		t.search.Close()
	}
}

func (t *transcriptView) mutateTrackedTool(tracked trackedTool, mutate func(mutableToolBlock)) bool {
	before := tracked.block.Expanded()
	mutate(tracked.block)
	t.content.Changed(tracked.id)
	return t.focused && tracked.id == t.selected && before && !tracked.block.Expanded()
}

func (t *transcriptView) appendCompleted(block agent.Block, registry *extensions.Registry) error {
	rendered, err := t.present(block, registry)
	if err != nil {
		return err
	}
	if block.Kind == agent.BlockTool {
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
			key = transcriptBlockKey(block.RunID, block.ID)
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

func (t *transcriptView) beginTool(block agent.Block, registry *extensions.Registry) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	if _, exists := t.tools[key]; exists {
		return fmt.Errorf("terminal transcript: tool block %s started twice", block.ID)
	}
	if _, exists := t.textStreams[key]; exists {
		return fmt.Errorf("terminal transcript: block %s is already a live text block", block.ID)
	}
	rendered, err := t.present(block, registry)
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

func groupedTool(rendered []headless.Block) (*toolBlock, bool) {
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
	t.content.Changed(group.id)
	return group
}

func (t *transcriptView) sealToolGroup() {
	group := t.activeToolGroup
	if group == nil {
		return
	}
	group.block.Seal()
	t.content.Changed(group.id)
	t.activeToolGroup = nil
	t.finishToolGroupIfReady(group)
}

func (t *transcriptView) finishToolGroupIfReady(group *trackedToolGroup) {
	if group != nil && group.block.ReadyToFinish() {
		t.content.Finish(group.id)
	}
}

// SealToolGroups closes the trailing adjacency window after a cold snapshot.
// A live event stream closes it naturally on the next semantic boundary.
func (t *transcriptView) SealToolGroups() { t.sealToolGroup() }

func (t *transcriptView) present(block agent.Block, registry *extensions.Registry) ([]headless.Block, error) {
	for _, presenter := range registry.Values(BlockPresenters) {
		if presenter.Kind == block.Kind {
			return presentSafely(presenter, BlockPresentation{
				Theme: t.theme, Glyphs: t.glyphs, Look: t.look, Syntax: t.syntax,
				Tools: registry.Values(ToolPresenters), Speaker: t.history.Speaker(block), Image: t.presentImage,
			}, block)
		}
	}
	return nil, fmt.Errorf("terminal transcript: no presenter for block kind %q", block.Kind)
}

func (t *transcriptView) presentImage(image agent.InlineImage) headless.Block {
	if t.images != nil {
		return t.images.Present(t.theme, image)
	}
	return fallbackInlineImage(t.theme, image)
}

func presentSafely(presenter BlockPresenter, presentation BlockPresentation, block agent.Block) (rendered []headless.Block, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("terminal transcript: presenter for %q panicked: %v", presenter.Kind, recovered)
		}
	}()
	return presenter.Present(presentation, block), nil
}

func (t *transcriptView) Append(block headless.Block) {
	t.sealToolGroup()
	t.append(block)
}

func (t *transcriptView) append(block headless.Block) headless.BlockID {
	id := t.place(block, true)
	t.refreshSearch()
	return id
}

func (t *transcriptView) place(block headless.Block, finished bool) headless.BlockID {
	t.entrance = nil
	entry := newTranscriptEntry(t.theme, t.glyphs, block)
	id := t.content.Append(entry)
	t.entries[id] = entry
	if finished {
		t.content.Finish(id)
	}
	return id
}

type discardedOutput struct{}

func (discardedOutput) Print(grid.Drawable) {}

func (t *transcriptView) DiscardExcess() {
	if t.content.Width() <= 0 {
		return
	}
	finished := 0
	for i := range t.content.Len() {
		id := t.content.FirstBlock() + headless.BlockID(i)
		if !t.content.Finished(id) {
			break
		}
		finished++
	}
	if excess := finished - t.retain; excess > 0 {
		t.view.Commit(discardedOutput{}, excess)
	}
	first := t.content.FirstBlock()
	t.toolViews = slices.DeleteFunc(t.toolViews, func(item trackedToolView) bool { return item.id < first })
	for id := range t.entries {
		if id < first {
			delete(t.entries, id)
		}
	}
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
	t.contentLease.retire()
	t.contentLease = newTranscriptContentLease()
	t.content = headless.Transcript{}
	t.scroll = headless.Scroll{}
	t.scroll.Wheel(t.wheel)
	t.scroll.ToBottom()
	t.selection = headless.Selection{}
	t.view.Content, t.view.Scroll = &t.content, &t.scroll
	t.view.Selection = &t.selection
	for _, live := range t.textStreams {
		live.stream.Reset()
	}
	clear(t.textStreams)
	t.search.Reset(&t.content)
	clear(t.tools)
	clear(t.pendingQuestions)
	clear(t.entries)
	t.history.Reset()
	t.activeToolGroup = nil
	t.hasSelected = false
	t.pointerGesture.cancel()
	t.toolViews = nil
}

func (t *transcriptView) SetRuns(runs []agent.Run) {
	t.history.ReplaceRuns(runs)
}

func (t *transcriptView) JumpToRun(runID string) bool {
	first := t.content.FirstBlock()
	last := first + blockOffset(t.content.Len())
	id, ok := t.history.FirstRetained(runID, first, last)
	if !ok {
		return false
	}
	t.selectEntry(id, true)
	return true
}

func blockOffset(index int) headless.BlockID {
	if index < 0 {
		panic("terminal: negative transcript block offset")
	}
	return headless.BlockID(index) // #nosec G115 -- validated nonnegative and int cannot exceed uint64.
}

func transcriptBlockKey(runID, blockID string) string {
	return (agent.BlockIdentity{RunID: runID, BlockID: blockID}).Key()
}

func (t *transcriptView) lookFor(kind agent.BlockKind) markdown.Look {
	look := t.look
	if kind == agent.BlockReasoning {
		look.Text, look.Strong, look.Code = t.theme.Muted, t.theme.Subtle, t.theme.Info
	}
	return look
}

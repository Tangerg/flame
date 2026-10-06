package terminal

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/cli/internal/application/extensions"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/components/headless"
)

func (t *transcriptView) Apply(event conversation.Event, registry *extensions.Registry) error {
	return t.apply("", event, registry)
}

func (t *transcriptView) ApplyRunEvent(envelope conversation.RunEvent, registry *extensions.Registry) error {
	if started, ok := envelope.Event.(conversation.SegmentStarted); ok {
		t.history.Observe(started.Run)
	}
	return t.apply(envelope.RunID, envelope.Event, registry)
}

func (t *transcriptView) apply(runID string, event conversation.Event, registry *extensions.Registry) error {
	switch e := event.(type) {
	case conversation.BlockStarted:
		if e.Block.Kind == conversation.BlockAssistant || e.Block.Kind == conversation.BlockReasoning {
			return t.begin(e.Block)
		}
		if e.Block.Kind == conversation.BlockTool {
			return t.beginTool(e.Block, registry)
		}
		t.sealToolGroup()
	case conversation.BlockDelta:
		key := transcriptBlockKey(runID, e.BlockID)
		if _, live := t.tools[key]; live {
			return t.deltaTool(key, e)
		}
		return t.delta(key, e)
	case conversation.ToolArgumentsDelta, conversation.RunProgress:
		// Tool arguments are provisional JSON and progress belongs in the status
		// chrome. Neither creates an authoritative transcript block.
	case conversation.CustomEvent:
		return t.appendCustom(runID, e, registry)
	case conversation.BlockCompleted:
		return t.complete(e.Block, registry)
	case conversation.RunFinished:
		t.settleRun(runID)
	case conversation.RunInterrupted:
		t.sealToolGroup()
	}
	return nil
}

func (t *transcriptView) appendCustom(runID string, event conversation.CustomEvent, registry *extensions.Registry) error {
	for _, presenter := range registry.Values(CustomEventPresenters) {
		if presenter.Name != event.Name {
			continue
		}
		rendered, err := presentCustomSafely(presenter, BlockPresentation{
			Theme: t.theme, Glyphs: t.glyphs, Look: t.look, Syntax: t.syntax,
			Tools: registry.Values(ToolPresenters), Speaker: "runtime", Image: t.presentImage,
		}, event)
		if err != nil {
			return err
		}
		t.sealToolGroup()
		for _, block := range rendered {
			id := t.append(block)
			t.history.Append(runID, id)
		}
		return nil
	}
	return nil
}

func presentCustomSafely(presenter CustomEventPresenter, presentation BlockPresentation, event conversation.CustomEvent) (rendered []headless.Block, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("terminal transcript: custom presenter for %q panicked: %v", presenter.Name, recovered)
		}
	}()
	return presenter.Present(presentation, event), nil
}

func (t *transcriptView) begin(block conversation.Block) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	if _, exists := t.textStreams[key]; exists {
		return fmt.Errorf("terminal transcript: text block %s started twice", block.ID)
	}
	if _, exists := t.tools[key]; exists {
		return fmt.Errorf("terminal transcript: block %s is already a live tool", block.ID)
	}
	t.sealToolGroup()
	speaker := t.history.Speaker(block)
	live := &liveText{
		runID: block.RunID, kind: block.Kind, text: conversation.NewStreamedText(block.Text),
		block: &markdownBlock{theme: t.theme, speaker: speaker},
	}
	live.stream.SetLook(t.lookFor(block.Kind))
	live.id = t.place(live.block, false)
	t.history.Append(block.RunID, live.id)
	t.textStreams[key] = live
	if block.Text != "" {
		t.updateLiveText(live, block.Text)
	}
	return nil
}

func (t *transcriptView) delta(key string, delta conversation.BlockDelta) error {
	live, ok := t.textStreams[key]
	if !ok {
		return fmt.Errorf("terminal transcript: delta for inactive text block %s", delta.BlockID)
	}
	if err := live.text.Apply(delta); err != nil {
		return fmt.Errorf("terminal transcript: stream text block %s: %w", delta.BlockID, err)
	}
	t.updateLiveText(live, delta.Text)
	return nil
}

func (t *transcriptView) updateLiveText(live *liveText, text string) {
	stable, feedErr := live.stream.Feed(text)
	live.stable = append(live.stable, stable...)
	if feedErr != nil {
		live.diagnostic = errors.Join(live.diagnostic, feedErr)
	}
	blocks := slices.Clone(live.stable)
	open, openErr := live.stream.Open()
	blocks = append(blocks, open...)
	live.block.setBlocks(blocks, errors.Join(live.diagnostic, openErr))
	t.content.Changed(live.id)
	t.refreshSearch()
}

func (t *transcriptView) deltaTool(key string, delta conversation.BlockDelta) error {
	live, ok := t.tools[key]
	if !ok {
		return fmt.Errorf("terminal transcript: delta for inactive tool block %s", delta.BlockID)
	}
	for _, tracked := range live.blocks {
		tracked.block.AppendOutput(delta.Text)
		t.content.Changed(tracked.id)
	}
	t.refreshSearch()
	t.announceSelection()
	return nil
}

func (t *transcriptView) complete(block conversation.Block, registry *extensions.Registry) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	if _, live := t.textStreams[key]; live {
		return t.completeStream(block)
	}
	if block.Kind == conversation.BlockTool && t.completeLiveTool(block) {
		return nil
	}
	if block.Kind == conversation.BlockQuestion && t.revealAnsweredQuestion(block) {
		return nil
	}
	return t.appendCompleted(block, registry)
}

func (t *transcriptView) completeStream(block conversation.Block) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	live, ok := t.textStreams[key]
	if !ok {
		return fmt.Errorf("terminal transcript: completion for inactive text block %s", block.ID)
	}
	// The completed value is authoritative. Re-rendering it once also repairs a
	// transport that intentionally replaced an earlier provisional tail.
	live.block.setSource(block.Text, t.lookFor(block.Kind))
	t.finishMarkdown(live.id, live.block)
	live.stream.Reset()
	delete(t.textStreams, key)
	for _, image := range block.Images {
		id := t.append(t.presentImage(image))
		t.history.Append(block.RunID, id)
	}
	t.refreshSearch()
	return nil
}

func (t *transcriptView) completeLiveTool(block conversation.Block) bool {
	key := transcriptBlockKey(block.RunID, block.ID)
	live, ok := t.tools[key]
	if !ok {
		return false
	}
	selectedCollapsed := false
	for _, tracked := range live.blocks {
		selectedCollapsed = t.mutateTrackedTool(tracked, func(tool mutableToolBlock) { tool.Update(block) }) || selectedCollapsed
	}
	if selectedCollapsed {
		t.revealSelected()
	}
	if live.group != nil {
		t.finishToolGroupIfReady(live.group)
	} else {
		for _, id := range live.ids {
			t.content.Finish(id)
		}
	}
	delete(t.tools, key)
	if len(live.blocks) == 0 {
		return false
	}
	t.refreshSearch()
	t.announceSelection()
	return true
}

func (t *transcriptView) settleLivePresentation(toolStatus conversation.ToolStatus) {
	for id, live := range t.textStreams {
		live.block.setSource(live.text.String(), t.lookFor(live.kind))
		t.finishMarkdown(live.id, live.block)
		live.stream.Reset()
		delete(t.textStreams, id)
	}
	selectedCollapsed := false
	for id, live := range t.tools {
		for _, tracked := range live.blocks {
			selectedCollapsed = t.mutateTrackedTool(tracked, func(tool mutableToolBlock) { tool.Finish(toolStatus) }) || selectedCollapsed
		}
		if live.group != nil {
			t.finishToolGroupIfReady(live.group)
		} else {
			for _, blockID := range live.ids {
				t.content.Finish(blockID)
			}
		}
		delete(t.tools, id)
	}
	t.finishPendingQuestions("")
	if selectedCollapsed {
		t.revealSelected()
	}
	t.sealToolGroup()
	t.refreshSearch()
	t.announceSelection()
}

// rejectLivePresentation closes provisional terminal blocks after the client
// can no longer trust its event projection. It does not invent a Runtime Run
// outcome; an authoritative cold snapshot will replace this presentation.
func (t *transcriptView) rejectLivePresentation() {
	t.settleLivePresentation(conversation.ToolError)
}

// settleRun ends a Run's presentation. Runtime closes every open Item before
// a Run finishes and the conversation fold rejects a finish that leaves one
// open, so only presentation-owned state remains to settle here.
func (t *transcriptView) settleRun(runID string) {
	t.finishPendingQuestions(runID)
	if t.activeToolGroup != nil && t.activeToolGroup.runID == runID {
		t.sealToolGroup()
	}
	t.refreshSearch()
	t.announceSelection()
}

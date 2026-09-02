package terminal

import (
	"fmt"
	"slices"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/markdown"

	"github.com/Tangerg/flame/cli/internal/application/extensions"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func (t *transcriptView) Apply(event agent.Event, registry *extensions.Registry) error {
	return t.apply("", event, registry)
}

func (t *transcriptView) ApplyRunEvent(envelope agent.RunEvent, registry *extensions.Registry) error {
	if started, ok := envelope.Event.(agent.SegmentStarted); ok {
		t.history.Observe(started.Run)
	}
	return t.apply(envelope.RunID, envelope.Event, registry)
}

func (t *transcriptView) apply(runID string, event agent.Event, registry *extensions.Registry) error {
	switch e := event.(type) {
	case agent.BlockStarted:
		if e.Block.Kind == agent.BlockAssistant || e.Block.Kind == agent.BlockReasoning {
			return t.begin(e.Block)
		}
		if e.Block.Kind == agent.BlockTool {
			return t.beginTool(e.Block, registry)
		}
		t.sealToolGroup()
	case agent.BlockDelta:
		key := transcriptBlockKey(runID, e.BlockID)
		if _, live := t.tools[key]; live {
			return t.deltaTool(key, e)
		}
		return t.delta(key, e)
	case agent.ToolArgumentsDelta, agent.RunProgress:
		// Tool arguments are provisional JSON and progress belongs in the status
		// chrome. Neither creates an authoritative transcript block.
	case agent.CustomEvent:
		return t.appendCustom(runID, e, registry)
	case agent.BlockCompleted:
		return t.complete(e.Block, registry)
	case agent.RunFinished:
		if runID == "" {
			t.settleLive(e.Outcome)
		} else {
			t.settleRun(runID, e.Outcome)
		}
	case agent.RunInterrupted:
		t.sealToolGroup()
	}
	return nil
}

func (t *transcriptView) appendCustom(runID string, event agent.CustomEvent, registry *extensions.Registry) error {
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

func presentCustomSafely(presenter CustomEventPresenter, presentation BlockPresentation, event agent.CustomEvent) (rendered []headless.Block, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("terminal transcript: custom presenter for %q panicked: %v", presenter.Name, recovered)
		}
	}()
	return presenter.Present(presentation, event), nil
}

func (t *transcriptView) begin(block agent.Block) error {
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
		runID: block.RunID, kind: block.Kind, text: agent.NewStreamedText(block.Text),
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

func (t *transcriptView) delta(key string, delta agent.BlockDelta) error {
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
	live.stable = append(live.stable, live.stream.Feed(text)...)
	blocks := slices.Clone(live.stable)
	blocks = append(blocks, live.stream.Open()...)
	live.block.doc.SetBlocks(blocks)
	t.content.Changed(live.id)
	t.refreshSearch()
}

func (t *transcriptView) deltaTool(key string, delta agent.BlockDelta) error {
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

func (t *transcriptView) complete(block agent.Block, registry *extensions.Registry) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	if _, live := t.textStreams[key]; live {
		return t.completeStream(block)
	}
	if block.Kind == agent.BlockTool && t.completeLiveTool(block) {
		return nil
	}
	return t.appendCompleted(block, registry)
}

func (t *transcriptView) completeStream(block agent.Block) error {
	key := transcriptBlockKey(block.RunID, block.ID)
	live, ok := t.textStreams[key]
	if !ok {
		return fmt.Errorf("terminal transcript: completion for inactive text block %s", block.ID)
	}
	// The completed value is authoritative. Re-rendering it once also repairs a
	// transport that intentionally replaced an earlier provisional tail.
	live.block.doc.SetBlocks(markdown.Render(block.Text, t.lookFor(block.Kind)))
	t.content.Changed(live.id)
	t.content.Finish(live.id)
	live.stream.Reset()
	delete(t.textStreams, key)
	for _, image := range block.Images {
		id := t.append(t.presentImage(image))
		t.history.Append(block.RunID, id)
	}
	t.refreshSearch()
	return nil
}

func (t *transcriptView) completeLiveTool(block agent.Block) bool {
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

func (t *transcriptView) settleLive(outcome agent.Outcome) {
	toolStatus := agent.ToolError
	if outcome.Status == agent.OutcomeCanceled {
		toolStatus = agent.ToolCanceled
	}
	t.settleLivePresentation(toolStatus)
}

func (t *transcriptView) settleLivePresentation(toolStatus agent.ToolStatus) {
	for id, live := range t.textStreams {
		t.content.Finish(live.id)
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
	t.settleLivePresentation(agent.ToolError)
}

func (t *transcriptView) settleRun(runID string, outcome agent.Outcome) {
	for id, live := range t.textStreams {
		if live.runID != runID {
			continue
		}
		t.content.Finish(live.id)
		live.stream.Reset()
		delete(t.textStreams, id)
	}
	toolStatus := agent.ToolError
	if outcome.Status == agent.OutcomeCanceled {
		toolStatus = agent.ToolCanceled
	}
	selectedCollapsed := false
	for id, live := range t.tools {
		if live.runID != runID {
			continue
		}
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
	t.finishPendingQuestions(runID)
	if selectedCollapsed {
		t.revealSelected()
	}
	if t.activeToolGroup != nil && t.activeToolGroup.runID == runID {
		t.sealToolGroup()
	}
	t.refreshSearch()
	t.announceSelection()
}

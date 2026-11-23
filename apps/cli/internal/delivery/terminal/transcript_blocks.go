package terminal

import (
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/ui/base"
)

func (t *transcriptView) mutateTrackedTool(tracked trackedTool, mutate func(mutableToolBlock)) bool {
	before := tracked.block.Expanded()
	mutate(tracked.block)
	t.projectEntry(tracked.id)
	return t.focused && t.isSelected(tracked.id) && before && !tracked.block.Expanded()
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
		state := settledEntry
		if isPendingQuestion {
			state = liveEntry
		}
		id := t.place(item, state)
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
		id := t.place(item, liveEntry)
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
		group.id = t.place(block, liveEntry)
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

func (t *transcriptView) SealToolGroups() { t.sealToolGroup() }

func (t *transcriptView) present(block conversation.Block) ([]base.Block, error) {
	var present func(blockPresentation, conversation.Block) []base.Block
	switch block.Kind {
	case conversation.BlockUser:
		present = presentUser
	case conversation.BlockAssistant:
		present = presentMarkdown(assistantSpeaker)
	case conversation.BlockReasoning:
		present = presentMarkdown(reasoningSpeaker)
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

package terminal

import (
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

// transcriptHistory owns the retained transcript projection for each run and
// the lineage needed to label that run. Keeping both facts together prevents
// navigation and presentation from advancing independent run views.
type transcriptHistory struct {
	entries  map[string][]transcriptEntryID
	lineages map[string]conversation.RunLineage
}

func newTranscriptHistory() transcriptHistory {
	return transcriptHistory{
		entries:  make(map[string][]transcriptEntryID),
		lineages: make(map[string]conversation.RunLineage),
	}
}

func (h *transcriptHistory) Reset() {
	clear(h.entries)
	clear(h.lineages)
}

func (h *transcriptHistory) ReplaceRuns(runs []conversation.Run) {
	clear(h.lineages)
	for _, run := range runs {
		h.Observe(run)
	}
}

func (h *transcriptHistory) Observe(run conversation.Run) {
	h.lineages[run.ID] = run.Lineage
}

func (h *transcriptHistory) Append(runID string, id transcriptEntryID) {
	if runID == "" {
		return
	}
	h.entries[runID] = append(h.entries[runID], id)
}

func (h *transcriptHistory) DiscardBefore(first transcriptEntryID) {
	for runID, ids := range h.entries {
		ids = slices.DeleteFunc(ids, func(id transcriptEntryID) bool { return id < first })
		if len(ids) == 0 {
			delete(h.entries, runID)
			continue
		}
		h.entries[runID] = ids
	}
}

func (h *transcriptHistory) FirstRetained(
	runID string,
	first, last transcriptEntryID,
) (transcriptEntryID, bool) {
	for _, id := range h.entries[runID] {
		if id >= first && id < last {
			return id, true
		}
	}
	return 0, false
}

func (h *transcriptHistory) Speaker(block conversation.Block) string {
	lineage, known := h.lineages[block.RunID]
	if !known || lineage.IsRoot() {
		switch block.Kind {
		case conversation.BlockUser:
			return selfSpeaker
		case conversation.BlockReasoning:
			return "thinking"
		default:
			return "flame"
		}
	}
	identity := shortIdentity(block.RunID)
	switch block.Kind {
	case conversation.BlockUser:
		return "subagent input · " + identity
	case conversation.BlockReasoning:
		return "subagent thinking · " + identity
	default:
		return "subagent · " + identity
	}
}

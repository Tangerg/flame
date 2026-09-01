package terminal

import (
	"slices"

	"github.com/Tangerg/oolong/components/headless"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

// transcriptHistory owns the retained transcript projection for each run and
// the lineage needed to label that run. Keeping both facts together prevents
// navigation and presentation from advancing independent run views.
type transcriptHistory struct {
	entries  map[string][]headless.BlockID
	lineages map[string]agent.RunLineage
}

func newTranscriptHistory() transcriptHistory {
	return transcriptHistory{
		entries:  make(map[string][]headless.BlockID),
		lineages: make(map[string]agent.RunLineage),
	}
}

func (h *transcriptHistory) Reset() {
	clear(h.entries)
	clear(h.lineages)
}

func (h *transcriptHistory) ReplaceRuns(runs []agent.Run) {
	clear(h.lineages)
	for _, run := range runs {
		h.Observe(run)
	}
}

func (h *transcriptHistory) Observe(run agent.Run) {
	h.lineages[run.ID] = run.Lineage
}

func (h *transcriptHistory) Append(runID string, id headless.BlockID) {
	if runID == "" {
		return
	}
	h.entries[runID] = append(h.entries[runID], id)
}

func (h *transcriptHistory) DiscardBefore(first headless.BlockID) {
	for runID, ids := range h.entries {
		ids = slices.DeleteFunc(ids, func(id headless.BlockID) bool { return id < first })
		if len(ids) == 0 {
			delete(h.entries, runID)
			continue
		}
		h.entries[runID] = ids
	}
}

func (h *transcriptHistory) FirstRetained(
	runID string,
	first, last headless.BlockID,
) (headless.BlockID, bool) {
	for _, id := range h.entries[runID] {
		if id >= first && id < last {
			return id, true
		}
	}
	return 0, false
}

func (h *transcriptHistory) Speaker(block agent.Block) string {
	lineage, known := h.lineages[block.RunID]
	if !known || lineage.IsRoot() {
		switch block.Kind {
		case agent.BlockUser:
			return "you"
		case agent.BlockReasoning:
			return "thinking"
		default:
			return "flame"
		}
	}
	identity := shortIdentity(block.RunID)
	switch block.Kind {
	case agent.BlockUser:
		return "subagent input · " + identity
	case agent.BlockReasoning:
		return "subagent thinking · " + identity
	default:
		return "subagent · " + identity
	}
}

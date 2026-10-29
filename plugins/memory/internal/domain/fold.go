package agentmemory

type FoldPlan struct {
	InsertContents []string
	PruneIDs       []ItemID
}

// Approved facts and rejected tombstones outlive curator output; only unpinned auto proposals may be pruned.
func (p Publication) Fold(existing []Item) (FoldPlan, error) {
	if err := p.Validate(); err != nil {
		return FoldPlan{}, err
	}
	contents := p.contents
	present := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		if err := validateGenerationItem(p.project, item); err != nil {
			return FoldPlan{}, err
		}
		present[Digest(item.content)] = struct{}{}
	}

	var plan FoldPlan
	desired := make(map[string]struct{}, len(contents))
	for _, content := range contents {
		digest := Digest(content)
		desired[digest] = struct{}{}
		if _, seen := present[digest]; !seen {
			plan.InsertContents = append(plan.InsertContents, content)
		}
	}

	for _, item := range existing {
		if item.origin != OriginAuto || item.status != StatusPending || item.pinned {
			continue
		}
		// Omitted context and intervening human changes cannot authorize deletion.
		if revision, observed := p.basis[item.id]; !observed || revision != item.Revision() {
			continue
		}
		if _, keep := desired[Digest(item.content)]; !keep {
			plan.PruneIDs = append(plan.PruneIDs, item.id)
		}
	}
	return plan, nil
}

func normalizeGeneration(contents []string) ([]string, error) {
	return normalizeFactList(contents, MaxCurationProposals, "curation result")
}

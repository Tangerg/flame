package agentmemory

type FoldPlan struct {
	InsertContents []string
	PruneIDs       []ItemID
}

// Approved facts and rejected tombstones outlive curator output; only unpinned auto proposals may be pruned.
func Fold(existing []Item, contents []string) (FoldPlan, error) {
	contents, err := NormalizeGeneration(contents)
	if err != nil {
		return FoldPlan{}, err
	}
	present := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		present[Digest(item.content)] = struct{}{}
	}

	var plan FoldPlan
	desired := make(map[string]struct{}, len(contents))
	for _, content := range contents {
		digest := Digest(content)
		if _, dup := desired[digest]; dup {
			continue
		}
		desired[digest] = struct{}{}
		if _, seen := present[digest]; !seen {
			plan.InsertContents = append(plan.InsertContents, content)
		}
	}

	for _, item := range existing {
		if item.status != StatusPending {
			continue
		}
		if _, keep := desired[Digest(item.content)]; !keep {
			plan.PruneIDs = append(plan.PruneIDs, item.id)
		}
	}
	return plan, nil
}

func NormalizeGeneration(contents []string) ([]string, error) {
	return normalizeFactList(contents, MaxCurationProposals, "curation result")
}

package runs

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func (p Pending) ValidateProjection(values []rundomain.Run, items []transcript.Item) error {
	treeRuns := make([]rundomain.Run, 0, len(p.Continuations))
	for _, value := range values {
		if value.Lineage().TreeRootID(value.ID()) == p.RootRunID {
			treeRuns = append(treeRuns, value)
		}
	}
	if err := validatePendingRunTree(p, treeRuns); err != nil {
		return err
	}

	active := make(map[identity.RunID]rundomain.Run, len(p.Continuations))
	for _, value := range treeRuns {
		if !value.State().IsTerminal() {
			active[value.ID()] = value
		}
	}
	itemsByID := indexPendingItems(items)
	if _, err := p.ProjectInterrupts(itemsByID); err != nil {
		return fmt.Errorf("runs: validate parked Run tree %q: %w", p.RootRunID, err)
	}
	interruptItems := make(map[identity.ItemID]struct{}, len(p.Interrupts))
	for _, open := range p.Interrupts {
		interruptItems[open.ItemID] = struct{}{}
	}
	drainedItems := make(map[identity.ItemID]struct{})
	for _, continuation := range p.Continuations {
		for _, drained := range continuation.DrainedTools {
			drainedItems[drained.ItemID] = struct{}{}
		}
	}
	if err := validatePendingRunningItems(
		p.RootRunID,
		active,
		items,
		interruptItems,
		drainedItems,
	); err != nil {
		return err
	}
	claimedItems := make(map[identity.ItemID]string, len(interruptItems))
	for itemID := range interruptItems {
		claimedItems[itemID] = "interrupt"
	}
	for _, continuation := range p.Continuations {
		if err := validatePendingContinuationTools(
			p.RootRunID,
			continuation,
			itemsByID,
			claimedItems,
		); err != nil {
			return err
		}
	}
	return nil
}

func validatePendingRunTree(pending Pending, values []rundomain.Run) error {
	if _, _, err := pending.parkedTree(values); err != nil {
		return fmt.Errorf("runs: validate parked Run tree %q: %w", pending.RootRunID, err)
	}
	return nil
}

func (p Pending) parkedTree(values []rundomain.Run) (rundomain.Tree, map[identity.RunID]rundomain.Run, error) {
	if err := p.Validate(); err != nil {
		return rundomain.Tree{}, nil, err
	}
	all, active, err := indexParkedRuns(values)
	if err != nil {
		return rundomain.Tree{}, nil, err
	}
	root, found := all[p.RootRunID]
	if !found || !root.Lineage().IsRoot() {
		return rundomain.Tree{}, nil, errors.New("root Run is missing")
	}
	if root.State() != rundomain.Waiting {
		return rundomain.Tree{}, nil, fmt.Errorf("root Run is %s", root.State())
	}
	if len(active) != len(p.Continuations) {
		return rundomain.Tree{}, nil, fmt.Errorf("%d continuations do not cover %d active Runs", len(p.Continuations), len(active))
	}
	if err := admitParkedTree(p, root); err != nil {
		return rundomain.Tree{}, nil, err
	}
	tree, err := p.continuationTree(active, root)
	return tree, active, err
}

func indexParkedRuns(values []rundomain.Run) (map[identity.RunID]rundomain.Run, map[identity.RunID]rundomain.Run, error) {
	all := make(map[identity.RunID]rundomain.Run, len(values))
	active := make(map[identity.RunID]rundomain.Run)
	for index, value := range values {
		if value.ID().IsZero() {
			return nil, nil, fmt.Errorf("run[%d] was never constructed", index)
		}
		if _, duplicate := all[value.ID()]; duplicate {
			return nil, nil, fmt.Errorf("duplicate Run %q", value.ID())
		}
		all[value.ID()] = value
		if !value.State().IsTerminal() {
			active[value.ID()] = value
		}
	}
	return all, active, nil
}

func admitParkedTree(pending Pending, root rundomain.Run) error {
	capabilities := root.Capabilities()
	for _, open := range pending.Interrupts {
		if !slices.Contains(capabilities.InterruptTypes, open.Type()) {
			return fmt.Errorf("interrupt item %q has kind %s outside the root Run's capabilities", open.ItemID, open.Type())
		}
	}
	if len(pending.Continuations) > 1 && !capabilities.ChildRuns {
		return errors.New("child Runs park under a root without child-Run capability")
	}
	return nil
}

func (p Pending) continuationTree(active map[identity.RunID]rundomain.Run, root rundomain.Run) (rundomain.Tree, error) {
	members := make([]rundomain.TreeMember, 0, len(p.Continuations))
	for _, continuation := range p.Continuations {
		value, found := active[continuation.RunID]
		if !found || value.SessionID() != root.SessionID() || value.State() != rundomain.Waiting {
			return rundomain.Tree{}, fmt.Errorf("continuation Run %q is not active and waiting", continuation.RunID)
		}
		members = append(members, rundomain.TreeMember{RunID: value.ID(), Lineage: value.Lineage()})
	}
	tree, err := rundomain.NewTree(p.RootRunID, members)
	if err != nil {
		return rundomain.Tree{}, err
	}
	for index, runID := range tree.Postorder() {
		if p.Continuations[index].RunID != runID {
			return rundomain.Tree{}, fmt.Errorf("continuation[%d] is Run %q, canonical postorder requires %q", index, p.Continuations[index].RunID, runID)
		}
	}
	return tree, nil
}

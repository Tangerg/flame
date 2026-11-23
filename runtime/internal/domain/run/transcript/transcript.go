package transcript

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

var (
	ErrRunNotFound = errors.New("run not found in timeline")
	ErrNotRoot     = errors.New("run is not a root run")
)

type RunNode struct {
	ID          identity.RunID
	Lineage     run.Lineage
	CreatedAt   time.Time
	MessageMark run.MessageMark
	Terminal    bool
}

func (r RunNode) IsRoot() bool { return r.Lineage.IsRoot() }

type Timeline []RunNode

func TimelineFromRuns(runs []run.Run) Timeline {
	nodes := make(Timeline, len(runs))
	for i, current := range runs {
		nodes[i] = RunNode{
			ID: current.ID(), Lineage: current.Lineage(),
			CreatedAt: current.CreatedAt(), MessageMark: current.MessageMark(),
			Terminal: current.State().IsTerminal(),
		}
	}
	return nodes
}

func OpeningUserMessagesByRun(items []Item) map[identity.RunID][]ContentBlock {
	out := make(map[identity.RunID][]ContentBlock)
	for _, item := range items {
		message, ok := item.body.(UserMessageBody)
		if !ok {
			continue
		}
		if _, exists := out[item.RunID()]; exists {
			continue
		}
		out[item.RunID()] = CloneContent(message.Content)
	}
	return out
}

type Boundary struct {
	KeepMessageMark run.MessageMark
	KeepRunID       identity.RunID
	Dropped         []RunNode
}

type PortableBoundary struct {
	KeepMessageMark run.MessageMark
	KeepRunID       identity.RunID
	RunIDs          []identity.RunID
}

func (b Boundary) DroppedRunIDs() []identity.RunID {
	ids := make([]identity.RunID, len(b.Dropped))
	for i, node := range b.Dropped {
		ids[i] = node.ID
	}
	return ids
}

func (t Timeline) BoundaryAt(runID identity.RunID) (Boundary, error) {
	nodes, err := t.ordered()
	if err != nil {
		return Boundary{}, err
	}
	if index := slices.IndexFunc(nodes, func(n RunNode) bool { return n.ID == runID }); index >= 0 && !nodes[index].IsRoot() {
		return Boundary{}, fmt.Errorf("%w: %q", ErrNotRoot, runID)
	}
	return boundaryAtOrdered(nodes, runID)
}

func runTrees(nodes []RunNode) ([][]RunNode, error) {
	var trees [][]RunNode
	for start := 0; start < len(nodes); {
		if !nodes[start].IsRoot() {
			return nil, fmt.Errorf("timeline starts a Run tree with child %q", nodes[start].ID)
		}
		end := start + 1
		for end < len(nodes) && !nodes[end].IsRoot() {
			end++
		}
		trees = append(trees, nodes[start:end])
		start = end
	}
	return trees, nil
}

func (t Timeline) PortableBoundaryAt(runID identity.RunID) (PortableBoundary, error) {
	nodes, err := t.ordered()
	if err != nil {
		return PortableBoundary{}, err
	}

	trees, err := runTrees(nodes)
	if err != nil {
		return PortableBoundary{}, err
	}
	portable := make([]RunNode, 0, len(nodes))
	targetPortable := runID.IsZero()
	for _, tree := range trees {
		if !slices.ContainsFunc(tree, func(node RunNode) bool { return !node.Terminal }) {
			portable = append(portable, tree...)
			targetPortable = targetPortable || slices.ContainsFunc(tree, func(node RunNode) bool { return node.ID == runID })
		}
	}
	if !targetPortable {
		return PortableBoundary{}, ErrRunNotFound
	}
	if len(portable) == 0 {
		return PortableBoundary{KeepMessageMark: run.MessageMarkAt(0)}, nil
	}
	if runID.IsZero() {
		runID = portable[len(portable)-1].ID
	}
	boundary, err := boundaryAtOrdered(portable, runID)
	if err != nil {
		return PortableBoundary{}, err
	}
	kept := portable[:len(portable)-len(boundary.Dropped)]
	runIDs := make([]identity.RunID, len(kept))
	for index, node := range kept {
		runIDs[index] = node.ID
	}
	return PortableBoundary{
		KeepMessageMark: boundary.KeepMessageMark,
		KeepRunID:       boundary.KeepRunID,
		RunIDs:          runIDs,
	}, nil
}

// Ordering by time alone would let a child whose clock ties another root's land in
// the wrong tree; identity breaks ties.
func (t Timeline) ordered() ([]RunNode, error) {
	byTime := func(a, b RunNode) int { return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), a.ID.Compare(b.ID)) }
	var roots []RunNode
	children := make(map[identity.RunID][]RunNode)
	for index, node := range t {
		if err := node.Lineage.Validate(node.ID); err != nil {
			return nil, fmt.Errorf("timeline Run[%d]: %w", index, err)
		}
		if node.IsRoot() {
			roots = append(roots, node)
		} else {
			children[node.Lineage.RootRunID()] = append(children[node.Lineage.RootRunID()], node)
		}
	}
	slices.SortFunc(roots, byTime)
	nodes := make([]RunNode, 0, len(t))
	for _, root := range roots {
		tree := children[root.ID]
		slices.SortFunc(tree, byTime)
		nodes = append(append(nodes, root), tree...)
		delete(children, root.ID)
	}
	if len(children) > 0 {
		orphan := children[slices.MinFunc(slices.Collect(maps.Keys(children)), identity.RunID.Compare)][0]
		return nil, fmt.Errorf("timeline child Run %q has no root %q", orphan.ID, orphan.Lineage.RootRunID())
	}
	return nodes, nil
}

func boundaryAtOrdered(nodes []RunNode, runID identity.RunID) (Boundary, error) {
	if runID.IsZero() {
		return Boundary{KeepMessageMark: run.MessageMarkAt(0), Dropped: nodes}, nil
	}
	if err := runID.Validate(); err != nil {
		return Boundary{}, fmt.Errorf("timeline boundary: %w", err)
	}
	idx := slices.IndexFunc(nodes, func(n RunNode) bool { return n.ID == runID })
	if idx < 0 {
		return Boundary{}, ErrRunNotFound
	}
	for k := idx + 1; k < len(nodes); k++ {
		if nodes[k].IsRoot() {
			kept := keptTreeRoot(nodes[:k])
			return Boundary{
				KeepMessageMark: kept.MessageMark,
				KeepRunID:       kept.ID,
				Dropped:         slices.Clone(nodes[k:]),
			}, nil
		}
	}
	kept := keptTreeRoot(nodes)
	return Boundary{KeepMessageMark: kept.MessageMark, KeepRunID: kept.ID}, nil
}

func keptTreeRoot(nodes []RunNode) RunNode {
	for index := len(nodes) - 1; index >= 0; index-- {
		if nodes[index].IsRoot() {
			return nodes[index]
		}
	}
	return nodes[len(nodes)-1]
}

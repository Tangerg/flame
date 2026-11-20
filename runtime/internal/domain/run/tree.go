package run

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// ErrInvalidTree reports a set of Run identities that cannot form one
// complete tree under the declared root.
var ErrInvalidTree = errors.New("run: invalid tree")

// TreeMember is the identity-only input used to assemble a Tree. Host
// state, persistence records, executor state, and presentation values do not
// belong here: topology is the one fact shared by all of those representations.
type TreeMember struct {
	RunID   identity.RunID
	Lineage Lineage
}

type runTreeInterval struct {
	start int
	end   int
}

const (
	runTreeUnvisited uint8 = iota
	runTreeVisiting
	runTreeVisited
)

type runTreeBuilder struct {
	rootRunID identity.RunID
	lineages  map[identity.RunID]Lineage
	children  map[identity.RunID][]identity.RunID
	states    map[identity.RunID]uint8
	tree      Tree
}

// Tree is an immutable, validated Run topology. Its canonical order is
// postorder: descendants before ancestors, siblings ordered lexically by Run
// ID, and the root last.
type Tree struct {
	postorder []identity.RunID
	intervals map[identity.RunID]runTreeInterval
}

// NewTree assembles one complete tree under rootRunID. Every member must be
// present exactly once, every child must name the same root and an existing
// parent, and no disconnected component or cycle is accepted.
func NewTree(rootRunID identity.RunID, members []TreeMember) (Tree, error) {
	if err := rootRunID.Validate(); err != nil {
		return Tree{}, fmt.Errorf("%w: root %v", ErrInvalidTree, err)
	}
	if len(members) == 0 {
		return Tree{}, fmt.Errorf("%w: tree has no members", ErrInvalidTree)
	}

	builder := newRunTreeBuilder(rootRunID, len(members))
	for index, member := range members {
		if err := builder.addMember(index, member); err != nil {
			return Tree{}, err
		}
	}
	if _, rootFound := builder.lineages[rootRunID]; !rootFound {
		return Tree{}, fmt.Errorf("%w: root run %q is missing", ErrInvalidTree, rootRunID)
	}
	if err := builder.validateParents(); err != nil {
		return Tree{}, err
	}
	return builder.build()
}

func newRunTreeBuilder(rootRunID identity.RunID, size int) *runTreeBuilder {
	return &runTreeBuilder{
		rootRunID: rootRunID,
		lineages:  make(map[identity.RunID]Lineage, size),
		children:  make(map[identity.RunID][]identity.RunID, size),
		states:    make(map[identity.RunID]uint8, size),
		tree: Tree{
			postorder: make([]identity.RunID, 0, size),
			intervals: make(map[identity.RunID]runTreeInterval, size),
		},
	}
}

func (r *runTreeBuilder) addMember(index int, member TreeMember) error {
	if err := member.Lineage.Validate(member.RunID); err != nil {
		return fmt.Errorf(
			"%w: member[%d] run %q lineage: %w",
			ErrInvalidTree,
			index,
			member.RunID,
			err,
		)
	}
	if _, duplicate := r.lineages[member.RunID]; duplicate {
		return fmt.Errorf("%w: duplicate run %q", ErrInvalidTree, member.RunID)
	}
	r.lineages[member.RunID] = member.Lineage

	if member.Lineage.IsRoot() {
		if member.RunID != r.rootRunID {
			return fmt.Errorf(
				"%w: run %q has root lineage, want only %q",
				ErrInvalidTree,
				member.RunID,
				r.rootRunID,
			)
		}
		return nil
	}
	if member.RunID == r.rootRunID {
		return fmt.Errorf(
			"%w: root run %q carries child lineage",
			ErrInvalidTree,
			r.rootRunID,
		)
	}
	if member.Lineage.RootRunID() != r.rootRunID {
		return fmt.Errorf(
			"%w: child run %q names root %q, want %q",
			ErrInvalidTree,
			member.RunID,
			member.Lineage.RootRunID(),
			r.rootRunID,
		)
	}
	r.children[member.Lineage.ParentRunID()] = append(
		r.children[member.Lineage.ParentRunID()],
		member.RunID,
	)
	return nil
}

func (r *runTreeBuilder) validateParents() error {
	for runID, lineage := range r.lineages {
		if lineage.IsRoot() {
			continue
		}
		if _, exists := r.lineages[lineage.ParentRunID()]; !exists {
			return fmt.Errorf(
				"%w: child run %q names unknown parent %q",
				ErrInvalidTree,
				runID,
				lineage.ParentRunID(),
			)
		}
	}
	for parentRunID := range r.children {
		slices.SortFunc(r.children[parentRunID], identity.RunID.Compare)
	}
	return nil
}

func (r *runTreeBuilder) build() (Tree, error) {
	if err := r.visit(r.rootRunID); err != nil {
		return Tree{}, err
	}
	if len(r.tree.postorder) != len(r.lineages) {
		// With exactly one root-shaped member and every parent present, any
		// disconnected component necessarily contains a cycle. Traverse it so
		// callers receive the precise invariant violation.
		runIDs := make([]identity.RunID, 0, len(r.lineages))
		for runID := range r.lineages {
			runIDs = append(runIDs, runID)
		}
		slices.SortFunc(runIDs, identity.RunID.Compare)
		for _, runID := range runIDs {
			if r.states[runID] == runTreeUnvisited {
				if err := r.visit(runID); err != nil {
					return Tree{}, err
				}
			}
		}
		return Tree{}, fmt.Errorf("%w: tree contains a Run disconnected from root %q", ErrInvalidTree, r.rootRunID)
	}
	return r.tree, nil
}

func (r *runTreeBuilder) visit(runID identity.RunID) error {
	switch r.states[runID] {
	case runTreeVisiting:
		return fmt.Errorf("%w: tree contains a cycle at run %q", ErrInvalidTree, runID)
	case runTreeVisited:
		return nil
	}
	r.states[runID] = runTreeVisiting
	start := len(r.tree.postorder)
	for _, childRunID := range r.children[runID] {
		if err := r.visit(childRunID); err != nil {
			return err
		}
	}
	r.tree.postorder = append(r.tree.postorder, runID)
	r.tree.intervals[runID] = runTreeInterval{start: start, end: len(r.tree.postorder)}
	r.states[runID] = runTreeVisited
	return nil
}

// Postorder returns a defensive copy of the complete canonical Run order.
func (t Tree) Postorder() []identity.RunID {
	return slices.Clone(t.postorder)
}

// SubtreePostorder returns a defensive copy of runID and all its descendants in
// canonical postorder. The boolean is false when runID is not a tree member.
func (t Tree) SubtreePostorder(runID identity.RunID) ([]identity.RunID, bool) {
	interval, exists := t.intervals[runID]
	if !exists {
		return nil, false
	}
	return slices.Clone(t.postorder[interval.start:interval.end]), true
}

// CheckCancellation admits a snapshot only while every open member belongs to
// the same execution phase as its root. Executor bindings are acquired later.
func (t Tree) CheckCancellation(targetID identity.RunID, members map[identity.RunID]Run) error {
	if len(t.postorder) == 0 || len(t.postorder) != len(members) {
		return fmt.Errorf("%w: incomplete cancellation tree", ErrInvalidTree)
	}
	root := members[t.postorder[len(t.postorder)-1]]
	if !root.State().AdvanceProgress() {
		return fmt.Errorf("%w: root Run %q is %s", ErrInvalidTree, root.ID(), root.State())
	}
	target, found := members[targetID]
	if !found || target.State().IsTerminal() {
		return fmt.Errorf("%w: target Run %q is missing or finished", ErrInvalidTree, targetID)
	}
	for _, id := range t.postorder {
		member, found := members[id]
		if !found || member.ID() != id {
			return fmt.Errorf("%w: missing Run %q", ErrInvalidTree, id)
		}
		if !member.State().IsTerminal() && member.State() != root.State() {
			return fmt.Errorf("%w: non-terminal Run %q is %s while root %q is %s", ErrInvalidTree, id, member.State(), root.ID(), root.State())
		}
	}
	return nil
}

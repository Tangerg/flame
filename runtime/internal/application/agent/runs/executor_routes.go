package runs

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	corechat "github.com/Tangerg/scope/core/chat"
)

type executorRoute struct {
	member           ExecutorMember
	memberBound      bool
	runID            identity.RunID
	segmentID        identity.SegmentID
	rootRunID        identity.RunID
	lineage          rundomain.Lineage
	reducer          *reducer
	resumed          *rundomain.Replacement
	segmentStartedAt time.Time
	boundary         segmentBoundary
}

type executorRoutes struct {
	root           *executorRoute
	byMember       map[string]*executorRoute
	byRunID        map[identity.RunID]*executorRoute
	admissionOrder []*executorRoute
}

func (c *Segments) openingRoutes(spec segmentSpec) (*executorRoutes, error) {
	switch opening := spec.Opening.(type) {
	case rootOpening:
		return c.rootExecutorRoutes(spec, opening)
	case treeResumption:
		return c.resumedExecutorRoutes(spec, opening)
	default:
		return nil, fmt.Errorf("runs: unknown Segment opening %T", spec.Opening)
	}
}

func (c *Segments) rootExecutorRoutes(spec segmentSpec, opening rootOpening) (*executorRoutes, error) {
	if opening.ModelOnlyInput && opening.GoalIncarnationID.IsZero() {
		return nil, errors.New("runs: model-only opening input requires a Goal root")
	}
	if len(opening.Input) > 0 && (opening.ConversationInput == nil ||
		opening.ConversationInput.Role != corechat.RoleUser || opening.ConversationInput.Validate() != nil) {
		return nil, errors.New("runs: fresh root requires its exact composed conversation input")
	}
	opened, err := rundomain.Admit(opening.draft(spec))
	if err != nil {
		return nil, fmt.Errorf("runs: admit root: %w", err)
	}
	root := &executorRoute{
		runID:     spec.RunID,
		segmentID: spec.SegmentID,
		rootRunID: spec.RunID,
		reducer: newReducer(reducerConfig{
			Opened: opened, WorkspaceCWD: spec.WorkspaceCWD, Isolated: spec.Isolated,
			UserInput: opening.Input, ConversationInput: opening.ConversationInput, ModelOnlyInput: opening.ModelOnlyInput,
			Now: c.publications.nowUTC,
		}),
	}
	return &executorRoutes{
		root:           root,
		byMember:       make(map[string]*executorRoute),
		byRunID:        map[identity.RunID]*executorRoute{root.runID: root},
		admissionOrder: []*executorRoute{root},
	}, nil
}

func (e *executorRoutes) unfinishedInPostorder() ([]*executorRoute, error) {
	if e == nil || e.root == nil {
		return nil, errors.New("runs: executor routes have no root")
	}
	byRunID := make(map[identity.RunID]*executorRoute, len(e.admissionOrder))
	members := make([]rundomain.TreeMember, 0, len(e.admissionOrder))
	for _, route := range e.admissionOrder {
		if route == nil {
			return nil, errors.New("runs: executor routes contain a nil route")
		}
		byRunID[route.runID] = route
		members = append(members, rundomain.TreeMember{
			RunID:   route.runID,
			Lineage: route.lineage,
		})
	}
	tree, err := rundomain.NewTree(e.root.runID, members)
	if err != nil {
		return nil, fmt.Errorf("runs: executor routes: %w", err)
	}
	ordered := make([]*executorRoute, 0, e.unfinishedCount())
	for _, runID := range tree.Postorder() {
		route := byRunID[runID]
		if route == nil {
			return nil, fmt.Errorf("runs: executor tree ordered unknown run %q", runID)
		}
		if !route.boundary.finished() {
			if route.lineage.IsChild() {
				parent := byRunID[route.lineage.ParentRunID()]
				if parent == nil {
					return nil, fmt.Errorf(
						"runs: active executor route %q has no parent route %q",
						route.runID,
						route.lineage.ParentRunID(),
					)
				}
				if parent.boundary.finished() {
					return nil, fmt.Errorf(
						"runs: active executor route %q descends from finished route %q",
						route.runID,
						parent.runID,
					)
				}
			}
			ordered = append(ordered, route)
		}
	}
	return ordered, nil
}

func (e *executorRoutes) unfinishedCount() int {
	count := 0
	for _, route := range e.admissionOrder {
		if !route.boundary.finished() {
			count++
		}
	}
	return count
}

func (e *executorRoute) activeDuration(boundary time.Time) time.Duration {
	if e.segmentStartedAt.IsZero() || boundary.Before(e.segmentStartedAt) {
		return 0
	}
	return boundary.Sub(e.segmentStartedAt)
}

func (e *executorRoutes) resolve(member ExecutorMember) (*executorRoute, error) {
	if member.Child() {
		return e.resolveChild(member)
	}
	return e.resolveRoot(member)
}

func (e *executorRoutes) resolveChild(member ExecutorMember) (*executorRoute, error) {
	if member.SpawnCallID == "" {
		return nil, fmt.Errorf("runs: child executor member %q has no spawn-call identity", member.MemberID)
	}
	route := e.byMember[member.MemberID]
	if route == nil {
		return nil, fmt.Errorf("runs: child executor member %q has no admitted child run", member.MemberID)
	}
	if !route.lineage.IsChild() {
		return nil, fmt.Errorf("runs: root Run %q emitted a child executor member", route.runID)
	}
	if err := e.bindChildMember(route, member); err != nil {
		return nil, err
	}
	if route.boundary.finished() {
		return nil, fmt.Errorf("runs: child executor member %q published after its segment finished", member.MemberID)
	}
	return route, nil
}

// A resumed child learns its executor member from its first publication; from
// then on the member is immutable lineage.
func (e *executorRoutes) bindChildMember(route *executorRoute, member ExecutorMember) error {
	if route.memberBound {
		if route.member != member {
			return fmt.Errorf("runs: child executor member %q changed immutable lineage", member.MemberID)
		}
		return nil
	}
	parent := e.byRunID[route.lineage.ParentRunID()]
	if parent == nil || parent.member.MemberID == "" {
		return fmt.Errorf(
			"runs: resumed child Run %q has no bound parent Run %q",
			route.runID,
			route.lineage.ParentRunID(),
		)
	}
	if member.ParentID != parent.member.MemberID {
		return fmt.Errorf(
			"runs: resumed child executor member %q names parent %q, want Run %q member %q",
			member.MemberID,
			member.ParentID,
			parent.runID,
			parent.member.MemberID,
		)
	}
	route.member, route.memberBound = member, true
	return nil
}

func (e *executorRoutes) resolveRoot(member ExecutorMember) (*executorRoute, error) {
	if e.root.memberBound {
		if e.root.member != member {
			return nil, fmt.Errorf(
				"runs: root executor member changed from %q to %q",
				e.root.member.MemberID,
				member.MemberID,
			)
		}
		return e.root, nil
	}
	e.root.member, e.root.memberBound = member, true
	if member.MemberID != "" {
		e.byMember[member.MemberID] = e.root
	}
	return e.root, nil
}

func (e *executorRoutes) parent(member ExecutorMember) (*executorRoute, error) {
	if !member.Child() {
		return nil, errors.New("runs: child opening request requires a child executor member")
	}
	if member.SpawnCallID == "" {
		return nil, fmt.Errorf("runs: child executor member %q has no spawn-call identity", member.MemberID)
	}
	if e.byMember[member.MemberID] != nil {
		return nil, fmt.Errorf("runs: child executor member %q opened more than once", member.MemberID)
	}
	parent := e.byMember[member.ParentID]
	if parent == nil {
		return nil, fmt.Errorf(
			"runs: child executor member %q references unknown parent member %q",
			member.MemberID,
			member.ParentID,
		)
	}
	if parent.member.MemberID != member.ParentID {
		return nil, fmt.Errorf("runs: child executor member %q parent route is inconsistent", member.MemberID)
	}
	if parent.reducer == nil {
		return nil, fmt.Errorf(
			"runs: child executor member %q parent member %q has no active reducer",
			member.MemberID,
			member.ParentID,
		)
	}
	if parent.boundary.finished() {
		return nil, fmt.Errorf(
			"runs: child executor member %q parent member %q already finished",
			member.MemberID,
			member.ParentID,
		)
	}
	return parent, nil
}

func (e *executorRoutes) installChild(member ExecutorMember, route *executorRoute) {
	route.member = member
	route.memberBound = true
	e.byMember[member.MemberID] = route
	e.byRunID[route.runID] = route
	e.admissionOrder = append(e.admissionOrder, route)
}

func (e *executorRoutes) abortUnfinished(cause error) {
	for _, route := range e.admissionOrder {
		if !route.boundary.finished() && route.reducer != nil {
			route.reducer.abort(cause)
		}
	}
}

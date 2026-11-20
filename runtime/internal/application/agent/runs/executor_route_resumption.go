package runs

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func (c *Segments) resumedExecutorRoutes(spec segmentSpec, resumption treeResumption) (*executorRoutes, error) {
	continuation := resumption.Continuation
	if err := validateResumedRouteRequest(spec, continuation); err != nil {
		return nil, err
	}
	builder := resumedRouteBuilder{
		spec:         spec,
		resumption:   resumption,
		newSegmentID: c.newSegmentID,
		now:          c.publications.nowUTC,
		resumedAt:    c.publications.nowUTC(),
		routes: &executorRoutes{
			byMember: make(map[string]*executorRoute, len(continuation.continuations)),
			byRunID:  make(map[identity.RunID]*executorRoute, len(continuation.continuations)),
		},
		segmentIDs: map[identity.SegmentID]struct{}{spec.SegmentID: {}},
	}
	return builder.build()
}

func validateResumedRouteRequest(spec segmentSpec, continuation *treeContinuation) error {
	if continuation == nil {
		return errors.New("runs: resumed routes require a tree continuation")
	}
	if err := continuation.validate(); err != nil {
		return fmt.Errorf("runs: build resumed routes: %w", err)
	}
	if continuation.rootRunID != spec.RunID || continuation.sessionID() != spec.SessionID {
		return fmt.Errorf(
			"runs: resumed route scope %q/%q does not match continuation %q/%q",
			spec.SessionID,
			spec.RunID,
			continuation.sessionID(),
			continuation.rootRunID,
		)
	}
	if spec.SegmentID.IsZero() {
		return errors.New("runs: resumed root segment id is required")
	}
	return nil
}

// resumedRouteBuilder reconstructs every per-Run reducer from one validated
// continuation and derives their deterministic admission order. Keeping the
// segment identity set with the construction state makes duplicate detection
// structural rather than a convention shared by unrelated helpers.
type resumedRouteBuilder struct {
	spec         segmentSpec
	resumption   treeResumption
	newSegmentID func() identity.SegmentID
	now          func() time.Time
	// resumedAt is the one opening time every Run in the tree resumes at.
	resumedAt  time.Time
	routes     *executorRoutes
	segmentIDs map[identity.SegmentID]struct{}
}

func (r *resumedRouteBuilder) build() (*executorRoutes, error) {
	for _, continuationState := range r.resumption.Continuation.continuations {
		route, err := r.newRoute(continuationState)
		if err != nil {
			return nil, err
		}
		r.routes.byMember[route.member.MemberID] = route
		r.routes.byRunID[route.runID] = route
		if route.runID == r.resumption.Continuation.rootRunID {
			r.routes.root = route
		}
	}
	if r.routes.root == nil {
		return nil, errors.New("runs: resumed tree has no root route")
	}
	if err := r.orderRoutesPreorder(); err != nil {
		return nil, err
	}
	return r.routes, nil
}

// restoreSegment continues the parked Segment itself: its Items keep their
// identities and its open Tool attempts wait for the executor to replay them.
func (r *resumedRouteBuilder) restoreSegment(reducer *reducer, runID identity.RunID, segmentID identity.SegmentID) error {
	var ids []identity.ItemID
	for _, item := range r.resumption.Continuation.items {
		if item.RunID() == runID {
			ids = append(ids, item.ID())
		}
	}
	if err := reducer.itemIDs.Restore(ids); err != nil {
		return err
	}
	reducer.restoredToolAttempts = make(map[string]OpenToolInvocation)
	for _, attempt := range r.resumption.toolAttempts {
		if attempt.RunID == runID && attempt.SegmentID == segmentID {
			reducer.restoredToolAttempts[attempt.CallID] = attempt
		}
	}
	return nil
}

func (r *resumedRouteBuilder) newRoute(continuationState Continuation) (*executorRoute, error) {
	segmentID, err := r.segmentIDFor(continuationState.RunID)
	if err != nil {
		return nil, err
	}
	member := ExecutorMember{MemberID: continuationState.MemberID}
	if err := member.Validate(); err != nil {
		return nil, fmt.Errorf("runs: resumed Run %q member: %w", continuationState.RunID, err)
	}
	parked, found := r.resumption.Continuation.run(continuationState.RunID)
	if !found {
		return nil, fmt.Errorf("runs: resumed Run %q is not parked", continuationState.RunID)
	}
	opened := parked
	var resumed *rundomain.Replacement
	if !r.spec.restoresSegments() {
		replacement, err := parked.Replace().Resume(segmentID, r.resumedAt)
		if err != nil {
			return nil, fmt.Errorf("runs: resume Run %q: %w", continuationState.RunID, err)
		}
		resumed, opened = &replacement, replacement.State()
	}
	userInput := []transcript.ContentBlock(nil)
	if continuationState.RunID == r.resumption.Continuation.rootRunID {
		userInput = r.resumption.Input
	}
	reducer := newReducer(reducerConfig{
		Opened: opened, WorkspaceCWD: r.spec.WorkspaceCWD, Isolated: r.spec.Isolated,
		UserInput: userInput, Continuation: r.resumption.Continuation, Now: r.now,
	})
	if r.spec.restoresSegments() {
		if err := r.restoreSegment(reducer, opened.ID(), segmentID); err != nil {
			return nil, err
		}
	}
	for _, recovered := range r.resumption.Continuation.recoveredTools {
		if recovered.Invocation.RunID != opened.ID() {
			continue
		}
		if recovered.Invocation.SegmentID != segmentID {
			return nil, errors.New("runs: recovered Tool changed its owner")
		}
		if err := reducer.adoptToolAttempt(recovered.Start, recovered.Invocation); err != nil {
			return nil, err
		}
	}
	return &executorRoute{
		member:      member,
		memberBound: parked.Lineage().IsRoot(),
		runID:       continuationState.RunID,
		segmentID:   segmentID,
		rootRunID:   r.resumption.Continuation.rootRunID,
		lineage:     parked.Lineage(),
		resumed:     resumed,
		reducer:     reducer,
	}, nil
}

func (r *resumedRouteBuilder) segmentIDFor(runID identity.RunID) (identity.SegmentID, error) {
	if r.spec.restoresSegments() {
		value, found := r.resumption.Continuation.run(runID)
		if !found || value.State() != rundomain.Running {
			return identity.SegmentID{}, errors.New("runs: accepted member is not running")
		}
		return value.ActiveSegmentID(), nil
	}
	if runID == r.resumption.Continuation.rootRunID {
		return r.spec.SegmentID, nil
	}
	segmentID := r.newSegmentID()
	if _, duplicate := r.segmentIDs[segmentID]; duplicate {
		return identity.SegmentID{}, fmt.Errorf("runs: resumed tree generated duplicate segment %q", segmentID)
	}
	r.segmentIDs[segmentID] = struct{}{}
	return segmentID, nil
}

func (r *resumedRouteBuilder) orderRoutesPreorder() error {
	childrenByParentRunID := make(map[identity.RunID][]*executorRoute, len(r.routes.byRunID))
	for _, route := range r.routes.byRunID {
		if route != r.routes.root {
			childrenByParentRunID[route.lineage.ParentRunID()] = append(
				childrenByParentRunID[route.lineage.ParentRunID()],
				route,
			)
		}
	}
	for parentRunID := range childrenByParentRunID {
		slices.SortFunc(childrenByParentRunID[parentRunID], func(left, right *executorRoute) int {
			return left.runID.Compare(right.runID)
		})
	}
	var appendPreorder func(*executorRoute)
	appendPreorder = func(route *executorRoute) {
		r.routes.admissionOrder = append(r.routes.admissionOrder, route)
		for _, child := range childrenByParentRunID[route.runID] {
			appendPreorder(child)
		}
	}
	appendPreorder(r.routes.root)
	if len(r.routes.admissionOrder) != len(r.resumption.Continuation.continuations) {
		return errors.New("runs: resumed route tree is disconnected")
	}
	return nil
}

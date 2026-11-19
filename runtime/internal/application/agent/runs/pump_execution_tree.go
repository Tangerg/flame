package runs

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// commitExecutionTree stages a settled tree against cloned reducers, commits
// it in one transaction, and only then activates children and publishes.
func (s *segmentPump) commitExecutionTree(tree ExecutionTreeSettled) error {
	if err := tree.Update.Validate(); err != nil {
		return err
	}
	if err := s.bindTreeRoot(tree.Update); err != nil {
		return err
	}
	for _, acceptance := range tree.InputAcceptances {
		if err := tree.Update.validateSteerAcceptance(acceptance); err != nil {
			return err
		}
		if acceptance.RunID != s.routes.root.runID {
			return errors.New("runs: foreign steer acceptance")
		}
	}
	commit := ExecutionTreeCommit{Update: tree.Update, Context: tree.Context, InputAcceptances: tree.InputAcceptances}
	children, startedRoutes, err := s.stageChildPublications(tree.Children, &commit)
	if err != nil {
		return err
	}
	projections, err := s.stageTreeFacts(tree.Facts, startedRoutes, &commit)
	if err != nil {
		return err
	}
	write := func() error { return s.segments.publications.events.CommitExecutionTree(s.ownerCtx, commit) }
	retryOnFailure := slices.ContainsFunc(commit.Events, func(event EventCommit) bool {
		return event.ObsoleteCheckpointRootID != "" || event.childCancellation != nil
	})
	if err := commitRunBoundary(s.ownerCtx, s.spec.SessionID, s.spec.RunID, write, retryOnFailure); err != nil {
		return err
	}
	if err := s.settleChildStarts(children, startedRoutes); err != nil {
		return err
	}
	for _, projected := range projections {
		if err := s.publishTreeProjection(projected); err != nil {
			return err
		}
	}
	return nil
}

// bindTreeRoot binds the first tree activation to the product root and
// rejects a tree owned by another Session or root.
func (s *segmentPump) bindTreeRoot(update ExecutionTreeUpdate) error {
	if s.routes.root.member.MemberID == "" {
		member := ExecutorMember{MemberID: update.Head.RootID}
		route, err := s.routes.resolve(member)
		if err != nil || route != s.routes.root {
			return errors.Join(errors.New("runs: tree activation has no product root"), err)
		}
		if err := s.owner.bindExecutorMember(route.runID, member.MemberID); err != nil {
			return err
		}
	}
	if update.Head.SessionID != s.spec.SessionID || update.Head.RootID != s.routes.root.member.MemberID {
		return fmt.Errorf("runs: foreign execution tree")
	}
	return nil
}

// stageChildPublications matches each published child to its reservation. A
// repeated publication must agree with the outcome already settled.
func (s *segmentPump) stageChildPublications(publications []ChildRunPublication, commit *ExecutionTreeCommit) ([]*managedChildStart, map[string]*executorRoute, error) {
	startedRoutes := make(map[string]*executorRoute)
	var children []*managedChildStart
	seen := make(map[string]struct{})
	for _, publication := range publications {
		if _, duplicate := seen[publication.Binding.MemberID]; duplicate {
			return nil, nil, fmt.Errorf("runs: duplicate child publication")
		}
		seen[publication.Binding.MemberID] = struct{}{}
		if err := publication.Binding.Validate(); err != nil {
			return nil, nil, err
		}
		if !publication.Outcome.Valid() {
			return nil, nil, fmt.Errorf("runs: invalid child publication outcome")
		}
		managed := s.childStarts[publication.Binding.MemberID]
		if managed == nil || managed.prepared.reservation.Binding != publication.Binding {
			return nil, nil, fmt.Errorf("runs: published child has no matching reservation")
		}
		if managed.outcome.Valid() {
			if managed.outcome != publication.Outcome {
				return nil, nil, fmt.Errorf("runs: contradictory child publication")
			}
			continue
		}
		prepared := managed.prepared
		if publication.Outcome != ChildRunStarted {
			commit.AbortedChildren = append(commit.AbortedChildren, prepared.reservation)
			children = append(children, managed)
			continue
		}
		if prepared.route.reducer == nil {
			if err := s.segments.finalizeChildOpening(s.spec, s.owner, prepared, s.segments.publications.nowUTC()); err != nil {
				return nil, nil, err
			}
		}
		commit.StartedChildren = append(commit.StartedChildren, prepared.started)
		startedRoutes[prepared.member.MemberID] = prepared.route
		children = append(children, managed)
	}
	return children, startedRoutes, nil
}

// stageTreeFacts reduces each fact on a clone of its route's reducer; nothing
// becomes visible until the tree commit succeeds.
func (s *segmentPump) stageTreeFacts(facts []ExecutorEvent, startedRoutes map[string]*executorRoute, commit *ExecutionTreeCommit) ([]routedTreeProjection, error) {
	var projections []routedTreeProjection
	staged := make(map[*executorRoute]*reducer)
	unfinishedChildren := s.routes.unfinishedCount() - 1 + len(startedRoutes)
	for _, event := range facts {
		route, err := s.treeFactRoute(event, startedRoutes)
		if err != nil {
			return nil, err
		}
		fact, ok := event.Payload.(ExecutionFact)
		if !ok {
			return nil, fmt.Errorf("runs: execution tree carries %T", event.Payload)
		}
		candidate := staged[route]
		if candidate == nil {
			candidate = route.reducer.clone()
		}
		batch, err := candidate.reduce(s.classifyChildCancellationFact(route, fact, commit.Events))
		if err != nil {
			return nil, err
		}
		var eventCommit EventCommit
		if engineEventEndsSegment(fact) {
			eventCommit, err = combineTerminalEventCommit(batch)
		} else {
			eventCommit, err = combineAuthoritativeCommit(route, s.spec.SessionID, batch)
		}
		if err != nil {
			return nil, err
		}
		if eventCommit.State == StateTerminalize {
			if route == s.routes.root {
				if unfinishedChildren != 0 {
					return nil, errors.New("runs: confirmed root cancellation precedes child terminals")
				}
				eventCommit.ObsoleteCheckpointRootID = commit.Update.Head.RootID
				s.publisher.bindRootCancellationReceipt(&eventCommit)
			} else {
				unfinishedChildren--
			}
		}
		s.owner.bindChildCancellationReceipt(&eventCommit, commit.Events)
		commit.Events = append(commit.Events, eventCommit)
		staged[route] = candidate
		projections = append(projections, routedTreeProjection{route, candidate, batch})
	}
	return projections, nil
}

// treeFactRoute resolves a settlement fact's route. Known root cancellation
// closes with its Scope Exit so that a crash cannot leave the Run still open.
func (s *segmentPump) treeFactRoute(event ExecutorEvent, startedRoutes map[string]*executorRoute) (*executorRoute, error) {
	route := startedRoutes[event.Member.MemberID]
	if route != nil && route.member != event.Member {
		return nil, fmt.Errorf("runs: newly published child changed immutable lineage")
	}
	if route == nil {
		var err error
		route, err = s.routes.resolve(event.Member)
		if err != nil {
			return nil, err
		}
	}
	switch fact := event.Payload.(type) {
	case ToolResultsCommitted:
	case SegmentEnded:
		if route == s.routes.root && (fact.Ending.Outcome() != run.OutcomeCanceled || len(fact.unresolvedEffects) > 0) {
			return nil, errors.New("runs: tree settlement requires confirmed root cancellation")
		}
	default:
		return nil, fmt.Errorf("runs: unsupported tree settlement fact %T", event.Payload)
	}
	return route, nil
}

func (s *segmentPump) settleChildStarts(children []*managedChildStart, startedRoutes map[string]*executorRoute) error {
	for _, managed := range children {
		prepared := managed.prepared
		if startedRoutes[prepared.member.MemberID] == nil {
			managed.outcome = ChildRunStartAborted
			prepared.releaseBinding(s.owner)
			continue
		}
		managed.outcome = ChildRunStarted
		s.segments.activatePreparedChild(s.spec, s.routes, prepared)
		for _, reduced := range prepared.batch.events {
			if err := s.publisher.append(prepared.route, reduced); err != nil {
				return err
			}
		}
	}
	return nil
}

type routedTreeProjection struct {
	route   *executorRoute
	reducer *reducer
	batch   reductionBatch
}

func (s *segmentPump) publishTreeProjection(projected routedTreeProjection) error {
	projected.route.reducer = projected.reducer
	for _, reduced := range projected.batch.events {
		if reduced.Commit != nil && reduced.Commit.State == StateTerminalize {
			projected.route.boundary = boundaryFinished
			s.owner.recordTerminalRun(*reduced.Commit.Run)
			s.segments.publications.publishRunMoved(s.spec.SessionID, projected.route.runID)
			if reduced.Commit.ChargesGoal() {
				s.segments.publications.publishGoalMoved(s.spec.SessionID)
			}
		}
	}
	for _, reduced := range projected.batch.events {
		if reduced.Commit != nil {
			for _, write := range reduced.Commit.Items {
				s.owner.recordChildCancellationItem(projected.route.runID, write.Item())
			}
		}
		if err := s.publisher.append(projected.route, reduced); err != nil {
			return err
		}
	}
	return nil
}

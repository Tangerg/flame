package runs

import (
	"errors"
	"fmt"
)

func (s *segmentPump) commitExecutionTree(tree ExecutionTreeSettled) error {
	if err := tree.Update.Validate(); err != nil {
		return err
	}
	if s.routes.root.member.MemberID == "" {
		member := ExecutorMember{MemberID: tree.Update.Head.RootID}
		route, err := s.routes.resolve(member)
		if err != nil || route != s.routes.root {
			return errors.Join(errors.New("runs: tree activation has no product root"), err)
		}
		if err := s.owner.bindExecutorMember(route.runID, member.MemberID); err != nil {
			return err
		}
	}
	if tree.Update.Head.SessionID != s.spec.SessionID || tree.Update.Head.RootID != s.routes.root.member.MemberID {
		return fmt.Errorf("runs: foreign execution tree")
	}
	commit := ExecutionTreeCommit{Update: tree.Update, Context: tree.Context, InputAcceptances: tree.InputAcceptances}
	for _, acceptance := range tree.InputAcceptances {
		if err := acceptance.Validate(); err != nil {
			return err
		}
		if acceptance.Operation != CommandSteerInput || acceptance.SessionID != s.spec.SessionID || acceptance.RunID != s.routes.root.runID {
			return errors.New("runs: foreign steer acceptance")
		}
	}
	startedRoutes := make(map[string]*executorRoute)
	var children []*managedChildStart
	seen := make(map[string]struct{})
	for _, publication := range tree.Children {
		if _, duplicate := seen[publication.Binding.MemberID]; duplicate {
			return fmt.Errorf("runs: duplicate child publication")
		}
		seen[publication.Binding.MemberID] = struct{}{}
		if err := publication.Binding.Validate(); err != nil {
			return err
		}
		if !publication.Outcome.Valid() {
			return fmt.Errorf("runs: invalid child publication outcome")
		}
		managed := s.childStarts[publication.Binding.MemberID]
		if managed == nil || managed.prepared.reservation.Binding != publication.Binding {
			return fmt.Errorf("runs: published child has no matching reservation")
		}
		if managed.outcome.Valid() {
			if managed.outcome != publication.Outcome {
				return fmt.Errorf("runs: contradictory child publication")
			}
			continue
		}
		prepared := managed.prepared
		if publication.Outcome == ChildRunStarted {
			if prepared.route.reducer == nil {
				if err := s.coordinator.finalizeChildOpening(s.spec, s.owner, prepared, s.coordinator.publications.nowUTC()); err != nil {
					return err
				}
			}
			commit.StartedChildren = append(commit.StartedChildren, prepared.started)
			startedRoutes[prepared.member.MemberID] = prepared.route
		} else {
			commit.AbortedChildren = append(commit.AbortedChildren, prepared.reservation)
		}
		children = append(children, managed)
	}
	type projection struct {
		route   *executorRoute
		reducer *reducer
		batch   reductionBatch
	}
	var projections []projection
	staged := make(map[*executorRoute]*reducer)
	for _, event := range tree.Facts {
		route := startedRoutes[event.Member.MemberID]
		if route != nil && route.member != event.Member {
			return fmt.Errorf("runs: newly published child changed immutable lineage")
		}
		if route == nil {
			var err error
			route, err = s.routes.resolve(event.Member)
			if err != nil {
				return err
			}
		}
		switch event.Payload.(type) {
		case ToolResultsCommitted:
		case SegmentEnded:
			if route == s.routes.root {
				return fmt.Errorf("runs: tree settlement cannot close the root projection")
			}
		default:
			return fmt.Errorf("runs: unsupported tree settlement fact %T", event.Payload)
		}
		candidate := staged[route]
		if candidate == nil {
			candidate = route.reducer.clone()
		}
		fact, ok := event.Payload.(ExecutionFact)
		if !ok {
			return fmt.Errorf("runs: execution tree carries %T", event.Payload)
		}
		batch, err := candidate.reduce(s.classifyChildCancellationFact(route, fact))
		if err != nil {
			return err
		}
		var eventCommit EventCommit
		if engineEventEndsSegment(fact) {
			eventCommit, err = combineTerminalEventCommit(batch)
		} else {
			eventCommit, err = combineAuthoritativeCommit(route, s.spec.SessionID, batch)
		}
		if err != nil {
			return err
		}
		commit.Events = append(commit.Events, eventCommit)
		staged[route] = candidate
		projections = append(projections, projection{route, candidate, batch})
	}
	if err := s.coordinator.publications.events.CommitExecutionTree(s.ownerCtx, commit); err != nil {
		return err
	}
	for _, managed := range children {
		prepared := managed.prepared
		if startedRoutes[prepared.member.MemberID] != nil {
			managed.outcome = ChildRunStarted
			s.coordinator.activatePreparedChild(s.spec, s.routes, prepared)
			for _, reduced := range prepared.batch.events {
				if err := s.publisher.append(prepared.route, reduced); err != nil {
					return err
				}
			}
		} else {
			managed.outcome = ChildRunStartAborted
			prepared.releaseBinding(s.owner)
		}
	}
	for _, projected := range projections {
		projected.route.reducer = projected.reducer
		for _, reduced := range projected.batch.events {
			if reduced.Commit != nil && reduced.Commit.State == StateTerminalize {
				projected.route.boundary = boundaryFinished
				s.owner.recordTerminalRun(*reduced.Commit.Run)
				s.coordinator.publications.publishRunMoved(s.spec.SessionID, projected.route.runID)
				if reduced.Commit.ChargesGoal() {
					s.coordinator.publications.publishGoalMoved(s.spec.SessionID)
				}
			}
		}
		for _, reduced := range projected.batch.events {
			if reduced.Commit != nil {
				for _, item := range reduced.Commit.Items {
					s.owner.recordChildCancellationItem(projected.route.runID, item)
				}
			}
			if err := s.publisher.append(projected.route, reduced); err != nil {
				return err
			}
		}
	}
	return nil
}

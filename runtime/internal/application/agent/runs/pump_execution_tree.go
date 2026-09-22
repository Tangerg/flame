package runs

import "fmt"

func (s *segmentPump) commitExecutionTree(tree ExecutionTreeSettled) error {
	if err := tree.Update.Validate(); err != nil {
		return err
	}
	if tree.Update.Head.SessionID != s.spec.SessionID || tree.Update.Head.RootID != s.routes.root.member.MemberID {
		return fmt.Errorf("runs: foreign execution tree")
	}
	type projection struct {
		route   *executorRoute
		reducer *reducer
		batch   reductionBatch
	}
	var projections []projection
	var commits []EventCommit
	staged := make(map[*executorRoute]*reducer)
	for _, event := range tree.Facts {
		route, err := s.routes.resolve(event.Member)
		if err != nil {
			return err
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
		var commit EventCommit
		if engineEventEndsSegment(fact) {
			commit, err = combineTerminalEventCommit(batch)
		} else {
			commit, err = combineAuthoritativeCommit(route, s.spec.SessionID, batch)
		}
		if err != nil {
			return err
		}
		commits = append(commits, commit)
		staged[route] = candidate
		projections = append(projections, projection{route, candidate, batch})
	}
	if err := s.coordinator.publications.events.CommitExecutionTree(s.ownerCtx, tree.Update, commits); err != nil {
		return err
	}
	for _, projected := range projections {
		projected.route.reducer = projected.reducer
		for _, reduced := range projected.batch.events {
			if reduced.Commit != nil && reduced.Commit.State == StateTerminalize {
				projected.route.segmentFinished = true
				s.owner.recordTerminalRun(*reduced.Commit.Run)
				s.coordinator.publications.publishRunMoved(s.spec.SessionID, projected.route.runID)
				if reduced.Commit.GoalRun != nil {
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

package runs

import (
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func validateRouteReductionBatch(
	route *executorRoute,
	sessionID identity.SessionID,
	batch reductionBatch,
) error {
	if route.runID.IsZero() || route.segmentID.IsZero() {
		return fmt.Errorf("%w: executor route has incomplete run or segment identity", errReducerInvariant)
	}
	if err := route.lineage.Validate(route.runID); err != nil {
		return fmt.Errorf("%w: executor route %q lineage: %w", errReducerInvariant, route.runID, err)
	}
	if err := validateRouteCommit(route, sessionID, batch.parkCommit); err != nil {
		return err
	}
	for _, reduced := range batch.events {
		if err := validateRouteCommit(route, sessionID, reduced.Commit); err != nil {
			return err
		}
		if err := validateRoutedEvent(route, sessionID, reduced.Event); err != nil {
			return err
		}
	}
	return nil
}

func validateRouteCommit(route *executorRoute, sessionID identity.SessionID, commit *EventCommit) error {
	if commit == nil {
		return nil
	}
	if commit.RunID != route.runID || commit.SessionID != sessionID || commit.SegmentID != route.segmentID {
		return fmt.Errorf(
			"%w: route %q Segment %q commit targets Run %q Segment %q in session %q",
			errReducerInvariant,
			route.runID,
			route.segmentID,
			commit.RunID,
			commit.SegmentID,
			commit.SessionID,
		)
	}
	for _, write := range commit.Items {
		if err := validateRouteItem(route, sessionID, write.Item()); err != nil {
			return err
		}
	}
	if commit.Run != nil {
		if err := validateRouteRun(route, sessionID, *commit.Run); err != nil {
			return err
		}
	}
	return nil
}

func validateRoutedEvent(route *executorRoute, sessionID identity.SessionID, routed ProjectionEvent) error {
	if routed == nil {
		return fmt.Errorf("%w: route %q carries a nil event", errReducerInvariant, route.runID)
	}
	if err := routed.validate(); err != nil {
		return fmt.Errorf("%w: route %q event %T: %w", errReducerInvariant, route.runID, routed, err)
	}
	switch event := routed.(type) {
	case SegmentStarted:
		return validateRouteRun(route, sessionID, event.Run)
	case SegmentFinished:
		return validateRouteRun(route, sessionID, event.Run)
	case ItemStarted:
		return validateRouteItemStart(route, sessionID, event.Item)
	case ItemCompleted:
		return validateRouteItem(route, sessionID, event.Item)
	case QuestionAnswered:
		return validateRouteItem(route, sessionID, event.Item)
	case PlanSnapshot:
		if event.SessionID != sessionID {
			return fmt.Errorf(
				"%w: route %q carries state for session %q, want %q",
				errReducerInvariant,
				route.runID,
				event.SessionID,
				sessionID,
			)
		}
	}
	return nil
}

func validateRouteItemStart(route *executorRoute, sessionID identity.SessionID, item ItemStart) error {
	started := item.Identity()
	if started.RunID != route.runID || started.SessionID != sessionID {
		return fmt.Errorf(
			"%w: route %q carries Item start %q for run %q in session %q",
			errReducerInvariant,
			route.runID,
			started.ItemID,
			started.RunID,
			started.SessionID,
		)
	}
	return nil
}

func validateRouteRun(route *executorRoute, sessionID identity.SessionID, value rundomain.Run) error {
	if value.ID() != route.runID ||
		value.SessionID() != sessionID ||
		value.Lineage() != route.lineage {
		return fmt.Errorf(
			"%w: route %q carries run %q in session %q with lineage %+v; want session %q and lineage %+v",
			errReducerInvariant,
			route.runID,
			value.ID(),
			value.SessionID(),
			value.Lineage(),
			sessionID,
			route.lineage,
		)
	}
	if value.State() == rundomain.Running && value.ActiveSegmentID() != route.segmentID {
		return fmt.Errorf(
			"%w: route %q running segment is %q, want %q",
			errReducerInvariant,
			route.runID,
			value.ActiveSegmentID(),
			route.segmentID,
		)
	}
	return nil
}

func validateRouteItem(route *executorRoute, sessionID identity.SessionID, item transcript.Item) error {
	if item.RunID() != route.runID || item.SessionID() != sessionID {
		return fmt.Errorf(
			"%w: route %q carries item %q for run %q in session %q",
			errReducerInvariant,
			route.runID,
			item.ID(),
			item.RunID(),
			item.SessionID(),
		)
	}
	return nil
}

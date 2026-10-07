package run_test

import (
	"context"
	"errors"
	"testing"
	"time"

	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestColdRecoveryDoesNotInstallAChildCompletionAheadOfItsTail(t *testing.T) {
	for _, attachSession := range []bool{false, true} {
		name := "segment"
		if attachSession {
			name = "session"
		}
		t.Run(name, func(t *testing.T) {
			source := newColdTreeSource(t)
			var recovered runworkflow.Recovery
			var err error
			if attachSession {
				recovered, err = runworkflow.AttachSession(t.Context(), source, "ses_tree")
			} else {
				recovered, err = runworkflow.RecoverSegment(t.Context(), source, "ses_tree", "run_root")
			}
			if err != nil {
				t.Fatal(err)
			}
			projection := conversation.New()
			if err := projection.RestoreAttachedSnapshot(recovered.Snapshot, recovered.Stream); err != nil {
				t.Fatal(err)
			}
			if projection.Checkpoint() != "evt_opaque_head" {
				t.Fatalf("checkpoint before any tail event = %q", projection.Checkpoint())
			}
			for event, err := range recovered.Stream.Events {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := projection.ApplyRunEvent(event); err != nil {
					t.Fatalf("apply successor %s: %v", event.EventID, err)
				}
			}
			if projection.Phase() != conversation.Idle || projection.Checkpoint() != "evt_root_finished" {
				t.Fatalf("root did not finish after both children: phase=%s checkpoint=%s", projection.Phase(), projection.Checkpoint())
			}
			if source.reads != 1 || !source.request.Snapshot || source.request.AfterEventID != "" {
				t.Fatalf("recovery reads=%d subscription=%+v", source.reads, source.request)
			}
		})
	}
}

func TestRecoveryRetainsTheOpaqueSnapshotHeadAcrossAnImmediateDisconnect(t *testing.T) {
	source := newColdTreeSource(t)
	source.disconnect = true
	recovered, err := runworkflow.RecoverSegment(t.Context(), source, "ses_tree", "run_root")
	if err != nil {
		t.Fatal(err)
	}
	projection := conversation.New()
	if err := projection.RestoreAttachedSnapshot(recovered.Snapshot, recovered.Stream); err != nil {
		t.Fatal(err)
	}
	for _, err := range recovered.Stream.Events {
		if !errors.Is(err, conversation.ErrDisconnected) {
			t.Fatalf("immediate disconnect = %v", err)
		}
	}
	_, err = source.SubscribeRun(t.Context(), conversation.SubscribeRun{
		RunID: projection.RunID(), SegmentID: recovered.Stream.SegmentID,
		AfterEventID: projection.Checkpoint(),
	})
	if err != nil || source.request.AfterEventID != "evt_opaque_head" || source.request.Snapshot {
		t.Fatalf("reconnect = %+v, %v", source.request, err)
	}
}

type coldTreeSource struct {
	snapshot   conversation.SessionSnapshot
	tail       []conversation.RunEvent
	reads      int
	request    conversation.SubscribeRun
	disconnect bool
}

func newColdTreeSource(t *testing.T) *coldTreeSource {
	t.Helper()
	root := conversation.Run{
		ID: "run_root", SessionID: "ses_tree", Lineage: conversation.RootRunLineage(),
		Status: protocol.RunStatusRunning, ActiveSegmentID: "seg_root",
	}
	source := &coldTreeSource{snapshot: conversation.SessionSnapshot{
		Session: conversation.Session{ID: root.SessionID, Status: protocol.SessionStatusRunning},
		Runs:    []conversation.Run{root},
	}}
	for _, id := range []string{"a", "b"} {
		lineage, err := conversation.NewChildRunLineage("run_"+id, "item_delegate_"+id, root.ID, root.ID)
		if err != nil {
			t.Fatal(err)
		}
		child := conversation.Run{
			ID: "run_" + id, SessionID: root.SessionID, Lineage: lineage,
			Status: protocol.RunStatusRunning, ActiveSegmentID: "seg_" + id,
		}
		source.snapshot.Runs = append(source.snapshot.Runs, child)
		source.tail = append(source.tail, conversation.RunEvent{
			EventID: "evt_finished_" + id, RunID: child.ID, SegmentID: child.ActiveSegmentID,
			StreamSegmentID: root.ActiveSegmentID, At: time.Unix(1, 0),
			Event: conversation.SegmentFinished{Run: finishedRun(child)},
		})
	}
	source.tail = append(source.tail, conversation.RunEvent{
		EventID: "evt_root_finished", RunID: root.ID, SegmentID: root.ActiveSegmentID,
		At: time.Unix(2, 0), Event: conversation.SegmentFinished{Run: finishedRun(root)},
	})
	return source
}

func (s *coldTreeSource) GetSession(context.Context, string) (conversation.SessionSnapshot, error) {
	s.reads++
	snapshot := s.snapshot
	if s.reads > 1 {
		// The child commits after subscription and before an independent read.
		// Installing this material would reject its already-buffered completion.
		snapshot.Runs = append([]conversation.Run(nil), snapshot.Runs...)
		snapshot.Runs[1].Status = protocol.RunStatusFinished
		snapshot.Runs[1].ActiveSegmentID = ""
		snapshot.Runs[1].Outcome.Status = protocol.OutcomeCompleted
	}
	return snapshot, nil
}

func (s *coldTreeSource) SubscribeRun(_ context.Context, request conversation.SubscribeRun) (conversation.SegmentStream, error) {
	s.request = request
	stream := conversation.SegmentStream{RunID: "run_root", SegmentID: "seg_root", HeadEventID: "evt_opaque_head"}
	if request.Snapshot {
		snapshot := s.snapshot
		stream.Snapshot = &snapshot
	}
	stream.Events = func(yield func(conversation.RunEvent, error) bool) {
		if s.disconnect {
			yield(conversation.RunEvent{}, conversation.ErrDisconnected)
			return
		}
		for _, event := range s.tail {
			if !yield(event, nil) {
				return
			}
		}
	}
	return stream, nil
}

func finishedRun(run conversation.Run) conversation.Run {
	run.Status, run.ActiveSegmentID = protocol.RunStatusFinished, ""
	run.Outcome = conversation.Outcome{Status: protocol.OutcomeCompleted}
	return run
}

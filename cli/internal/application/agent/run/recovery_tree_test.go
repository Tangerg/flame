package run_test

import (
	"context"
	"errors"
	"testing"
	"time"

	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
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
			conversation := agent.NewConversation()
			if err := conversation.RestoreAttachedSnapshot(recovered.Snapshot, recovered.Stream); err != nil {
				t.Fatal(err)
			}
			if conversation.Checkpoint() != "evt_opaque_head" {
				t.Fatalf("checkpoint before any tail event = %q", conversation.Checkpoint())
			}
			for event, err := range recovered.Stream.Events {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := conversation.ApplyRunEvent(event); err != nil {
					t.Fatalf("apply successor %s: %v", event.EventID, err)
				}
			}
			if conversation.Phase() != agent.ConversationIdle || conversation.Checkpoint() != "evt_root_finished" {
				t.Fatalf("root did not finish after both children: phase=%s checkpoint=%s", conversation.Phase(), conversation.Checkpoint())
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
	conversation := agent.NewConversation()
	if err := conversation.RestoreAttachedSnapshot(recovered.Snapshot, recovered.Stream); err != nil {
		t.Fatal(err)
	}
	for _, err := range recovered.Stream.Events {
		if !errors.Is(err, agent.ErrDisconnected) {
			t.Fatalf("immediate disconnect = %v", err)
		}
	}
	_, err = source.SubscribeRun(t.Context(), agent.SubscribeRun{
		RunID: conversation.RunID(), SegmentID: recovered.Stream.SegmentID,
		AfterEventID: conversation.Checkpoint(),
	})
	if err != nil || source.request.AfterEventID != "evt_opaque_head" || source.request.Snapshot {
		t.Fatalf("reconnect = %+v, %v", source.request, err)
	}
}

type coldTreeSource struct {
	snapshot   agent.SessionSnapshot
	tail       []agent.RunEvent
	reads      int
	request    agent.SubscribeRun
	disconnect bool
}

func newColdTreeSource(t *testing.T) *coldTreeSource {
	t.Helper()
	root := agent.Run{
		ID: "run_root", SessionID: "ses_tree", Lineage: agent.RootRunLineage(),
		Status: protocol.RunStatusRunning, ActiveSegmentID: "seg_root",
	}
	source := &coldTreeSource{snapshot: agent.SessionSnapshot{
		Session: agent.Session{ID: root.SessionID, Status: protocol.SessionStatusRunning},
		Runs:    []agent.Run{root},
	}}
	for _, id := range []string{"a", "b"} {
		lineage, err := agent.NewChildRunLineage("run_"+id, "item_delegate_"+id, root.ID, root.ID)
		if err != nil {
			t.Fatal(err)
		}
		child := agent.Run{
			ID: "run_" + id, SessionID: root.SessionID, Lineage: lineage,
			Status: protocol.RunStatusRunning, ActiveSegmentID: "seg_" + id,
		}
		source.snapshot.Runs = append(source.snapshot.Runs, child)
		source.tail = append(source.tail, agent.RunEvent{
			EventID: "evt_finished_" + id, RunID: child.ID, SegmentID: child.ActiveSegmentID,
			StreamSegmentID: root.ActiveSegmentID, At: time.Unix(1, 0),
			Event: agent.RunFinished{Outcome: agent.Outcome{Status: protocol.OutcomeCompleted}},
		})
	}
	source.tail = append(source.tail, agent.RunEvent{
		EventID: "evt_root_finished", RunID: root.ID, SegmentID: root.ActiveSegmentID,
		At: time.Unix(2, 0), Event: agent.RunFinished{Outcome: agent.Outcome{Status: protocol.OutcomeCompleted}},
	})
	return source
}

func (s *coldTreeSource) GetSession(context.Context, string) (agent.SessionSnapshot, error) {
	s.reads++
	snapshot := s.snapshot
	if s.reads > 1 {
		// The child commits after subscription and before an independent read.
		// Installing this material would reject its already-buffered completion.
		snapshot.Runs = append([]agent.Run(nil), snapshot.Runs...)
		snapshot.Runs[1].Status = protocol.RunStatusFinished
		snapshot.Runs[1].ActiveSegmentID = ""
		snapshot.Runs[1].Outcome.Status = protocol.OutcomeCompleted
	}
	return snapshot, nil
}

func (s *coldTreeSource) SubscribeRun(_ context.Context, request agent.SubscribeRun) (agent.SegmentStream, error) {
	s.request = request
	stream := agent.SegmentStream{RunID: "run_root", SegmentID: "seg_root", HeadEventID: "evt_opaque_head"}
	if request.Snapshot {
		snapshot := s.snapshot
		stream.Snapshot = &snapshot
	}
	stream.Events = func(yield func(agent.RunEvent, error) bool) {
		if s.disconnect {
			yield(agent.RunEvent{}, agent.ErrDisconnected)
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

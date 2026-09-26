package runtimebinding

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestSnapshotSubscriptionProjectsOnlyTheAcknowledgedMaterial(t *testing.T) {
	metadata := snapshotSession(1)
	metadata.Status = protocol.SessionStatusRunning
	reads := &snapshotBindingStub{sessions: []*protocol.Session{metadata}}
	root := protocol.RunRef{
		RunSummary:      protocol.RunSummary{ID: "run_root", SessionID: metadata.ID, Status: protocol.RunStatusRunning},
		ActiveSegmentID: "seg_root",
	}
	material := &protocol.SessionSnapshot{Runs: []protocol.RunRef{root}}
	head := "evt_opaque_head"
	var streamContext context.Context
	runtime := &Connection{snapshot: reads, profile: snapshotProfile(t), meta: requestMeta("test")}
	runtime.runs = runBindingStub{subscribe: func(ctx context.Context, request protocol.SubscribeRunRequest, options flameruntime.RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
		if !request.Snapshot || request.RunID != root.ID || request.SegmentID != root.ActiveSegmentID || options.AfterEventID != "" {
			t.Fatalf("snapshot subscription = %+v, options = %+v", request, options)
		}
		streamContext = ctx
		return &protocol.SubscribeRunResponse{
			RunID: root.ID, SegmentID: root.ActiveSegmentID, Snapshot: material, HeadEventID: &head,
		}, func(func(protocol.RunEvent, error) bool) {}, nil
	}}
	stream, err := runtime.SubscribeRun(t.Context(), agent.SubscribeRun{
		SessionID: metadata.ID, RunID: root.ID, SegmentID: root.ActiveSegmentID, Snapshot: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stream.Snapshot == nil || len(stream.Snapshot.Runs) != 1 || stream.Snapshot.Runs[0].ID != root.ID || stream.HeadEventID != head {
		t.Fatalf("projected snapshot subscription = %+v", stream)
	}
	if reads.sessionCalls != 2 || len(reads.snapshotRequests) != 0 {
		t.Fatalf("independent reads: metadata=%d material=%d", reads.sessionCalls, len(reads.snapshotRequests))
	}
	if streamContext.Err() != nil {
		t.Fatalf("live subscription context was released: %v", streamContext.Err())
	}
	for range stream.Events {
	}
	if !errors.Is(streamContext.Err(), context.Canceled) {
		t.Fatal("finished snapshot subscription retained its context")
	}
}

func TestSnapshotSubscriptionReleasesTheTailWhenSessionMetadataChanges(t *testing.T) {
	first := snapshotSession(1)
	first.Status = protocol.SessionStatusRunning
	second := *first
	second.Revision = 2
	reads := &snapshotBindingStub{sessions: []*protocol.Session{first, &second, &second}}
	root := protocol.RunRef{
		RunSummary:      protocol.RunSummary{ID: "run_root", SessionID: first.ID, Status: protocol.RunStatusRunning},
		ActiveSegmentID: "seg_root",
	}
	var contexts []context.Context
	runtime := &Connection{snapshot: reads, profile: snapshotProfile(t), meta: requestMeta("test")}
	runtime.runs = runBindingStub{subscribe: func(ctx context.Context, _ protocol.SubscribeRunRequest, _ flameruntime.RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
		if len(contexts) != 0 && contexts[0].Err() == nil {
			t.Fatal("superseded snapshot tail is still attached")
		}
		contexts = append(contexts, ctx)
		return &protocol.SubscribeRunResponse{
			RunID: root.ID, SegmentID: root.ActiveSegmentID,
			Snapshot: &protocol.SessionSnapshot{Runs: []protocol.RunRef{root}},
		}, func(func(protocol.RunEvent, error) bool) {}, nil
	}}
	stream, err := runtime.SubscribeRun(t.Context(), agent.SubscribeRun{
		SessionID: first.ID, RunID: root.ID, SegmentID: root.ActiveSegmentID, Snapshot: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(contexts) != 2 || stream.Snapshot == nil || stream.Snapshot.Session.Revision != second.Revision || len(reads.snapshotRequests) != 0 {
		t.Fatalf("stable subscription: attempts=%d snapshot=%+v", len(contexts), stream.Snapshot)
	}
	for range stream.Events {
	}
}

func TestSnapshotSubscriptionRejectsMissingOrMisdirectedMaterial(t *testing.T) {
	for _, name := range []string{"missing", "another session"} {
		t.Run(name, func(t *testing.T) {
			metadata := snapshotSession(1)
			metadata.Status = protocol.SessionStatusRunning
			runtime := &Connection{
				snapshot: &snapshotBindingStub{sessions: []*protocol.Session{metadata}},
				profile:  snapshotProfile(t), meta: requestMeta("test"),
			}
			var attached context.Context
			runtime.runs = runBindingStub{subscribe: func(ctx context.Context, _ protocol.SubscribeRunRequest, _ flameruntime.RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
				attached = ctx
				ack := &protocol.SubscribeRunResponse{RunID: "run_root", SegmentID: "seg_root"}
				if name == "another session" {
					ack.Snapshot = &protocol.SessionSnapshot{Runs: []protocol.RunRef{{RunSummary: protocol.RunSummary{ID: "run_root", SessionID: "ses_other"}}}}
				}
				return ack, func(func(protocol.RunEvent, error) bool) {}, nil
			}}
			_, err := runtime.SubscribeRun(t.Context(), agent.SubscribeRun{SessionID: metadata.ID, RunID: "run_root", SegmentID: "seg_root", Snapshot: true})
			if !errors.Is(err, agent.ErrIncompatibleRuntime) || attached.Err() == nil {
				t.Fatalf("mismatched snapshot: error=%v context=%v", err, attached.Err())
			}
		})
	}
}

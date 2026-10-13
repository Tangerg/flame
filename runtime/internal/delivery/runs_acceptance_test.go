package delivery

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestCapturedOpeningCollisionPreservesKnownInvalidTarget(t *testing.T) {
	err := wireRunStartErr(session.ErrInvalid)
	if !errors.Is(err, protocol.ErrInvalidParams) || !errors.Is(err, session.ErrInvalid) {
		t.Fatalf("known opening target refusal lost its category or cause: %v", err)
	}
}

type acceptanceQueryRuns struct {
	runUseCases
	record runs.CommandAcceptanceRecord
	key    string
}

func (r *acceptanceQueryRuns) LookupCommandAcceptance(_ context.Context, key string) (runs.CommandAcceptanceRecord, error) {
	r.key = key
	return r.record, nil
}

func TestAcceptanceVerifiesOriginalRequestAndActorAfterMaterialRetires(t *testing.T) {
	request := protocol.StartRunRequest{NewSession: &protocol.RunSessionOpening{SessionID: "ses_captured", RunID: "run_captured", Title: "captured", Workspace: protocol.WorkspaceRef{Path: "/work"}}, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "original"}}}
	fingerprint, err := operationFingerprint(RunsStart, request)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := operation.NewActor("11111111-1111-4111-8111-111111111111", testsupport.Digest("release").String(), testsupport.Digest("authority").String(), "backend")
	if err != nil {
		t.Fatal(err)
	}
	ctx := operation.WithActor(t.Context(), actor)
	owner := &acceptanceQueryRuns{record: runs.CommandAcceptanceRecord{Found: true, Acceptance: runs.CommandAcceptance{Key: operation.ScopedKey(ctx, "original"), Fingerprint: fingerprint, Operation: runs.CommandStartOpening, SessionID: "ses_captured", RunID: "run_captured", SegmentID: "seg_captured", UserItemID: "item_captured"}}}
	handler := &Handler{runs: owner}
	for _, live := range []bool{true, false} {
		owner.record.MaterialAvailable = live
		accepted, err := handler.GetRunAcceptance(ctx, protocol.GetRunAcceptanceRequest{Key: "original", Request: &request})
		want := protocol.RunAcceptanceRetired
		if live {
			want = protocol.RunAcceptanceOpened
		}
		if err != nil || accepted.Type != want || owner.key != owner.record.Acceptance.Key {
			t.Fatalf("original acceptance=%+v key=%s error=%v", accepted, owner.key, err)
		}
		changed := request
		changed.Input = []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "changed"}}
		conflict, err := handler.GetRunAcceptance(ctx, protocol.GetRunAcceptanceRequest{Key: "original", Request: &changed})
		if err != nil || conflict.Type != protocol.RunAcceptanceConflict {
			t.Fatalf("changed request escaped conflict: %+v %v", conflict, err)
		}
	}
}

// TestPresentRunAcceptanceProjectsEveryRecordShape pins the closed mapping from a
// command-acceptance lookup to its wire union, and that every branch it produces
// is a frame the published wire shape accepts.
func TestPresentRunAcceptanceProjectsEveryRecordShape(t *testing.T) {
	t.Parallel()

	accepted := runs.CommandAcceptance{
		Key:         "start-once",
		Fingerprint: "0000000000000000000000000000000000000000000000000000000000000000",
		Operation:   runs.CommandStartOpening,
		SessionID:   "ses_1", RunID: "run_1", SegmentID: "seg_1", UserItemID: "item_1",
	}
	tests := []struct {
		name   string
		record runs.CommandAcceptanceRecord
		want   protocol.RunAcceptance
	}{
		{
			name:   "not recorded",
			record: runs.CommandAcceptanceRecord{},
			want:   protocol.RunAcceptance{Type: protocol.RunAcceptanceNotRecorded},
		},
		{
			name:   "bound to another operation",
			record: runs.CommandAcceptanceRecord{OtherOperation: true},
			want:   protocol.RunAcceptance{Type: protocol.RunAcceptanceConflict},
		},
		{
			name:   "opened and attachable",
			record: runs.CommandAcceptanceRecord{Found: true, MaterialAvailable: true, Acceptance: accepted},
			want: protocol.RunAcceptance{
				Type: protocol.RunAcceptanceOpened, RunID: "run_1", SegmentID: "seg_1",
				SessionID: "ses_1", UserItemID: "item_1",
			},
		},
		{
			name:   "accepted but retired",
			record: runs.CommandAcceptanceRecord{Found: true, Acceptance: accepted},
			want: protocol.RunAcceptance{
				Type: protocol.RunAcceptanceRetired, RunID: "run_1",
				SessionID: "ses_1", UserItemID: "item_1",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := presentRunAcceptance(test.record)
			if *got != test.want {
				t.Fatalf("presentRunAcceptance = %+v, want %+v", *got, test.want)
			}
			if err := protocol.ValidateWireTree(*got); err != nil {
				t.Fatalf("projected %s acceptance fails wire validation: %v", test.name, err)
			}
		})
	}
}

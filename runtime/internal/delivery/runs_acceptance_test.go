package delivery

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/protocol"
)

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

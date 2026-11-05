package sessions

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestExportSessionProjectsToolResultsFromOneSnapshot(t *testing.T) {
	for _, test := range []struct {
		name string
		view ToolResultExport
		want string
	}{
		{"referenced", ReferencedToolResults, "bounded preview"},
		{"expanded", ExpandedToolResults, "full body"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := offloadedSnapshot("bounded preview")
			root := portableSnapshot().Runs[0].Snapshot()
			root.ID = source.Items[0].RunID()
			source.Runs = []run.Run{testsupport.MustRestoreRun(root)}
			reads := 0
			stores := coordinatorStores{
				snapshot: source, snapshotReads: &reads,
				interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{}},
			}
			coordinator := newCoordinator(stores, inertExecutionReleaser{})
			exported, err := coordinator.ExportSession(t.Context(), source.Session.ID(), test.view)
			if err != nil {
				t.Fatal(err)
			}
			if reads != 1 {
				t.Fatalf("snapshot reads = %d, want one coherent capture", reads)
			}
			call, _ := testsupport.ToolInvocationOf(exported.Items[0])
			if text, _ := call.Result.String(); text != test.want {
				t.Fatalf("exported result = %q, want %q", text, test.want)
			}
			if call.Offload != nil || len(exported.ToolResults) != 1 {
				t.Fatal("export lost the sole portable tool-result binding")
			}
			if _, err := exported.CanonicalSnapshot(); err != nil {
				t.Fatalf("exported snapshot is not restorable: %v", err)
			}
			original, _ := testsupport.ToolInvocationOf(source.Items[0])
			if text, _ := original.Result.String(); text != "bounded preview" {
				t.Fatal("export changed the captured source result")
			}
		})
	}
}

func TestExportSessionRejectsUnknownToolResultRepresentationBeforeAdmission(t *testing.T) {
	_, err := new(Coordinator).ExportSession(t.Context(), "session", ToolResultExport(255))
	if err == nil || !strings.Contains(err.Error(), "unsupported tool-result export") {
		t.Fatalf("unknown representation = %v", err)
	}
}

func TestExportSessionRejectsBrokenCapturedState(t *testing.T) {
	for _, view := range []ToolResultExport{ReferencedToolResults, ExpandedToolResults} {
		source := offloadedSnapshot("bounded preview")
		coordinator := newCoordinator(coordinatorStores{
			snapshot:   source,
			interrupts: &coordinatorInterrupts{pending: map[string]runs.Pending{}},
		}, inertExecutionReleaser{})
		_, err := coordinator.ExportSession(t.Context(), source.Session.ID(), view)
		if err == nil || !strings.Contains(err.Error(), "references unknown run") {
			t.Fatalf("export with broken run reference = %v", err)
		}
	}
}

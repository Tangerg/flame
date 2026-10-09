package delivery

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestSessionImportPreservesExactTimeBoundsAndEpoch(t *testing.T) {
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).UTC(), time.Unix(0, 0).UTC(), time.Unix(0, math.MaxInt64).UTC()} {
		t.Run(at.Format(time.RFC3339Nano), func(t *testing.T) {
			handler, _ := rollbackHarness(t)
			artifact := validArtifact()
			artifact.Session.Workspace.Path = t.TempDir()
			artifact.Session.CreatedAt, artifact.Session.UpdatedAt = at, at
			artifact.Runs[0].ProtocolProfile = &protocol.RunProtocolProfile{}
			artifact.Runs[0].CreatedAt, artifact.Runs[0].FinishedAt, artifact.Runs[0].UpdatedAt = at, at, at
			artifact.Items[0] = archivedToolAt(at)
			artifact.Items[0].Tool.Result = "preview"
			artifact.ToolResults = []protocol.ArtifactToolResult{{ID: "ABC234", ItemID: "item_1", Body: "full result", CreatedAt: at}}
			if _, err := handler.ImportSession(t.Context(), protocol.ImportSessionRequest{Artifact: artifact}); err != nil {
				t.Fatal(err)
			}
			exported, err := handler.ExportSession(t.Context(), protocol.ExportSessionRequest{SessionID: artifact.Session.ID})
			if err != nil {
				t.Fatal(err)
			}
			artifact.Session.Workspace.Path = canonicalWorkspacePath(t, artifact.Session.Workspace.Path)
			if before, after := encodeArtifact(t, artifact), encodeArtifact(t, *exported.Artifact); before != after {
				t.Fatalf("archive changed at exact time boundary\nbefore: %s\nafter: %s", before, after)
			}
		})
	}
}

func TestSessionImportRefusesTimesThatCannotSurvivePersistence(t *testing.T) {
	for _, at := range []time.Time{
		time.Date(1000, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC),
	} {
		for _, test := range []struct {
			name   string
			change func(*protocol.SessionArtifact)
		}{
			{"Session", func(a *protocol.SessionArtifact) { a.Session.CreatedAt, a.Session.UpdatedAt = at, at }},
			{"Run", func(a *protocol.SessionArtifact) {
				a.Runs[0].CreatedAt, a.Runs[0].FinishedAt, a.Runs[0].UpdatedAt = at, at, at
			}},
			{"Item", func(a *protocol.SessionArtifact) { a.Items[0].CreatedAt = at }},
			{"Tool lifecycle", func(a *protocol.SessionArtifact) {
				a.Items[0] = archivedToolAt(at)
			}},
			{"Tool result", func(a *protocol.SessionArtifact) {
				a.Items[0] = archivedToolAt(time.Unix(1, 0).UTC())
				a.Items[0].Tool.Result = "preview"
				a.ToolResults = []protocol.ArtifactToolResult{{ID: "ABC234", ItemID: "item_1", Body: "full result", CreatedAt: at}}
			}},
		} {
			t.Run(at.Format("2006")+"/"+test.name, func(t *testing.T) {
				handler, rt := rollbackHarness(t)
				artifact := validArtifact()
				artifact.Session.Workspace.Path = t.TempDir()
				artifact.Runs[0].ProtocolProfile = &protocol.RunProtocolProfile{}
				test.change(&artifact)
				if _, err := handler.ImportSession(t.Context(), protocol.ImportSessionRequest{Artifact: artifact}); !errors.Is(err, protocol.ErrInvalidParams) {
					t.Fatalf("import error = %v, want invalid_params before mutation", err)
				}
				if _, err := rt.sess.Get(t.Context(), artifact.Session.ID); !errors.Is(err, session.ErrNotFound) {
					t.Fatalf("refused import retained a Session: %v", err)
				}
			})
		}
	}
}

func TestSessionImportPreservesToolResultCreationNanoseconds(t *testing.T) {
	handler, _ := rollbackHarness(t)
	at := time.Unix(1, 123456789).UTC()
	artifact := validArtifact()
	artifact.Session.Workspace.Path = t.TempDir()
	artifact.Runs[0].ProtocolProfile = &protocol.RunProtocolProfile{}
	artifact.Items[0] = archivedToolAt(time.Unix(1, 0).UTC())
	artifact.Items[0].Tool.Result = "preview"
	artifact.ToolResults = []protocol.ArtifactToolResult{{ID: "ABC234", ItemID: "item_1", Body: "full result", CreatedAt: at}}
	if _, err := handler.ImportSession(t.Context(), protocol.ImportSessionRequest{Artifact: artifact}); err != nil {
		t.Fatal(err)
	}
	exported, err := handler.ExportSession(t.Context(), protocol.ExportSessionRequest{SessionID: artifact.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.Artifact.ToolResults) != 1 || !exported.Artifact.ToolResults[0].CreatedAt.Equal(at) {
		t.Fatalf("re-exported Tool results = %+v, want exact creation time %s", exported.Artifact.ToolResults, at)
	}
}

func archivedToolAt(at time.Time) protocol.ArtifactItem {
	return protocol.ArtifactItem{
		ID: "item_1", RunID: "run_1", Type: protocol.ItemTypeToolCall, Status: protocol.ItemStatusCompleted,
		StartedAt: at, FinishedAt: at, DurationMillis: new(int64),
		Tool: &protocol.ToolInvocation{Name: "shell", Arguments: map[string]any{}},
	}
}

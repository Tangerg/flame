package persistence

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestInstalledCallerCommitsWithDispatchAndOutlivesRunDeletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "callers.sqlite")
	db, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	draft := testsupport.RunDraft(run.Draft{RunID: "run_caller", SessionID: "ses_caller", SegmentID: "seg_caller", CreatedAt: time.Unix(100, 0)})
	if err := newRunStore(db).Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	id := testsupport.InstallationID(t)
	source := plugin.ToolCallerSnapshot{Invocation: portable.Invocation{ID: "logical-tool", Parent: "process:original", Origin: extension.Model, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: id.String()}}, Tool: testsupport.MCPTool(testsupport.InstallationMCPServer(id, "schedules"), "run_schedule"), Release: testsupport.Digest("release"), Authority: testsupport.Digest("authority")}
	caller, err := plugin.NewToolCaller(source)
	if err != nil {
		t.Fatal(err)
	}
	store := newToolInvocationStore(db)
	if err := store.StartToolInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, source.Invocation.ID, "item_caller", draft.CreatedAt, &caller); err != nil {
		t.Fatal(err)
	}
	changed := source
	changed.Authority = testsupport.Digest("replacement-authority")
	replacement, err := plugin.NewToolCaller(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartToolInvocation(t.Context(), draft.SessionID, draft.RunID, "seg_replacement", source.Invocation.ID, "item_replacement", draft.CreatedAt, &replacement); !errors.Is(err, plugin.ErrCommandConflict) {
		t.Fatalf("original caller was overwritten: %v", err)
	}
	failed := source
	failed.Invocation.ID = "failed-tool"
	unadmitted, err := plugin.NewToolCaller(failed)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartToolInvocation(t.Context(), draft.SessionID, "run_missing", draft.SegmentID, failed.Invocation.ID, "item_failed", draft.CreatedAt, &unadmitted); err == nil {
		t.Fatal("missing Run admitted a Tool caller")
	}
	if _, found, err := store.ToolCaller(t.Context(), failed.Invocation.ID); err != nil || found {
		t.Fatalf("failed dispatch retained usable authority: %t, %v", found, err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE installed_tool_callers SET caller_json='{}' WHERE call_id=?`, source.Invocation.ID); err == nil {
		t.Fatal("immutable caller was rewritten")
	}
	if _, err := db.ExecContext(t.Context(), `DELETE FROM runs WHERE run_id=?`, draft.RunID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	retained, found, err := newToolInvocationStore(db).ToolCaller(t.Context(), source.Invocation.ID)
	if err != nil || !found || retained.Snapshot() != source {
		t.Fatalf("deleted Run or restart changed original calling evidence: %t, %+v, %v", found, retained.Snapshot(), err)
	}
}

package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	models "github.com/Tangerg/flame/packages/go-sdk/model/catalog"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	scheduledb "github.com/Tangerg/flame/plugins/schedules/internal/sqlite"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func scheduleToolSession(t *testing.T, db *sql.DB, host *observedRunHost) (*sdk.ClientSession, func() error) {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "schedules-tool-test"}, nil)
	registered, err := registerServer(server, db, scheduledb.NewScheduleStore(db), filesystem.DirectoryResolver{}, models.Selections{}, func(context.Context, qimumcp.CallbackRef) (runHost, func() error, error) {
		return host, func() error { return nil }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "schedules-tool-consumer"}, nil)
	if err := qimumcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	left, right := sdk.NewInMemoryTransports()
	backend, err := server.Connect(t.Context(), left, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), right, nil)
	if err != nil {
		t.Fatal(err)
	}
	close := func() error { return errors.Join(session.Close(), backend.Close(), registered.Close()) }
	t.Cleanup(func() { _ = close() })
	return session, close
}

func scheduleToolMetadata(t *testing.T, id string) sdk.Meta {
	t.Helper()
	metadata, err := qimumcp.CallbackMetadata(qimumcp.CallbackRef{Endpoint: "http://127.0.0.1:1/", Credential: strings.Repeat("a", 64), Call: strings.Repeat("b", 64), MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := metadata.Set(qimumcp.InvocationMetadataKey, portable.Invocation{ID: id, Origin: extension.Model, Parent: "parent-call", Target: portable.Target{Scope: "installation", Reference: "schedules-owner"}}); err != nil {
		t.Fatal(err)
	}
	return metadata.Meta
}

func assertScheduleToolReceipt(t *testing.T, session *sdk.ClientSession, id, name string, input any, normal *sdk.CallToolResult) *chat.ToolOutput {
	t.Helper()
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := pluginsdk.ReadToolReceipt(t.Context(), session, id, name, admitted)
	if normal.IsError {
		if output == nil || err == nil || len(normal.Content) != 1 {
			t.Fatalf("refusal receipt %s = %+v, %v", name, output, err)
		}
		text, _ := output.Text()
		if text != normal.Content[0].(*sdk.TextContent).Text {
			t.Fatalf("refusal changed during recovery: %q, %+v", text, normal.Content)
		}
		return output
	}
	if err != nil || output == nil {
		t.Fatalf("receipt %s = %+v, %v", name, output, err)
	}
	expected, expectedErr := jsonvalue.Encode(normal.StructuredContent)
	observed, observedErr := jsonvalue.Parse(output.Details)
	if expectedErr != nil || observedErr != nil || expected != observed {
		t.Fatalf("receipt %s changed original output: %s, %s, %v, %v", name, expected, observed, expectedErr, observedErr)
	}
	return output
}

func TestModelScheduleEditsAndDeletionHaveHistoricalReceipts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	db, err := scheduledb.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	host := &observedRunHost{db: db, namespace: "original-runtime"}
	session, close := scheduleToolSession(t, db, host)
	type observed struct {
		id, name string
		input    any
		output   *sdk.CallToolResult
	}
	var records []observed
	call := func(id, name string, input any, refused bool) *sdk.CallToolResult {
		t.Helper()
		output, err := session.CallTool(t.Context(), &sdk.CallToolParams{Meta: scheduleToolMetadata(t, id), Name: name, Arguments: input})
		if err != nil || output.IsError != refused {
			t.Fatalf("%s: %+v, %v", name, output, err)
		}
		assertScheduleToolReceipt(t, session, id, name, input, output)
		records = append(records, observed{id, name, input, output})
		return output
	}
	created := call("create", createScheduleTool, map[string]any{"title": "original", "instructions": "inspect", "cron": "0 9 * * *"}, false)
	encoded, err := json.Marshal(created.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var first scheduleResponse
	if err := json.Unmarshal(encoded, &first); err != nil {
		t.Fatal(err)
	}
	updated := call("edit", updateScheduleTool, map[string]any{"schedule_id": first.Schedule.ScheduleID, "expected_revision": first.Schedule.Revision, "title": "edited", "enabled": false}, false)
	encoded, err = json.Marshal(updated.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var next scheduleResponse
	if err := json.Unmarshal(encoded, &next); err != nil {
		t.Fatal(err)
	}
	if next.Schedule.Revision != first.Schedule.Revision+1 || next.Schedule.Title != "edited" || next.Schedule.Enabled {
		t.Fatalf("edit failed to preserve the observed transition: %+v", next)
	}
	call("stale", updateScheduleTool, map[string]any{"schedule_id": first.Schedule.ScheduleID, "expected_revision": first.Schedule.Revision, "title": "stale"}, true)
	call("bad-model", updateScheduleTool, map[string]any{"schedule_id": first.Schedule.ScheduleID, "expected_revision": next.Schedule.Revision, "provider": "incomplete"}, true)
	call("delete", deleteScheduleTool, map[string]any{"schedule_id": first.Schedule.ScheduleID}, false)
	if err := close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = scheduledb.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	host.db = db
	session, _ = scheduleToolSession(t, db, host)
	for _, record := range records {
		assertScheduleToolReceipt(t, session, record.id, record.name, record.input, record.output)
	}
	var definitions int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM schedules`).Scan(&definitions); err != nil || definitions != 0 {
		t.Fatalf("receipt recovery recreated deleted work: %d, %v", definitions, err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE commands SET reply='{"value":null}' WHERE id='tool:delete'`); err != nil {
		t.Fatal(err)
	}
	admitted, err := jsonvalue.Encode(map[string]any{"schedule_id": first.Schedule.ScheduleID})
	if err != nil {
		t.Fatal(err)
	}
	if output, err := pluginsdk.ReadToolReceipt(t.Context(), session, "delete", deleteScheduleTool, admitted); output != nil || err == nil {
		t.Fatalf("null receipt became evidence of deletion: %+v, %v", output, err)
	}
}

func TestModelManualRunRetainsItsCommandAcrossRestartAndDeletedDefinition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	db, err := scheduledb.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := scheduledb.NewScheduleStore(db)
	definition, err := schedule.New("sch_model_manual", schedule.Draft{Instructions: "captured original", Cron: "@hourly"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(t.Context(), definition); err != nil {
		t.Fatal(err)
	}
	host := &observedRunHost{db: db, namespace: "original-runtime"}
	host.lost.Store(true)
	session, close := scheduleToolSession(t, db, host)
	input := map[string]any{"ID": definition.ID()}
	output, callErr := session.CallTool(t.Context(), &sdk.CallToolParams{Meta: scheduleToolMetadata(t, "manual"), Name: runScheduleTool, Arguments: input})
	if callErr == nil && output != nil && !output.IsError {
		t.Fatal("lost Runtime acknowledgement became success")
	}
	pending, err := store.PendingManual(t.Context(), "", 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending capture = %+v, %v", pending, err)
	}
	captured := pending[0]
	if captured.CommandID() != "tool:manual" || captured.Invocation().ID != "manual" || captured.Invocation().Origin != extension.Model {
		t.Fatalf("calling identity and receipt command were conflated: %+v", captured.Snapshot())
	}
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := pluginsdk.ReadToolReceipt(t.Context(), session, "manual", runScheduleTool, admitted); output != nil || err != nil {
		t.Fatalf("unknown admission fabricated a final receipt: %+v, %v", output, err)
	}
	if _, err := store.Delete(t.Context(), definition.ID()); err != nil {
		t.Fatal(err)
	}
	if err := close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = scheduledb.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	store, host.db = scheduledb.NewScheduleStore(db), db
	host.lost.Store(false)
	pending, err = store.PendingManual(t.Context(), "", 10)
	if err != nil || len(pending) != 1 || pending[0].Snapshot() != captured.Snapshot() {
		t.Fatalf("restart changed captured work: %+v, %v", pending, err)
	}
	ctx, err := qimumcp.WithInvocation(t.Context(), captured.Invocation())
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := resumeCaptured(ctx, db, store, host, pending[0])
	if err != nil || accepted.RunID != captured.RunID() || accepted.SessionID != captured.SessionID() {
		t.Fatalf("original manual recovery = %+v, %v", accepted, err)
	}
	calls := host.calls.Load()
	session, _ = scheduleToolSession(t, db, host)
	replayed, err := session.CallTool(t.Context(), &sdk.CallToolParams{Meta: scheduleToolMetadata(t, "manual"), Name: runScheduleTool, Arguments: input})
	if err != nil || replayed.IsError || host.calls.Load() != calls {
		t.Fatalf("replay repeated Runtime admission: %+v, %v", replayed, err)
	}
	assertScheduleToolReceipt(t, session, "manual", runScheduleTool, input, replayed)
	if host.calls.Load() != calls {
		t.Fatal("read-only receipt lookup started work")
	}
}

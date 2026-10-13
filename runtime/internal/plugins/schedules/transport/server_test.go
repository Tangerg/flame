package transport

import (
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/modelcatalog"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	scheduledb "github.com/Tangerg/flame/runtime/internal/plugins/schedules/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestOriginalCreateResultIsHistoricalAndReadOnly(t *testing.T) {
	db, err := scheduledb.Open(t.Context(), filepath.Join(t.TempDir(), "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := sdk.NewServer(&sdk.Implementation{Name: "schedules-proof"}, nil)
	if err := RegisterServer(server, db, scheduledb.NewScheduleStore(db), workspace.Resolver{}, modelcatalog.Selections{}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "runtime-proof"}, nil)
	if err := rpc.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	backend, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	input := map[string]any{"title": "original title", "instructions": "inspect progress", "cron": "0 0 1 1 *", "workspace_path": t.TempDir()}
	arguments, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(id, args string) (rpc.OriginalCreateOutput, error) {
		return managedmcp.Call[rpc.OriginalCreateInput, rpc.OriginalCreateOutput](t.Context(), session, rpc.OriginalCreateResult, rpc.OriginalCreateInput{CallID: id, Arguments: args})
	}
	if result, err := lookup("original", string(arguments)); err != nil || result.Output != nil {
		t.Fatalf("absence: %+v,%v", result, err)
	}
	normal, err := session.CallTool(t.Context(), &sdk.CallToolParams{Meta: sdk.Meta{managedmcp.InvocationKey: "original"}, Name: "create_schedule", Arguments: input})
	if err != nil || normal.IsError {
		t.Fatalf("normal create: %+v,%v", normal, err)
	}
	original, err := lookup("original", string(arguments))
	if err != nil || original.Output == nil {
		t.Fatalf("original proof: %+v,%v", original, err)
	}
	var details any
	if err := json.Unmarshal(original.Output.Details, &details); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(details, normal.StructuredContent) {
		t.Fatalf("receipt output differs from the normal Tool output: %+v / %+v", original.Output.Details, normal.StructuredContent)
	}
	page, err := managedmcp.Call[rpc.ListInput, rpc.Page](t.Context(), session, rpc.List, rpc.ListInput{})
	if err != nil || len(page.Rows) != 1 {
		t.Fatalf("created schedules: %+v,%v", page, err)
	}
	updated := "edited title"
	if _, err := managedmcp.CallCommand[rpc.UpdateInput, schedule.Snapshot](t.Context(), session, rpc.Update, "edit", rpc.UpdateInput{ID: page.Rows[0].ID, ExpectedRevision: page.Rows[0].Revision, Title: &updated}); err != nil {
		t.Fatal(err)
	}
	if _, err := managedmcp.CallCommand[rpc.ItemInput, rpc.Ack](t.Context(), session, rpc.Delete, "delete", rpc.ItemInput{ID: page.Rows[0].ID}); err != nil {
		t.Fatal(err)
	}
	historical, err := lookup("original", string(arguments))
	if err != nil || !reflect.DeepEqual(historical, original) {
		t.Fatalf("mutable Schedule changed its original receipt: %+v,%v", historical, err)
	}
	input["title"] = "different"
	changed, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := lookup("original", string(changed)); result.Output != nil || !errors.Is(rpc.DecodeError(err), idempotency.ErrKeyConflict) {
		t.Fatalf("wrong input became evidence: %+v,%v", result, err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE commands SET result='{"wrong":"result"}' WHERE id='tool:original'`); err != nil {
		t.Fatal(err)
	}
	if result, err := lookup("original", string(arguments)); result.Output != nil || err == nil {
		t.Fatalf("corrupt receipt became evidence: %+v,%v", result, err)
	}
	var schedules, creates int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM schedules`).Scan(&schedules); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM commands WHERE method=?`, rpc.Create).Scan(&creates); err != nil {
		t.Fatal(err)
	}
	if schedules != 0 || creates != 1 {
		t.Fatalf("queries repeated creation: schedules=%d creates=%d", schedules, creates)
	}
}

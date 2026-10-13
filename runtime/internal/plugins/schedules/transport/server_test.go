package transport

import (
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"context"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/modelcatalog"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	scheduledb "github.com/Tangerg/flame/runtime/internal/plugins/schedules/sqlite"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/tools"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestOriginalCreateResultIsHistoricalAndReadOnly(t *testing.T) {
	db, err := scheduledb.Open(t.Context(), filepath.Join(t.TempDir(), "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := sdk.NewServer(&sdk.Implementation{Name: "schedules-proof"}, nil)
	if err := RegisterServer(server, db, scheduledb.NewScheduleStore(db), workspace.Resolver{}, modelcatalog.Selections{}, func(context.Context, pluginsdk.CallbackRef) (RunHost, func() error, error) {
		return nil, nil, errors.New("unexpected Run connection")
	}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "runtime-proof"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
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
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	exported, err := pluginsdk.ResolveCapability(t.Context(), session, "schedules", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	create, err := exported.Method(rpc.Create)
	if err != nil {
		t.Fatal(err)
	}
	list, err := contracts.List.Bind(exported)
	if err != nil {
		t.Fatal(err)
	}
	update, err := contracts.Update.Bind(exported)
	if err != nil {
		t.Fatal(err)
	}
	remove, err := contracts.Delete.Bind(exported)
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(id, args string) (*chat.ToolOutput, error) {
		admitted, err := jsoninput.Parse([]byte(args))
		if err != nil {
			return nil, err
		}
		receipt, found, err := create.ReadReceipt(t.Context(), "tool:"+id, admitted)
		if err != nil || !found {
			return nil, rpc.DecodeError(err)
		}
		var snapshot schedule.Snapshot
		if err := json.Unmarshal(receipt.JSON(), &snapshot, json.RejectUnknownMembers(true)); err != nil {
			return nil, err
		}
		value, err := schedule.Restore(snapshot)
		if err != nil {
			return nil, err
		}
		output, err := tools.CreateOutput(value)
		return &output, err
	}
	if result, err := lookup("original", string(arguments)); err != nil || result != nil {
		t.Fatalf("absence: %+v,%v", result, err)
	}
	normal, err := session.CallTool(t.Context(), &sdk.CallToolParams{Meta: sdk.Meta{pluginsdk.InvocationMetadataKey: capability.Invocation{ID: "original", Origin: capability.Model, Target: capability.Target{Scope: "installation", Reference: "schedules-owner"}}}, Name: "create_schedule", Arguments: input})
	if err != nil || normal.IsError {
		t.Fatalf("normal create: %+v,%v", normal, err)
	}
	original, err := lookup("original", string(arguments))
	if err != nil || original == nil {
		t.Fatalf("original proof: %+v,%v", original, err)
	}
	var details any
	if err := json.Unmarshal(original.Details, &details); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(details, normal.StructuredContent) {
		t.Fatalf("receipt output differs from the normal Tool output: %+v / %+v", original.Details, normal.StructuredContent)
	}
	page, err := list.Call(t.Context(), rpc.ListInput{})
	if err != nil || len(page.Rows) != 1 {
		t.Fatalf("created schedules: %+v,%v", page, err)
	}
	updated := "edited title"
	if _, err := update.CallCommand(t.Context(), "edit", rpc.UpdateInput{ID: page.Rows[0].ID, ExpectedRevision: page.Rows[0].Revision, Title: &updated}); err != nil {
		t.Fatal(err)
	}
	if _, err := remove.CallCommand(t.Context(), "delete", rpc.ItemInput{ID: page.Rows[0].ID}); err != nil {
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
	if result, err := lookup("original", string(changed)); result != nil || !errors.Is(rpc.DecodeError(err), idempotency.ErrKeyConflict) {
		t.Fatalf("wrong input became evidence: %+v,%v", result, err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE commands SET reply='{"wrong":"result"}' WHERE id='tool:original'`); err != nil {
		t.Fatal(err)
	}
	if result, err := lookup("original", string(arguments)); result != nil || err == nil {
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

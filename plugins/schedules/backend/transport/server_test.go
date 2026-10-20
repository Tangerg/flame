package transport

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/storage/directories"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/plugins/schedules/backend/rpc"
	scheduledb "github.com/Tangerg/flame/plugins/schedules/backend/sqlite"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/idempotency"
	"github.com/Tangerg/flame/runtime/jsoninput"
	modelcatalog "github.com/Tangerg/flame/runtime/model/catalog"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestOriginalCreateResultIsHistoricalAndReadOnly(t *testing.T) {
	db, err := scheduledb.Open(t.Context(), filepath.Join(t.TempDir(), "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := sdk.NewServer(&sdk.Implementation{Name: "schedules-proof"}, nil)
	registered, err := RegisterServer(server, db, scheduledb.NewScheduleStore(db), directories.Resolver{}, modelcatalog.Selections{}, func(context.Context, pluginsdk.CallbackRef) (RunHost, func() error, error) {
		return nil, nil, errors.New("unexpected Run connection")
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := registered.Close(); err != nil {
			t.Error(err)
		}
	})
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
		return pluginsdk.ReadToolReceipt(t.Context(), session, id, "create_schedule", admitted)
	}
	if result, err := lookup("original", string(arguments)); err != nil || result != nil {
		t.Fatalf("absence: %+v,%v", result, err)
	}
	var metadata sdk.Meta
	if err := metadata.Set(pluginsdk.InvocationMetadataKey, capability.Invocation{ID: "original", Origin: capability.Model, Target: capability.Target{Scope: "installation", Reference: "schedules-owner"}}); err != nil {
		t.Fatal(err)
	}
	normal, err := session.CallTool(t.Context(), &sdk.CallToolParams{Meta: metadata, Name: "create_schedule", Arguments: input})
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
	refusedInput := map[string]any{"instructions": "invalid automation", "cron": "invalid", "workspace_path": input["workspace_path"]}
	refusedJSON, err := json.Marshal(refusedInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := metadata.Set(pluginsdk.InvocationMetadataKey, capability.Invocation{ID: "refused", Origin: capability.Model, Target: capability.Target{Scope: "installation", Reference: "schedules-owner"}}); err != nil {
		t.Fatal(err)
	}
	refusal, err := session.CallTool(t.Context(), &sdk.CallToolParams{Meta: metadata, Name: "create_schedule", Arguments: refusedInput})
	if err != nil || !refusal.IsError {
		t.Fatalf("normal refusal: %+v, %v", refusal, err)
	}
	refusedOutput, err := lookup("refused", string(refusedJSON))
	failure, known := errors.AsType[*toolcontract.Failure](err)
	if !known || refusedOutput == nil {
		t.Fatalf("original refusal not recovered: %+v, %v", refusedOutput, err)
	}
	text, _ := failure.Output().Text()
	if len(refusal.Content) != 1 {
		t.Fatalf("normal refusal output: %+v", refusal.Content)
	}
	normalText, ok := refusal.Content[0].(*sdk.TextContent)
	if !ok || text != normalText.Text {
		t.Fatalf("receipt changed original refusal: %q / %+v", text, refusal.Content)
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
	if schedules != 0 || creates != 2 {
		t.Fatalf("queries repeated creation: schedules=%d creates=%d", schedules, creates)
	}
}

func TestBackgroundSetupChecksRuntimeAndJoinsItsConnection(t *testing.T) {
	for _, mismatched := range []bool{false, true} {
		t.Run(fmt.Sprint(mismatched), func(t *testing.T) {
			db, err := scheduledb.Open(t.Context(), filepath.Join(t.TempDir(), "schedules.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if mismatched {
				if err := bindRuntime(t.Context(), db, "original"); err != nil {
					t.Fatal(err)
				}
			}
			server := sdk.NewServer(&sdk.Implementation{Name: "background-owner"}, nil)
			var opened, closed atomic.Int32
			registered, err := RegisterServer(server, db, scheduledb.NewScheduleStore(db), directories.Resolver{}, modelcatalog.Selections{}, func(context.Context, pluginsdk.CallbackRef) (RunHost, func() error, error) {
				opened.Add(1)
				return &observedRunHost{db: db, namespace: "attached"}, func() error { closed.Add(1); return nil }, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer registered.Close()
			client := sdk.NewClient(&sdk.Implementation{Name: "background-consumer"}, nil)
			if err := pluginsdk.RegisterClient(client); err != nil {
				t.Fatal(err)
			}
			left, right := sdk.NewInMemoryTransports()
			owner, err := server.Connect(t.Context(), left, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close()
			session, err := client.Connect(t.Context(), right, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			metadata, err := pluginsdk.CallbackMetadata(pluginsdk.CallbackRef{Endpoint: "http://127.0.0.1:1/", Call: strings.Repeat("a", 64), Credential: strings.Repeat("b", 64), MaxResponseBytes: 4096})
			if err != nil {
				t.Fatal(err)
			}
			err = pluginsdk.SetUp(t.Context(), session, []pluginsdk.SetupContribution{{ID: "schedules-provider", Export: "schedules", Requires: []pluginsdk.SetupDependency{}, Background: &capability.Target{Scope: "installation", Reference: "11111111-1111-4111-8111-111111111111"}}}, metadata)
			if (err != nil) != mismatched {
				t.Fatalf("setup mismatched=%v: %v", mismatched, err)
			}
			if opened.Load() != 1 {
				t.Fatalf("setup connections=%d", opened.Load())
			}
			if err := registered.Close(); err != nil {
				t.Fatal(err)
			}
			if closed.Load() != 1 {
				t.Fatalf("activation connection was not joined once: %d", closed.Load())
			}
			var rows int
			if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM runtime_target`).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			want := 0
			if mismatched {
				want = 1
			}
			if rows != want {
				t.Fatalf("setup advanced private Runtime binding: %d", rows)
			}
		})
	}
}

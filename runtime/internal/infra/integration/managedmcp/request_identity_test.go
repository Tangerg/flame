package managedmcp

import (
	"context"
	"encoding/json/jsontext"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestReceiptRetainsOriginalRawInputAcrossTypedConversionAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	schema := []string{CommandsSchema, `CREATE TABLE IF NOT EXISTS counter(value INTEGER NOT NULL)`, `INSERT INTO counter SELECT 0 WHERE NOT EXISTS(SELECT 1 FROM counter)`}
	db, err := sqlitedb.OpenSchema(t.Context(), path, schema)
	if err != nil {
		t.Fatal(err)
	}
	first := db
	t.Cleanup(func() { _ = first.Close() })
	type input struct {
		N float64 `json:"n"`
		Z string  `json:"z"`
	}
	connect := func() (*sdk.ClientSession, *sdk.ServerSession) {
		t.Helper()
		server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
		binding, err := pluginsdk.NewBackend(server)
		if err != nil {
			t.Fatal(err)
		}
		method, err := pluginsdk.NewMethod[input, string](pluginsdk.MethodConfig{Name: "update", Kind: capability.Mutation, Target: "installation", Description: "update"})
		if err != nil {
			t.Fatal(err)
		}
		if err := RegisterCommand(binding, db, method, nil, func(ctx context.Context, _ *sdk.ServerSession, _ input) (string, error) {
			_, err := sqlitedb.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=value+1`)
			return "saved", err
		}); err != nil {
			t.Fatal(err)
		}
		if err := RegisterReceipts(binding, db); err != nil {
			t.Fatal(err)
		}
		if err := binding.Publish(); err != nil {
			t.Fatal(err)
		}
		client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
		if err := pluginsdk.RegisterClient(client); err != nil {
			t.Fatal(err)
		}
		a, b := sdk.NewInMemoryTransports()
		owner, err := server.Connect(t.Context(), a, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = owner.Close() })
		session, err := client.Connect(t.Context(), b, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		return session, owner
	}
	session, owner := connect()
	raw := jsontext.Value(`{"z":"same","n":1.0}`)
	if value, err := pluginsdk.CallCommand[jsontext.Value, string](t.Context(), session, "update", "original", raw); err != nil || value != "saved" {
		t.Fatalf("mutation = %q, %v", value, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlitedb.OpenSchema(t.Context(), path, schema)
	if err != nil {
		t.Fatal(err)
	}
	db = reopened
	t.Cleanup(func() { _ = reopened.Close() })
	session, _ = connect()
	if value, found, err := pluginsdk.ReadReceipt[jsontext.Value, string](t.Context(), session, "original", "update", jsontext.Value(` { "n":1.0, "z":"same" } `)); err != nil || !found || value != "saved" {
		t.Fatalf("original raw receipt = %q, %t, %v", value, found, err)
	}
	if _, _, err := pluginsdk.ReadReceipt[input, string](t.Context(), session, "original", "update", input{N: 1, Z: "same"}); err == nil {
		t.Fatal("decoded DTO replaced original numeric identity")
	}
	if _, err := pluginsdk.CallCommand[jsontext.Value, string](t.Context(), session, "update", "original", jsontext.Value(`{"n":1,"z":"same"}`)); err == nil {
		t.Fatal("changed raw input replayed an earlier mutation")
	}
	var count int
	var admitted string
	if err := db.QueryRowContext(t.Context(), `SELECT value FROM counter`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("recovery or conflicting input repeated mutation: %d, %v", count, err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT input_json FROM commands WHERE id='original'`).Scan(&admitted); err != nil || admitted != `{"n":1.0,"z":"same"}` {
		t.Fatalf("private owner replaced raw request: %s, %v", admitted, err)
	}
}

func TestReceiptRegistrationRequiresACommandMethod(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "command-admission"}, nil)
	backend, err := pluginsdk.NewBackend(server)
	if err != nil {
		t.Fatal(err)
	}
	query, err := pluginsdk.NewMethod[struct{}, string](pluginsdk.MethodConfig{Name: "read", Kind: capability.Query, Target: "installation", Description: "read"})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []*pluginsdk.Method[struct{}, string]{nil, query} {
		if err := RegisterCommand(backend, nil, method, nil, func(context.Context, *sdk.ServerSession, struct{}) (string, error) { return "unexpected", nil }); err == nil {
			t.Fatal("receipt registration accepted a non-command method")
		}
	}
	if err := backend.Publish(); err == nil {
		t.Fatal("refused registration left a live export")
	}
}

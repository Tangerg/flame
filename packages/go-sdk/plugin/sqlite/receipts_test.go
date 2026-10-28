package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/sqlite"
)

func TestBusinessMutationAndSuccessOrRefusalReceiptAreAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	schema := []string{CommandsSchema, `CREATE TABLE counter (value INTEGER NOT NULL)`, `INSERT INTO counter VALUES(0)`}
	db, err := sqlite.OpenSchema(t.Context(), path, schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	known := errors.New("business conflict")
	classify := func(err error) string {
		if errors.Is(err, known) {
			return "conflict"
		}
		return ""
	}
	handler := func(ctx context.Context, input string) (string, error) {
		if _, err := sqlite.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=value+1`); err != nil {
			return "", err
		}
		switch input {
		case "known":
			return "", known
		case "unknown":
			return "", errors.New("commit outcome unknown")
		default:
			return "saved", nil
		}
	}
	_, err = Command(t.Context(), db, "update", "refused", receiptInput(t, "known"), classify, handler)
	failure, ok := errors.AsType[*portable.Refusal](err)
	if !ok || failure.Code != "conflict" {
		t.Fatalf("known refusal=%v", err)
	}
	value, err := Command(t.Context(), db, "update", "success", receiptInput(t, "valid"), classify, handler)
	if err != nil || value != "saved" {
		t.Fatalf("success=%q, %v", value, err)
	}
	_, err = Command(t.Context(), db, "update", "unconfirmed", receiptInput(t, "unknown"), classify, handler)
	if err == nil {
		t.Fatal("unknown mutation succeeded")
	}
	if _, ok := errors.AsType[*portable.Refusal](err); ok {
		t.Fatalf("unknown became known=%v", err)
	}
	var count, total int
	if err := db.QueryRowContext(t.Context(), `SELECT value FROM counter`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("refused/unknown mutation survived rollback=%d, %v", count, err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM commands`).Scan(&total); err != nil || total != 2 {
		t.Fatalf("receipt count=%d, %v", total, err)
	}
	for _, key := range []struct{ id, input string }{{"refused", "known"}, {"success", "valid"}} {
		_, err := Command(t.Context(), db, "update", key.id, receiptInput(t, key.input), classify, func(context.Context, string) (string, error) {
			t.Fatal("receipt replay invoked mutation")
			return "", nil
		})
		if key.id == "refused" {
			if _, ok := errors.AsType[*portable.Refusal](err); !ok {
				t.Fatalf("refusal replay=%v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM commands WHERE id='unconfirmed'`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("unknown receipt fabricated=%d, %v", rows, err)
	}
}

func TestFormerDigestReceiptSchemasAreRefusedWithoutMutation(t *testing.T) {
	for _, column := range []string{"result", "reply"} {
		t.Run(column, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "private.db")
			former := `CREATE TABLE IF NOT EXISTS commands (id TEXT PRIMARY KEY, method TEXT NOT NULL, input_digest TEXT NOT NULL, ` + column + ` TEXT NOT NULL CHECK(json_valid(` + column + `)))`
			db, err := sqlite.OpenSchema(t.Context(), path, []string{former})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), `INSERT INTO commands VALUES('retained','review.update','original','{"saved":true}')`); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if current, err := sqlite.OpenSchema(t.Context(), path, []string{CommandsSchema}); err == nil {
				_ = current.Close()
				t.Fatal("former private receipt schema was converted or accepted")
			}
			raw, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			var result string
			if err := raw.QueryRowContext(t.Context(), `SELECT `+column+` FROM commands WHERE id='retained'`).Scan(&result); err != nil || result != `{"saved":true}` {
				t.Fatalf("refusal changed private data = %q, %v", result, err)
			}
		})
	}
}

func receiptInput[T any](t *testing.T, value T) jsonvalue.Value {
	t.Helper()
	input, err := jsonvalue.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestFormerCompletedOnlyReceiptSchemaIsRefusedWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	former := `CREATE TABLE IF NOT EXISTS commands (id TEXT PRIMARY KEY, method TEXT NOT NULL, business_input_json TEXT NOT NULL CHECK(json_valid(business_input_json)), reply TEXT NOT NULL CHECK(json_valid(reply)))`
	db, err := sqlite.OpenSchema(t.Context(), path, []string{former})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO commands VALUES('original','maintain','{"captured":true}','{"value":{"Published":true}}')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if current, err := sqlite.OpenSchema(t.Context(), path, []string{CommandsSchema}); err == nil {
		_ = current.Close()
		t.Fatal("completed-only schema was interpreted as pending-capable")
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var input, reply string
	if err := raw.QueryRowContext(t.Context(), `SELECT business_input_json,reply FROM commands WHERE id='original'`).Scan(&input, &reply); err != nil || input != `{"captured":true}` || reply != `{"value":{"Published":true}}` {
		t.Fatalf("schema refusal changed retained evidence: %s, %s, %v", input, reply, err)
	}
}

func TestCommandDerivesTypedMutationInputFromOriginalReceiptInput(t *testing.T) {
	db, err := sqlite.OpenSchema(t.Context(), filepath.Join(t.TempDir(), "private.db"), []string{CommandsSchema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	type input struct {
		N int64 `json:"n"`
	}
	original, err := jsonvalue.Parse([]byte(`{"n":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Command(t.Context(), db, "retain", "original", original, nil, func(_ context.Context, value input) (int64, error) { return value.N, nil })
	if err != nil || got != 9007199254740993 {
		t.Fatalf("mutation input = (%d, %v)", got, err)
	}
	var retained string
	if err := db.QueryRowContext(t.Context(), `SELECT business_input_json FROM commands WHERE id='original'`).Scan(&retained); err != nil || retained != original.String() {
		t.Fatalf("receipt input = (%q, %v)", retained, err)
	}
	malformed := receiptInput(t, "not an input object")
	if _, err := Command(t.Context(), db, "retain", "malformed", malformed, nil, func(context.Context, input) (bool, error) {
		t.Fatal("malformed input reached mutation")
		return false, nil
	}); err == nil {
		t.Fatal("malformed typed input was accepted")
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM commands WHERE id='malformed'`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("invalid input created a receipt = (%d, %v)", rows, err)
	}
}

func TestCommandAdmissionRefusesMethodsWithoutReceiptIdentity(t *testing.T) {
	db, err := sqlite.OpenSchema(t.Context(), filepath.Join(t.TempDir(), "private.db"), []string{CommandsSchema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	input := receiptInput(t, "original")
	for _, method := range []struct{ name, value string }{
		{"empty", ""},
		{"space", "two methods"},
		{"control", "two\tmethods"},
		{"invalid utf8", "method\xff"},
		{"overlong", strings.Repeat("m", portable.ProfileLimits().MaxTokenBytes+1)},
	} {
		t.Run(method.name, func(t *testing.T) {
			calls := 0
			_, err := Command(t.Context(), db, method.value, "mutation", input, nil, func(context.Context, string) (bool, error) {
				calls++
				return true, nil
			})
			if err == nil || calls != 0 {
				t.Fatalf("invalid method reached mutation: calls=%d, err=%v", calls, err)
			}
			prepared, reply, err := AdmitCommand(t.Context(), db, method.value, "external", input, func(context.Context) error {
				calls++
				return nil
			})
			if err == nil || calls != 0 || prepared != nil || len(reply) != 0 {
				t.Fatalf("invalid method acquired external admission: calls=%d, command=%v, reply=%s, err=%v", calls, prepared, reply, err)
			}
			var rows int
			if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM commands`).Scan(&rows); err != nil || rows != 0 {
				t.Fatalf("invalid method retained command state: rows=%d, err=%v", rows, err)
			}
		})
	}
}

func TestCommandPreservesPublishedNullableInputAndExactNumbers(t *testing.T) {
	type input struct {
		Name  *string `json:"name" jsonschema:"nullable"`
		Value any     `json:"value"`
	}
	method, err := qimumcp.NewMethod[input, bool](qimumcp.MethodConfig{
		Name: "numbers.save", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Save an optional name and original number.",
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := portable.AdmitMethod(method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.OpenSchema(t.Context(), filepath.Join(t.TempDir(), "private.db"), []string{CommandsSchema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	admitted, err := jsonvalue.Parse([]byte(`{"name":null,"value":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateInput(admitted); err != nil {
		t.Fatalf("published input refused: %v", err)
	}
	calls := 0
	handler := func(_ context.Context, value input) (bool, error) {
		calls++
		encoded, err := jsonvalue.Encode(value)
		if err != nil || value.Name != nil || encoded != admitted {
			t.Fatalf("admitted nullable input or number changed: %s, %v", encoded.String(), err)
		}
		return true, nil
	}
	value, err := Command(t.Context(), db, method.Name(), "nullable", admitted, nil, handler)
	if err != nil || !value || calls != 1 {
		t.Fatalf("published input narrowed by receipt adapter: value=%t, calls=%d, err=%v", value, calls, err)
	}
	unknown, err := jsonvalue.Parse([]byte(`{"name":null,"value":9007199254740993,"unknown":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Command(t.Context(), db, method.Name(), "unknown", unknown, nil, handler); err == nil || calls != 1 {
		t.Fatalf("unknown member reached mutation: calls=%d, err=%v", calls, err)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM commands`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("unknown member retained command state: rows=%d, err=%v", rows, err)
	}
}

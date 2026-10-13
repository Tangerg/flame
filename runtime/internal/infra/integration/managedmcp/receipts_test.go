package managedmcp

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"path/filepath"
	"testing"

	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestBusinessMutationAndSuccessOrRefusalReceiptAreAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	schema := []string{CommandsSchema, `CREATE TABLE counter (value INTEGER NOT NULL)`, `INSERT INTO counter VALUES(0)`}
	db, err := sqlitedb.OpenSchema(t.Context(), path, schema)
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
		if _, err := sqlitedb.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=value+1`); err != nil {
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
	_, err = Command(t.Context(), db, "update", "refused", "known", receiptInput(t, "known"), classify, handler)
	failure, ok := errors.AsType[*capability.Refusal](err)
	if !ok || failure.Code != "conflict" {
		t.Fatalf("known refusal=%v", err)
	}
	value, err := Command(t.Context(), db, "update", "success", "valid", receiptInput(t, "valid"), classify, handler)
	if err != nil || value != "saved" {
		t.Fatalf("success=%q, %v", value, err)
	}
	_, err = Command(t.Context(), db, "update", "unconfirmed", "unknown", receiptInput(t, "unknown"), classify, handler)
	if err == nil {
		t.Fatal("unknown mutation succeeded")
	}
	if _, ok := errors.AsType[*capability.Refusal](err); ok {
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
		_, err := Command(t.Context(), db, "update", key.id, key.input, receiptInput(t, key.input), classify, func(context.Context, string) (string, error) {
			t.Fatal("receipt replay invoked mutation")
			return "", nil
		})
		if key.id == "refused" {
			if _, ok := errors.AsType[*capability.Refusal](err); !ok {
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
			db, err := sqlitedb.OpenSchema(t.Context(), path, []string{former})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), `INSERT INTO commands VALUES('retained','review.update','original','{"saved":true}')`); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if current, err := sqlitedb.OpenSchema(t.Context(), path, []string{CommandsSchema}); err == nil {
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

func receiptInput[T any](t *testing.T, value T) jsoninput.Input {
	t.Helper()
	input, err := jsoninput.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestFormerCompletedOnlyReceiptSchemaIsRefusedWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	former := `CREATE TABLE IF NOT EXISTS commands (id TEXT PRIMARY KEY, method TEXT NOT NULL, business_input_json TEXT NOT NULL CHECK(json_valid(business_input_json)), reply TEXT NOT NULL CHECK(json_valid(reply)))`
	db, err := sqlitedb.OpenSchema(t.Context(), path, []string{former})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO commands VALUES('original','maintain','{"captured":true}','{"value":{"Published":true}}')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if current, err := sqlitedb.OpenSchema(t.Context(), path, []string{CommandsSchema}); err == nil {
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

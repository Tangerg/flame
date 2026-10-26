package sqlite

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormerUnboundRuntimeSchemaIsRefusedWithoutChangingCapturedWork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP TABLE runtime_target`,
		`INSERT INTO commands(id,method,business_input_json,reply) VALUES('retained','schedules.dispatch','{"runId":"run_original"}',NULL)`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := Open(t.Context(), path); err == nil {
		_ = reopened.Close()
		t.Fatal("former unbound private database was admitted")
	} else if !strings.Contains(err.Error(), "open a fresh data directory") {
		t.Fatalf("schema refusal=%v", err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var input string
	var reply sql.NullString
	if err := raw.QueryRowContext(t.Context(), `SELECT business_input_json,reply FROM commands WHERE id='retained'`).Scan(&input, &reply); err != nil || input != `{"runId":"run_original"}` || reply.Valid {
		t.Fatalf("refused opening changed retained work: %s, %+v, %v", input, reply, err)
	}
	var targetTables int
	if err := raw.QueryRowContext(t.Context(), `SELECT count(*) FROM sqlite_master WHERE name='runtime_target'`).Scan(&targetTables); err != nil || targetTables != 0 {
		t.Fatalf("refused opening converted the schema: %d, %v", targetTables, err)
	}
}

func TestFormerManualCaptureWithoutCommandOwnerIsRefusedWithoutConversion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP TRIGGER immutable_manual_request`,
		`DROP TABLE manual_firings`,
		`CREATE TABLE manual_firings (run_id TEXT PRIMARY KEY, captured_request_json TEXT NOT NULL CHECK(json_valid(captured_request_json)), state TEXT NOT NULL CHECK(state IN ('pending','accepted')))`,
		`INSERT INTO manual_firings(run_id,captured_request_json,state) VALUES('run_original','{"Invocation":{"ID":"original"}}','pending')`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := Open(t.Context(), path); err == nil {
		_ = reopened.Close()
		t.Fatal("manual schema without a receipt command was admitted")
	} else if !strings.Contains(err.Error(), "open a fresh data directory") {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var captured, state string
	if err := raw.QueryRowContext(t.Context(), `SELECT captured_request_json,state FROM manual_firings WHERE run_id='run_original'`).Scan(&captured, &state); err != nil || captured != `{"Invocation":{"ID":"original"}}` || state != "pending" {
		t.Fatalf("refused opening rewrote original work: %s, %s, %v", captured, state, err)
	}
}

func TestFormerCallContextInputIsRefusedWithoutChangingPrivateEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	const input = `{"Grant":{"Credential":"former-call-context"},"captured":true}`
	if _, err := db.ExecContext(t.Context(), `INSERT INTO commands(id,method,business_input_json,reply) VALUES('retained','dispatch',?,NULL)`, input); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `ALTER TABLE commands RENAME COLUMN business_input_json TO input_json`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if current, err := Open(t.Context(), path); err == nil {
		_ = current.Close()
		t.Fatal("former call-context input was admitted as business-only input")
	} else if !strings.Contains(err.Error(), "open a fresh data directory") {
		t.Fatalf("schema refusal=%v", err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var retained string
	var reply sql.NullString
	if err := raw.QueryRowContext(t.Context(), `SELECT input_json,reply FROM commands WHERE id='retained'`).Scan(&retained, &reply); err != nil || retained != input || reply.Valid {
		t.Fatalf("refusal changed retained evidence: %s, %+v, %v", retained, reply, err)
	}
	var columns int
	if err := raw.QueryRowContext(t.Context(), `SELECT count(*) FROM pragma_table_info('commands') WHERE name='business_input_json'`).Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("refusal converted the schema: %d, %v", columns, err)
	}
}

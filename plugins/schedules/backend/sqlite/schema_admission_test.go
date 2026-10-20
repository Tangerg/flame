package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/plugins/schedules/backend/sqlite"
)

func TestFormerUnboundRuntimeSchemaIsRefusedWithoutChangingCapturedWork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	db, err := sqlite.Open(t.Context(), path)
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
	if reopened, err := sqlite.Open(t.Context(), path); err == nil {
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

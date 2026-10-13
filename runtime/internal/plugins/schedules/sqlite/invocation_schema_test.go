package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/sqlite"
)

func TestFormerTriggerStorageIsRefusedWithoutChangingItsData(t *testing.T) {
	for _, statement := range []string{
		`ALTER TABLE schedule_firings RENAME COLUMN invocation_json TO former_invocation`,
		`ALTER TABLE manual_firings RENAME COLUMN captured_request_json TO request`,
	} {
		t.Run(statement, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "schedules.db")
			db, err := sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), `INSERT INTO commands(id,method,business_input_json,reply) VALUES('retained','dispatch','{}',NULL)`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if opened, err := sqlite.Open(t.Context(), path); err == nil {
				_ = opened.Close()
				t.Fatal("former trigger storage was admitted")
			}
			raw, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			var input string
			var reply sql.NullString
			if err := raw.QueryRowContext(t.Context(), `SELECT business_input_json,reply FROM commands WHERE id='retained'`).Scan(&input, &reply); err != nil || input != "{}" || reply.Valid {
				t.Fatalf("refusal changed retained work: %s, %+v, %v", input, reply, err)
			}
		})
	}
}

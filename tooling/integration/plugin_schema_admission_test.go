package integration

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	memorysqlite "github.com/Tangerg/flame/plugins/memory/backend/sqlite"
	schedulesqlite "github.com/Tangerg/flame/plugins/schedules/backend/sqlite"
)

func TestFormerCallContextInputIsRefusedWithoutChangingPrivateEvidence(t *testing.T) {
	for _, backend := range []struct {
		name string
		open func(context.Context, string) (*sql.DB, error)
	}{{"memory", memorysqlite.Open}, {"schedules", schedulesqlite.Open}} {
		t.Run(backend.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "private.db")
			db, err := backend.open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			const input = `{"Grant":{"Credential":"former-call-context"},"captured":true}`
			if _, err := db.ExecContext(t.Context(), `INSERT INTO commands(id,method,business_input_json,reply) VALUES('retained','maintain',?,NULL)`, input); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), `ALTER TABLE commands RENAME COLUMN business_input_json TO input_json`); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if current, err := backend.open(t.Context(), path); err == nil {
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
		})
	}
}

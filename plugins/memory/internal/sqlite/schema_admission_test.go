package sqlite

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
)

func TestRevisionlessMemoryIsRefusedWithoutMigratingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	item, _, err := NewAgentMemoryStore(db).Add(t.Context(), memory.ScopeUser, "", "retained fact", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `ALTER TABLE agent_memory_items DROP COLUMN revision`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if current, err := Open(t.Context(), path); err == nil {
		_ = current.Close()
		t.Fatal("revisionless memory was admitted")
	} else if !strings.Contains(err.Error(), "open a fresh data directory") {
		t.Fatalf("schema refusal: %v", err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var content string
	if err := raw.QueryRowContext(t.Context(), `SELECT content FROM agent_memory_items WHERE id=?`, item.ID().String()).Scan(&content); err != nil || content != item.Content() {
		t.Fatalf("refusal changed original content: %q, %v", content, err)
	}
}

func TestFormerCallContextInputIsRefusedWithoutChangingPrivateEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	db, err := Open(t.Context(), path)
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

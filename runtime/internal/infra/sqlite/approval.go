package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// ApprovalRuleStore persists approval rules in SQLite. It satisfies the
// Application consumer interface structurally without importing it. This is the
// persistent home for fine-grained "remember this decision" rules. Put is an
// upsert by deterministic rule id, replacing the decision and source authority.
// The DB must have been opened via [Open] so the approval_rules table exists.
type ApprovalRuleStore struct {
	db *sql.DB
}

// NewApprovalRuleStore binds the rule store to db.
func NewApprovalRuleStore(db *sql.DB) *ApprovalRuleStore {
	return &ApprovalRuleStore{db: db}
}

func (a *ApprovalRuleStore) Put(ctx context.Context, r approval.Rule) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("sqlite: put approval rule: %w", err)
	}
	_, err := conn(ctx, a.db).ExecContext(ctx, putApprovalRuleSQL, approvalRuleArgs(r)...)
	if err != nil {
		return fmt.Errorf("sqlite: put approval rule: %w", err)
	}
	return nil
}

func (a *ApprovalRuleStore) Visible(ctx context.Context, sessionID, projectDir string, limit int) ([]approval.Rule, error) {
	if err := validateOptionalSessionResource("list visible approval rules", sessionID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > approval.MaximumVisibleRules+1 {
		return nil, fmt.Errorf("sqlite: list approval rules: invalid limit %d", limit)
	}
	// Scope predicate expressed as a WHERE clause — the mirror of approval's
	// visible(): session rules for this session, project rules for this cwd
	// (skipped entirely when the call has no cwd), and all global rules.
	rows, err := conn(ctx, a.db).QueryContext(ctx,
		`SELECT id, scope, scope_key, tool_ref, source_fingerprint, subject_type, subject, decision FROM approval_rules
		 WHERE (scope = 'session' AND scope_key = ?)
		    OR (scope = 'project' AND ? <> '' AND scope_key = ?)
		    OR scope = 'global'
		 ORDER BY scope, scope_key, tool_ref, subject_type, subject, id
		 LIMIT ?`,
		sessionID, projectDir, projectDir, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list approval rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []approval.Rule
	for rows.Next() {
		var r approval.Rule
		var scope, decision, reference string
		if err := rows.Scan(&r.ID, &scope, &r.ScopeKey, &reference, &r.SourceFingerprint, &r.Subject.Type, &r.Subject.Value, &decision); err != nil {
			return nil, fmt.Errorf("sqlite: scan approval rule: %w", err)
		}
		r.Tool, err = tool.ParseRef(reference)
		if err != nil {
			return nil, err
		}
		r.Scope = approval.Scope(scope)
		r.Decision = approval.Decision(decision)
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("sqlite: decode approval rule %q: %w", r.ID, err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list approval rules: %w", err)
	}
	return out, nil
}

func (a *ApprovalRuleStore) Delete(ctx context.Context, id string) error {
	if _, err := conn(ctx, a.db).ExecContext(ctx, `DELETE FROM approval_rules WHERE id = ?`, id); err != nil {
		return fmt.Errorf("sqlite: delete approval rule: %w", err)
	}
	return nil
}

func (a *ApprovalRuleStore) DeleteSession(ctx context.Context, sessionID string) error {
	if err := validateSessionResource("delete Session approval rules", sessionID); err != nil {
		return err
	}
	if _, err := conn(ctx, a.db).ExecContext(ctx,
		`DELETE FROM approval_rules WHERE scope = 'session' AND scope_key = ?`, sessionID); err != nil {
		return fmt.Errorf("sqlite: delete session approval rules: %w", err)
	}
	return nil
}

const approvalRulesSchema = `CREATE TABLE IF NOT EXISTS approval_rules (
 id TEXT PRIMARY KEY,
 scope TEXT NOT NULL,
 scope_key TEXT NOT NULL DEFAULT '',
 tool_ref TEXT NOT NULL,
 source_fingerprint TEXT NOT NULL,
 mcp_server TEXT REFERENCES mcp_servers(name) ON DELETE CASCADE,
 subject_type TEXT NOT NULL CHECK (subject_type IN ('all','exact','glob')),
 subject TEXT NOT NULL,
 decision TEXT NOT NULL CHECK (decision IN ('allow','deny')),
 CHECK ((subject_type = 'all' AND subject = '') OR (subject_type <> 'all' AND subject <> '')),
 CHECK ((mcp_server IS NULL AND tool_ref NOT LIKE 'mcp:%') OR
        (mcp_server IS NOT NULL AND substr(tool_ref,1,length(mcp_server)+5) = 'mcp:' || mcp_server || ':'))
)`
const putApprovalRuleSQL = `INSERT INTO approval_rules
 (id,scope,scope_key,tool_ref,source_fingerprint,mcp_server,subject_type,subject,decision)
 VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET decision=excluded.decision, source_fingerprint=excluded.source_fingerprint`

func approvalRuleArgs(r approval.Rule) []any {
	var server any
	if r.Tool.Kind() == tool.MCPKind {
		server = r.Tool.Server().String()
	}
	return []any{r.ID, string(r.Scope), r.ScopeKey, r.Tool.String(), r.SourceFingerprint, server, string(r.Subject.Type), r.Subject.Value, string(r.Decision)}
}

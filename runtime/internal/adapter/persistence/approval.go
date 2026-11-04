package persistence

import (
	"context"
	"fmt"
	"strings"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	framevalue "github.com/Tangerg/frame/value"
)

// ApprovalRuleStore persists approval rules in SQLite. It satisfies the
// Application consumer interface structurally without importing it. This is the
// persistent home for fine-grained "remember this decision" rules. Put is an
// upsert by deterministic rule id, replacing the decision and source authority.
// The DB must have been opened via [Open] so the approval_rules table exists.
type ApprovalRuleStore struct{ db *database }

// newApprovalRuleStore binds the rule store to db.
func newApprovalRuleStore(db *database) *ApprovalRuleStore {
	return &ApprovalRuleStore{db: db}
}
func (a *ApprovalRuleStore) Put(ctx context.Context, r approval.Rule) error {
	return a.db.write(ctx, func(ctx context.Context) error {
		_, err := a.db.connection(ctx).ExecContext(ctx, putApprovalRuleSQL, approvalRuleArgs(r)...)
		if err != nil {
			return fmt.Errorf("sqlite: put approval rule: %w", err)
		}
		return nil
	})
}

// Scope predicate expressed as a WHERE clause — the mirror of approval's
// visible(): session rules for this session, project rules for this cwd
// (skipped entirely when the call has no cwd), and all global rules.
func (a *ApprovalRuleStore) Visible(ctx context.Context, sessionID, projectDir string, limit int) ([]approval.Rule, error) {
	return scopedValue(ctx, a.db.read, func(ctx context.Context) ([]approval.Rule, error) {
		if err := validateOptionalSessionResource("list visible approval rules", sessionID); err != nil {
			return nil, err
		}
		if limit <= 0 || limit > approval.MaximumVisibleRules+1 {
			return nil, fmt.Errorf("sqlite: list approval rules: invalid limit %d", limit)
		}
		rows, err := a.db.connection(ctx).QueryContext(ctx, `SELECT r.id, r.scope, r.scope_key, r.tool_kind, r.tool_name, s.origin, s.installation_id, s.name,
		        r.source_fingerprint, r.subject_type, r.subject, r.decision
		   FROM approval_rules r LEFT JOIN mcp_sources s ON s.id = r.mcp_source
		 WHERE (r.scope = 'session' AND r.scope_key = ?)
		    OR (r.scope = 'project' AND ? <> '' AND r.scope_key = ?)
		    OR r.scope = 'global'
		 ORDER BY r.scope, r.scope_key, r.tool_kind, s.origin <> 'user', s.installation_id, s.name, r.tool_name, r.subject_type, r.subject, r.id
		 LIMIT ?`, sessionID, projectDir, projectDir, limit)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list approval rules: %w", err)
		}
		defer func() {
			_ = rows.Close()
		}()
		var out []approval.Rule
		for rows.Next() {
			var storedID, scope, scopeKey, decision, kind, name, fingerprint string
			var subject approval.Subject
			var source storedMCPSource
			if err := rows.Scan(&storedID, &scope, &scopeKey, &kind, &name, &source.origin, &source.installation, &source.name, &fingerprint, &subject.Type, &subject.Value, &decision); err != nil {
				return nil, fmt.Errorf("sqlite: scan approval rule: %w", err)
			}
			ref, err := decodeRuleTool(kind, name, source)
			if err != nil {
				return nil, fmt.Errorf("sqlite: decode approval rule %q tool: %w", storedID, err)
			}
			var digest framevalue.Digest
			if err := digest.UnmarshalText([]byte(fingerprint)); err != nil {
				return nil, fmt.Errorf("sqlite: decode approval rule %q fingerprint: %w", storedID, err)
			}
			ruleScope, err := approval.RestoreRuleScope(approval.Scope(scope), scopeKey)
			if err != nil {
				return nil, fmt.Errorf("sqlite: decode approval rule %q: %w", storedID, err)
			}
			r, err := approval.NewRule(ruleScope, ref, digest, subject, approval.Decision(decision))
			if err != nil {
				return nil, fmt.Errorf("sqlite: decode approval rule %q: %w", storedID, err)
			}
			if storedID != r.ID() {
				return nil, fmt.Errorf("sqlite: approval rule %q is stored under another rule's identity", storedID)
			}
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: list approval rules: %w", err)
		}
		return out, nil
	})
}
func (a *ApprovalRuleStore) Delete(ctx context.Context, id string) error {
	return a.db.write(ctx, func(ctx context.Context) error {
		if _, err := a.db.connection(ctx).ExecContext(ctx, `DELETE FROM approval_rules WHERE id = ?`, id); err != nil {
			return fmt.Errorf("sqlite: delete approval rule: %w", err)
		}
		return nil
	})
}
func (a *ApprovalRuleStore) DeleteSession(ctx context.Context, sessionID string) error {
	return a.db.write(ctx, func(ctx context.Context) error {
		if err := validateSessionResource("delete Session approval rules", sessionID); err != nil {
			return err
		}
		if _, err := a.db.connection(ctx).ExecContext(ctx, `DELETE FROM approval_rules WHERE scope = 'session' AND scope_key = ?`, sessionID); err != nil {
			return fmt.Errorf("sqlite: delete session approval rules: %w", err)
		}
		return nil
	})
}

// approvalRulesSchema keeps the closed scope and tool reference vocabularies
// with their owners. The id is derived from the scope, its key, the tool
// reference and the subject, but not the decision: remembering a decision for
// the same rule replaces it.
func approvalRulesSchema() string {
	kinds := make([]string, 0, len(tool.RefKinds()))
	for _, kind := range tool.RefKinds() {
		kinds = append(kinds, string(kind))
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS approval_rules (
 id TEXT PRIMARY KEY,
 scope TEXT NOT NULL CHECK (scope IN (%[1]s)),
 scope_key TEXT NOT NULL DEFAULT '',
 tool_kind TEXT NOT NULL CHECK (tool_kind IN (%[2]s)),
 tool_name TEXT NOT NULL CHECK (tool_name <> ''),
 mcp_source INTEGER REFERENCES mcp_sources(id) ON DELETE CASCADE,
 source_fingerprint TEXT NOT NULL,
 subject_type TEXT NOT NULL CHECK (subject_type IN ('all','exact','glob')),
 subject TEXT NOT NULL,
 decision TEXT NOT NULL CHECK (decision IN ('allow','deny')),
 CHECK ((scope = '%[3]s') = (scope_key = '')),
 CHECK ((subject_type = 'all' AND subject = '') OR (subject_type <> 'all' AND subject <> '')),
 CHECK ((tool_kind = '%[4]s') = (mcp_source IS NOT NULL))
)`, sqlValues(string(approval.ScopeSession), string(approval.ScopeProject), string(approval.ScopeGlobal)), sqlValues(kinds...), approval.ScopeGlobal, tool.MCPKind)
}

const putApprovalRuleSQL = `INSERT INTO approval_rules
 (id,scope,scope_key,tool_kind,tool_name,mcp_source,source_fingerprint,subject_type,subject,decision)
 VALUES (?,?,?,?,?,` + mcpSourceIDQuery + `,?,?,?,?) ON CONFLICT(id) DO UPDATE SET decision=excluded.decision, source_fingerprint=excluded.source_fingerprint`

func approvalRuleArgs(r approval.Rule) []any {
	ref := r.Tool()
	args := []any{r.ID(), string(r.Scope().Kind()), r.Scope().Key(), string(ref.Kind())}
	var source []any
	switch ref.Kind() {
	case tool.BuiltInKind:
		name, _ := ref.BuiltIn()
		args = append(args, string(name))
		source = noMCPSource()
	case tool.MCPKind:
		server, remote, _ := ref.MCP()
		args = append(args, remote.String())
		source = mcpSourceArgs(server)
	}
	args = append(args, source...)
	subject := r.Subject()
	return append(args, r.SourceFingerprint().String(), string(subject.Type), subject.Value, string(r.Decision()))
}
func decodeRuleTool(kind, name string, source storedMCPSource) (tool.Ref, error) {
	switch tool.RefKind(kind) {
	case tool.BuiltInKind:
		return tool.BuiltIn(builtincontract.Name(name))
	case tool.MCPKind:
		server, err := source.id()
		if err != nil {
			return tool.Ref{}, err
		}
		remote, err := mcpserver.ParseRemoteToolName(name)
		if err != nil {
			return tool.Ref{}, err
		}
		return tool.MCP(server, remote)
	default:
		return tool.Ref{}, fmt.Errorf("sqlite: unknown tool kind %q", kind)
	}
}

// sqlValues spells a closed vocabulary as an SQL value list.
func sqlValues(values ...string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	return strings.Join(quoted, ", ")
}

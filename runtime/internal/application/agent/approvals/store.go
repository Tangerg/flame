package approvals

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

// RuleStore persists approval rules. Matching and precedence live in rule.go;
// implementations own storage validation and scope filtering on read so corrupt
// records never enter policy evaluation.
type RuleStore interface {
	// Put upserts a rule by its id (deterministic over scope/key/tool/subject),
	// so re-remembering the same rule replaces the decision rather than piling
	// duplicates.
	Put(ctx context.Context, r approval.Rule) error

	// Visible returns at most limit rules reachable from a session: its session-scoped
	// rules (ScopeKey == sessionID), its project's rules (ScopeKey ==
	// projectDir), and all global rules. Any tool — the domain filters by tool.
	// The result slice transfers ownership to the caller.
	Visible(ctx context.Context, sessionID, projectDir string, limit int) ([]approval.Rule, error)

	// Delete removes one rule by id; removing a missing id is not an error.
	Delete(ctx context.Context, id string) error
}

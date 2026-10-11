package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	agentmemory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
)

func Open(ctx context.Context, path string) (*sql.DB, error) {
	return sqlitedb.OpenSchema(ctx, path, []string{managedmcp.CommandsSchema,
		// Append-only per-project fact ledger. day is the daily-ledger partition
		// (YYYY-MM-DD); seq is both stable ordering and the curation watermark.
		// A content digest deduplicates facts independently, so mixed old/new
		// extraction batches never lose their new members.
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS agent_memory_ledger (
			seq         INTEGER PRIMARY KEY AUTOINCREMENT,
			project     TEXT    NOT NULL,
			day         TEXT    NOT NULL,
			session_id  TEXT    NOT NULL,
			fact        TEXT    NOT NULL CHECK (length(fact) BETWEEN 1 AND %d),
			digest      TEXT    NOT NULL,
			captured_at INTEGER NOT NULL,
			UNIQUE(project, digest)
		)`, agentmemory.MaxContentCharacters),
		`CREATE INDEX IF NOT EXISTS idx_agent_memory_ledger_project
			ON agent_memory_ledger(project, seq)`,
		// Memory items retain human review and edits independently of extraction.
		// digest is the content identity (a reconcile matches unchanged items by
		// it to keep their id stable); the unique (scope, project, digest) index
		// dedups a fact across auto/user/pinned rows. origin 'auto' | 'user',
		// scope 'project' | 'user'. Pinned items are always injected and never
		// auto-pruned. session_id/day carry provenance.
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS agent_memory_items (
			id         TEXT    PRIMARY KEY CHECK (
				length(id) = %[1]d AND
				substr(id, 1, length('%[2]s')) = '%[2]s' AND
				substr(id, length('%[2]s') + 1) NOT GLOB '*[^0-9a-f]*'
			),
			scope      TEXT    NOT NULL CHECK (scope IN ('%[4]s', '%[5]s')),
			project    TEXT    NOT NULL DEFAULT '',
			content    TEXT    NOT NULL CHECK (length(content) BETWEEN 1 AND %[3]d),
			digest     TEXT    NOT NULL,
			origin     TEXT    NOT NULL CHECK (origin IN ('%[6]s', '%[7]s')),
			-- HITL review lifecycle: active (approved/injected/searched), pending
			-- (proposed, awaiting review), rejected (tombstone that blocks the same
			-- fact from being re-proposed).
			status     TEXT    NOT NULL DEFAULT '%[8]s' CHECK (status IN ('%[8]s', '%[9]s', '%[10]s')),
			pinned     INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
			session_id TEXT    NOT NULL DEFAULT '',
			day        TEXT    NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			-- Search-owned derived cache. embedding_space identifies the exact
			-- vector space that produced the content vector; both values are empty
			-- until a configured embedder lazily fills them.
			embedding_space TEXT NOT NULL DEFAULT '',
			embedding       BLOB NOT NULL DEFAULT x'',
			CHECK ((scope = '%[4]s' AND project <> '') OR (scope = '%[5]s' AND project = '')),
			CHECK (origin <> '%[7]s' OR status = '%[8]s'),
			CHECK ((embedding_space = '' AND length(embedding) = 0) OR
			       (embedding_space <> '' AND length(embedding) > 0 AND length(embedding) %% 4 = 0)),
			UNIQUE(scope, project, digest)
		)`,
			agentmemory.MaximumItemIDCharacters,
			agentmemory.ItemIDPrefix,
			agentmemory.MaxContentCharacters,
			agentmemory.ScopeProject,
			agentmemory.ScopeUser,
			agentmemory.OriginAuto,
			agentmemory.OriginUser,
			agentmemory.StatusActive,
			agentmemory.StatusPending,
			agentmemory.StatusRejected,
		),
		`CREATE INDEX IF NOT EXISTS idx_agent_memory_items_scope
			ON agent_memory_items(scope, project)`,
		// Per-project curation watermark (the highest ledger seq already folded
		// into items). Kept apart from the items so a reconcile advances it with a
		// single compare-and-swap update.
		`CREATE TABLE IF NOT EXISTS agent_memory_state (
			project    TEXT    PRIMARY KEY,
			watermark  INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,
	})
}

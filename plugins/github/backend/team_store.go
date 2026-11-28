package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
)

const teamSchema = `
CREATE TABLE IF NOT EXISTS team_grants (
	id TEXT PRIMARY KEY, scope TEXT NOT NULL, project TEXT NOT NULL, delivery TEXT NOT NULL,
	member TEXT NOT NULL, role TEXT NOT NULL, project_access INTEGER NOT NULL, machine_access INTEGER NOT NULL,
	expires_at INTEGER NOT NULL, revoked_at INTEGER
);
CREATE INDEX IF NOT EXISTS team_grant_scope ON team_grants(scope);
CREATE TABLE IF NOT EXISTS team_audit (
	grant_id TEXT NOT NULL, scope TEXT NOT NULL, action TEXT NOT NULL, occurred_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS team_audit_scope ON team_audit(scope);`

const teamColumns = "id, project, delivery, member, role, project_access, machine_access, expires_at, revoked_at"

type rowScanner interface{ Scan(...any) error }

func scanGrant(row rowScanner) (wire.TeamGrant, error) {
	var grant wire.TeamGrant
	var expires int64
	var revoked sql.NullInt64
	err := row.Scan(&grant.ID, &grant.ProjectID, &grant.DeliveryID, &grant.Member, &grant.Role,
		&grant.ProjectAccess, &grant.MachineAccess, &expires, &revoked)
	grant.ExpiresAt = teamTime(expires)
	if revoked.Valid {
		grant.RevokedAt = new(teamTime(revoked.Int64))
	}
	return grant, err
}

func teamTime(when int64) instant.Timestamp {
	return instant.Timestamp{Time: time.Unix(0, when).UTC()}
}

func (s *store) grant(ctx context.Context, scope, id string) (wire.TeamGrant, error) {
	var grant wire.TeamGrant
	err := s.db.RunReadSnapshot(ctx, func(tx context.Context) error {
		var err error
		grant, err = scanGrant(s.db.Executor(tx).QueryRowContext(tx,
			"SELECT "+teamColumns+" FROM team_grants WHERE scope=? AND id=?", scope, id))
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return wire.TeamGrant{}, refused(refusalNotFound, "team grant is unavailable")
	}
	return grant, err
}

func (s *store) addGrant(ctx context.Context, scope string, grant wire.TeamGrant, now time.Time) error {
	_, err := s.db.Executor(ctx).ExecContext(ctx, "INSERT INTO team_grants(id,scope,project,delivery,member,role,project_access,machine_access,expires_at) VALUES(?,?,?,?,?,?,?,?,?)",
		grant.ID, scope, grant.ProjectID, grant.DeliveryID, grant.Member, grant.Role, grant.ProjectAccess, grant.MachineAccess, grant.ExpiresAt.UnixNano())
	if err != nil {
		return err
	}
	return s.recordTeamEvent(ctx, scope, grant.ID, "granted", now)
}

func (s *store) revokeGrant(ctx context.Context, scope, id string, now time.Time) error {
	result, err := s.db.Executor(ctx).ExecContext(ctx, "UPDATE team_grants SET revoked_at=? WHERE scope=? AND id=? AND revoked_at IS NULL", now.UnixNano(), scope, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return refused(refusalConflict, "team grant changed")
	}
	if err != nil {
		return err
	}
	return s.recordTeamEvent(ctx, scope, id, "revoked", now)
}

func (s *store) recordTeamEvent(ctx context.Context, scope, id, action string, now time.Time) error {
	_, err := s.db.Executor(ctx).ExecContext(ctx, "INSERT INTO team_audit(grant_id,scope,action,occurred_at) VALUES(?,?,?,?)", id, scope, action, now.UnixNano())
	return err
}

func (s *store) teamBoard(ctx context.Context, scope string, board *wire.TeamBoard) error {
	return s.db.RunReadSnapshot(ctx, func(tx context.Context) error {
		reader := s.db.Executor(tx)
		rows, err := reader.QueryContext(tx, "SELECT "+teamColumns+" FROM team_grants WHERE scope=? ORDER BY rowid DESC LIMIT 100", scope)
		if err != nil {
			return err
		}
		for rows.Next() {
			grant, err := scanGrant(rows)
			if err != nil {
				return errors.Join(err, rows.Close())
			}
			board.Grants = append(board.Grants, grant)
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return err
		}
		audit, err := reader.QueryContext(tx, "SELECT grant_id,action,occurred_at FROM team_audit WHERE scope=? ORDER BY rowid DESC LIMIT 200", scope)
		if err != nil {
			return err
		}
		defer audit.Close()
		for audit.Next() {
			var event wire.TeamAuditEvent
			var when int64
			if err := audit.Scan(&event.GrantID, &event.Action, &when); err != nil {
				return err
			}
			event.At = teamTime(when)
			board.Audit = append(board.Audit, event)
		}
		return audit.Err()
	})
}

package persistence

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/project"
	"github.com/Tangerg/flame/runtime/internal/domain/run"

	"github.com/Tangerg/frame/sqlite"
)

func (r *RunStore) RecordProjectBackground(ctx context.Context, draft run.Draft, source project.BackgroundSource, installation identity.InstallationID) error {
	return r.db.write(ctx, func(ctx context.Context) error {
		db := r.db.connection(ctx)
		var attached string
		var archived bool
		err := db.QueryRowContext(ctx, `SELECT COALESCE(s.project_id, ''), COALESCE(p.archived, 0)
			FROM sessions s LEFT JOIN projects p ON p.id = s.project_id WHERE s.id = ?`, draft.SessionID).Scan(&attached, &archived)
		if err != nil {
			return err
		}
		if attached != source.ProjectID.String() {
			return fmt.Errorf("%w: the captured Project does not own the Run's Session", project.ErrInvalid)
		}
		policy, active, admitted, err := backgroundAvailability(ctx, db, source.ProjectID, draft.CreatedAt)
		if err != nil {
			return err
		}
		if archived || policy.Paused || active >= policy.MaxActive || admitted >= policy.MaxDailyRuns {
			return project.ErrBackgroundCapacity
		}
		_, err = db.ExecContext(ctx, `INSERT INTO project_background_runs
			(run_id, project_id, session_id, installation_id, kind, source_id, source_version, title, admitted_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, draft.RunID, source.ProjectID, draft.SessionID, installation.String(),
			source.Kind, source.ID, source.Version, source.Title, draft.CreatedAt.UnixNano())
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: source version already admitted by another command", project.ErrInvalid)
		}
		return err
	})
}

func backgroundAvailability(ctx context.Context, db sqlite.Executor, id identity.ProjectID, now time.Time) (project.BackgroundPolicy, int, int, error) {
	policy := project.BackgroundPolicy{MaxActive: 2, MaxDailyRuns: 20, PullRequests: []string{}}
	var refs string
	err := db.QueryRowContext(ctx, `SELECT revision, paused, max_active, max_daily_runs, pull_requests
		FROM project_background_policy WHERE project_id = ?`, id).
		Scan(&policy.Revision, &policy.Paused, &policy.MaxActive, &policy.MaxDailyRuns, &refs)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return policy, 0, 0, err
	default:
		if err := json.Unmarshal([]byte(refs), &policy.PullRequests); err != nil {
			return policy, 0, 0, err
		}
	}
	start, end := project.DailyWindow(now)
	var active, admitted int
	err = db.QueryRowContext(ctx, `SELECT
		COUNT(CASE WHEN r.outcome = '' THEN 1 END),
		COUNT(CASE WHEN b.admitted_at >= ? AND b.admitted_at < ? THEN 1 END)
		FROM project_background_runs b LEFT JOIN runs r ON r.run_id = b.run_id
		WHERE b.project_id = ?`, start.UnixNano(), end.UnixNano(), id).Scan(&active, &admitted)
	return policy, active, admitted, err
}

func (s *ProjectStore) Background(ctx context.Context, id identity.ProjectID, now time.Time) (project.BackgroundActivity, error) {
	var activity project.BackgroundActivity
	err := s.db.read(ctx, func(ctx context.Context) error {
		if _, err := s.Get(ctx, id); err != nil {
			return err
		}
		db := s.db.connection(ctx)
		var err error
		activity.Policy, activity.Active, activity.Admitted, err = backgroundAvailability(ctx, db, id, now)
		if err != nil {
			return err
		}
		rows, err := db.QueryContext(ctx, `SELECT b.kind, b.source_id, b.source_version, b.title, b.session_id, b.run_id,
			COALESCE(r.state, 'retired'), COALESCE(r.outcome, ''), b.admitted_at
			FROM project_background_runs b LEFT JOIN runs r ON r.run_id = b.run_id
			WHERE b.project_id = ? ORDER BY b.admitted_at DESC, b.run_id DESC LIMIT 50`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			row := project.BackgroundRun{BackgroundSource: project.BackgroundSource{ProjectID: id}}
			var admittedAt int64
			if err := rows.Scan(&row.Kind, &row.ID, &row.Version, &row.Title, &row.SessionID, &row.RunID,
				&row.Status, &row.Outcome, &admittedAt); err != nil {
				return err
			}
			row.AdmittedAt = time.Unix(0, admittedAt).UTC()
			activity.Recent = append(activity.Recent, row)
		}
		return rows.Err()
	})
	return activity, err
}

func (s *ProjectStore) SetBackground(ctx context.Context, id identity.ProjectID, policy project.BackgroundPolicy) (project.BackgroundPolicy, error) {
	if err := policy.Validate(); err != nil {
		return project.BackgroundPolicy{}, err
	}
	if policy.PullRequests == nil {
		policy.PullRequests = []string{}
	}
	refs, err := json.Marshal(policy.PullRequests)
	if err != nil {
		return project.BackgroundPolicy{}, err
	}
	expected := policy.Revision
	policy.Revision++
	err = s.db.write(ctx, func(ctx context.Context) error {
		if _, err := s.Get(ctx, id); err != nil {
			return err
		}
		db := s.db.connection(ctx)
		var result sql.Result
		var err error
		if expected == 0 {
			result, err = db.ExecContext(ctx, `INSERT INTO project_background_policy
				(project_id, revision, paused, max_active, max_daily_runs, pull_requests) VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT(project_id) DO NOTHING`, id, policy.Revision, policy.Paused, policy.MaxActive, policy.MaxDailyRuns, string(refs))
		} else {
			result, err = db.ExecContext(ctx, `UPDATE project_background_policy
				SET revision = ?, paused = ?, max_active = ?, max_daily_runs = ?, pull_requests = ?
				WHERE project_id = ? AND revision = ?`, policy.Revision, policy.Paused, policy.MaxActive, policy.MaxDailyRuns, string(refs), id, expected)
		}
		if err != nil {
			return err
		}
		updated, err := result.RowsAffected()
		if err == nil && updated != 1 {
			return project.ErrRevisionConflict
		}
		return err
	})
	return policy, err
}

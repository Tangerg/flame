package main

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Tangerg/flame/packages/go-sdk/skill"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	storage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
)

const reviewsSchema = `CREATE TABLE IF NOT EXISTS reviews (
	library TEXT NOT NULL, name TEXT NOT NULL, proposal TEXT NOT NULL UNIQUE,
	decision TEXT, PRIMARY KEY(library,name)
)`

func staleReview() error {
	return &portable.Refusal{Code: skilllearning.StaleReview, Message: "the proposal is no longer available for this decision"}
}

func registerReviews(server *qimumcp.Backend, db *sql.DB) error {
	propose, e1 := skilllearning.ProposeMethod()
	queue, e2 := skilllearning.QueueMethod()
	approve, e3 := skilllearning.ApproveMethod()
	confirm, e4 := skilllearning.ConfirmMethod()
	reject, e5 := skilllearning.RejectMethod()
	if err := errors.Join(e1, e2, e3, e4, e5); err != nil {
		return err
	}
	reviews := reviewQueue{db: db}
	return errors.Join(
		reviewCommand(server, db, propose, reviews.stage),
		queue.RegisterServer(server, nil, func(ctx context.Context, _ *sdk.ServerSession, cmd qimumcp.Command[skilllearning.Libraries]) (skilllearning.Queue, error) {
			return reviews.pending(ctx, cmd.Input)
		}),
		reviewCommand(server, db, approve, reviews.authorize),
		reviewCommand(server, db, confirm, reviews.confirm),
		reviewCommand(server, db, reject, reviews.reject),
	)
}

func reviewCommand[P, R any](server *qimumcp.Backend, db *sql.DB, method *qimumcp.Method[P, R], apply func(context.Context, *sdk.ServerSession, P) (R, error)) error {
	return method.RegisterServer(server, nil, func(ctx context.Context, session *sdk.ServerSession, command qimumcp.Command[P]) (R, error) {
		var zero R
		inv, err := qimumcp.InvocationFromContext(ctx)
		if err != nil {
			return zero, err
		}
		if inv.ID != string(command.ID) {
			return zero, portable.ErrInvocationConflict
		}
		input, err := command.RequestInput()
		if err != nil {
			return zero, err
		}
		return storage.Command(ctx, db, method.Name(), inv.ID, input, nil, func(ctx context.Context, p P) (R, error) { return apply(ctx, session, p) })
	})
}

type reviewQueue struct{ db *sql.DB }

func (r reviewQueue) stage(ctx context.Context, _ *sdk.ServerSession, draft skilllearning.Draft) (skilllearning.ReviewRef, error) {
	inv, err := qimumcp.InvocationFromContext(ctx)
	if err != nil {
		return skilllearning.ReviewRef{}, err
	}
	return r.stageCommand(ctx, inv.ID, draft)
}

func (r reviewQueue) stageCommand(ctx context.Context, command string, draft skilllearning.Draft) (skilllearning.ReviewRef, error) {
	ref := skilllearning.ReviewRef{Command: command}
	conn := sqlite.Connection(ctx, r.db)
	// Replacing a pending name is free at capacity. An in-flight publication
	// keeps the slot reserved until the filesystem owner reconciles its permit.
	result, err := conn.ExecContext(ctx, `INSERT INTO reviews(library,name,proposal)
	SELECT ?,?,? WHERE EXISTS(SELECT 1 FROM reviews WHERE library=? AND name=?) OR (SELECT count(*) FROM reviews WHERE library=?) < ?
	ON CONFLICT(library,name) DO UPDATE SET proposal=excluded.proposal WHERE decision IS NULL`, draft.Library, draft.Skill.Name, ref.Command, draft.Library, draft.Skill.Name, draft.Library, skill.MaxPendingProposalsPerScope)
	if err := reviewChanged(result, err); err != nil {
		return skilllearning.ReviewRef{}, err
	}
	return ref, nil
}

func (r reviewQueue) pending(ctx context.Context, libs skilllearning.Libraries) (skilllearning.Queue, error) {
	rows, err := sqlite.Connection(ctx, r.db).QueryContext(ctx, `SELECT proposal FROM reviews WHERE library IN (?,?) ORDER BY library,name`, libs.Project, libs.User)
	if err != nil {
		return skilllearning.Queue{}, err
	}
	defer rows.Close()
	queue := skilllearning.Queue{Proposals: []skilllearning.ReviewRef{}}
	for rows.Next() {
		var ref skilllearning.ReviewRef
		if err := rows.Scan(&ref.Command); err != nil {
			return skilllearning.Queue{}, err
		}
		queue.Proposals = append(queue.Proposals, ref)
	}
	return queue, rows.Err()
}

func humanDecision(ctx context.Context) (portable.Invocation, error) {
	inv, err := qimumcp.InvocationFromContext(ctx)
	if err != nil {
		return inv, err
	}
	if inv.Origin != extension.Human {
		return inv, &portable.Refusal{Code: "human_required", Message: "an explicit human review is required"}
	}
	return inv, nil
}

func (r reviewQueue) authorize(ctx context.Context, _ *sdk.ServerSession, ref skilllearning.ReviewRef) (skilllearning.Permit, error) {
	inv, err := humanDecision(ctx)
	if err != nil {
		return skilllearning.Permit{}, err
	}
	var permit skilllearning.Permit
	err = sqlite.Connection(ctx, r.db).QueryRowContext(ctx, `UPDATE reviews SET decision=COALESCE(decision,?) WHERE proposal=? RETURNING decision`, inv.ID, ref.Command).Scan(&permit.Decision)
	if errors.Is(err, sql.ErrNoRows) {
		return permit, staleReview()
	}
	return permit, err
}

func (r reviewQueue) confirm(ctx context.Context, _ *sdk.ServerSession, effect skilllearning.Publication) (struct{}, error) {
	if _, err := humanDecision(ctx); err != nil {
		return struct{}{}, err
	}
	result, err := sqlite.Connection(ctx, r.db).ExecContext(ctx, `DELETE FROM reviews WHERE proposal=? AND decision=?`, effect.Proposal.Command, effect.Permit.Decision)
	return struct{}{}, reviewChanged(result, err)
}
func (r reviewQueue) reject(ctx context.Context, _ *sdk.ServerSession, ref skilllearning.ReviewRef) (struct{}, error) {
	if _, err := humanDecision(ctx); err != nil {
		return struct{}{}, err
	}
	result, err := sqlite.Connection(ctx, r.db).ExecContext(ctx, `DELETE FROM reviews WHERE proposal=? AND decision IS NULL`, ref.Command)
	return struct{}{}, reviewChanged(result, err)
}
func reviewChanged(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return staleReview()
	}
	return err
}

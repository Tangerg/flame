package main

import (
	"context"
	"database/sql"
	"errors"

	framesqlite "github.com/Tangerg/frame/sqlite"

	"github.com/Tangerg/flame/packages/go-sdk/skill"
	framemcp "github.com/Tangerg/frame/process/mcp"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	storage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const reviewsSchema = `CREATE TABLE IF NOT EXISTS reviews (
	library TEXT NOT NULL, name TEXT NOT NULL, proposal TEXT NOT NULL UNIQUE,
	decision TEXT, PRIMARY KEY(library,name)
)`

func staleReview() error {
	return &framemcp.Refusal{Code: skilllearning.StaleReview, Message: "the proposal is no longer available for this decision"}
}

func registerReviews(server *pluginmcp.Backend, db *framesqlite.Database) error {
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
		queue.RegisterServer(server, nil, func(ctx context.Context, _ *sdk.ServerSession, cmd pluginmcp.Command[skilllearning.Libraries]) (skilllearning.Queue, error) {
			return reviews.pending(ctx, cmd.Input)
		}),
		reviewCommand(server, db, approve, reviews.authorize),
		reviewCommand(server, db, confirm, reviews.confirm),
		reviewCommand(server, db, reject, reviews.reject),
	)
}

func reviewCommand[P, R any](server *pluginmcp.Backend, db *framesqlite.Database, method *pluginmcp.Method[P, R], prepare func(context.Context, *sdk.ServerSession, P) (storage.CommandPlan[R], error)) error {
	return method.RegisterServer(server, nil, func(ctx context.Context, session *sdk.ServerSession, command pluginmcp.Command[P]) (R, error) {
		var zero R
		inv, err := pluginmcp.InvocationFromContext(ctx)
		if err != nil {
			return zero, err
		}
		if inv.ID != string(command.ID) {
			return zero, framemcp.ErrInvocationConflict
		}
		input, err := command.RequestInput()
		if err != nil {
			return zero, err
		}
		return storage.Command(ctx, db, method.Name(), inv.ID, input, nil, func(ctx context.Context, input P) (storage.CommandPlan[R], error) {
			return prepare(ctx, session, input)
		})
	})
}

type reviewQueue struct{ db *framesqlite.Database }

func (r reviewQueue) stage(ctx context.Context, _ *sdk.ServerSession, draft skilllearning.Draft) (storage.CommandPlan[skilllearning.ReviewRef], error) {
	invocation, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return storage.CommandPlan[skilllearning.ReviewRef]{}, err
	}
	persist, err := r.prepareStage(ctx, invocation.ID, draft)
	if err != nil {
		return storage.CommandPlan[skilllearning.ReviewRef]{}, err
	}
	return storage.NewCommandPlan(skilllearning.ReviewRef{Command: invocation.ID}, persist)
}

func (r reviewQueue) prepareStage(ctx context.Context, command string, draft skilllearning.Draft) (func(context.Context) error, error) {
	connection := framesqlite.Connection(ctx, r.db)
	var decision sql.NullString
	err := connection.QueryRowContext(ctx, `SELECT decision FROM reviews WHERE library=? AND name=?`, draft.Library, draft.Skill.Name).Scan(&decision)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		var count int
		if err := connection.QueryRowContext(ctx, `SELECT count(*) FROM reviews WHERE library=?`, draft.Library).Scan(&count); err != nil {
			return nil, err
		}
		if count >= skill.MaxPendingProposalsPerScope {
			return nil, staleReview()
		}
	case err != nil:
		return nil, err
	case decision.Valid:
		// An authorized publication keeps its slot until the filesystem owner settles it.
		return nil, staleReview()
	}
	library, name := draft.Library, draft.Skill.Name
	return func(ctx context.Context) error {
		_, err := framesqlite.Connection(ctx, r.db).ExecContext(ctx, `INSERT INTO reviews(library,name,proposal) VALUES(?,?,?) ON CONFLICT(library,name) DO UPDATE SET proposal=excluded.proposal`, library, name, command)
		return err
	}, nil
}

func (r reviewQueue) pending(ctx context.Context, libraries skilllearning.Libraries) (skilllearning.Queue, error) {
	queue := skilllearning.Queue{Proposals: []skilllearning.ReviewRef{}}
	err := r.db.Read(ctx, func(reader framesqlite.Reader) error {
		rows, err := reader.QueryContext(ctx, `SELECT proposal FROM reviews WHERE library IN (?,?) ORDER BY library,name`, libraries.Project, libraries.User)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ref skilllearning.ReviewRef
			if err := rows.Scan(&ref.Command); err != nil {
				return err
			}
			queue.Proposals = append(queue.Proposals, ref)
		}
		return rows.Err()
	})
	return queue, err
}

func humanDecision(ctx context.Context) (framemcp.Invocation, error) {
	inv, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return inv, err
	}
	if inv.Origin != extension.Human {
		return inv, &framemcp.Refusal{Code: "human_required", Message: "an explicit human review is required"}
	}
	return inv, nil
}

func (r reviewQueue) decision(ctx context.Context, proposal string) (sql.NullString, error) {
	var decision sql.NullString
	err := framesqlite.Connection(ctx, r.db).QueryRowContext(ctx, `SELECT decision FROM reviews WHERE proposal=?`, proposal).Scan(&decision)
	if errors.Is(err, sql.ErrNoRows) {
		return decision, staleReview()
	}
	return decision, err
}

func (r reviewQueue) authorize(ctx context.Context, _ *sdk.ServerSession, ref skilllearning.ReviewRef) (storage.CommandPlan[skilllearning.Permit], error) {
	invocation, err := humanDecision(ctx)
	if err != nil {
		return storage.CommandPlan[skilllearning.Permit]{}, err
	}
	decision, err := r.decision(ctx, ref.Command)
	if err != nil {
		return storage.CommandPlan[skilllearning.Permit]{}, err
	}
	if decision.Valid {
		return storage.NewCommandPlan(skilllearning.Permit{Decision: decision.String}, nil)
	}
	return storage.NewCommandPlan(skilllearning.Permit{Decision: invocation.ID}, func(ctx context.Context) error {
		_, err := framesqlite.Connection(ctx, r.db).ExecContext(ctx, `UPDATE reviews SET decision=? WHERE proposal=?`, invocation.ID, ref.Command)
		return err
	})
}

func (r reviewQueue) confirm(ctx context.Context, _ *sdk.ServerSession, publication skilllearning.Publication) (storage.CommandPlan[struct{}], error) {
	if _, err := humanDecision(ctx); err != nil {
		return storage.CommandPlan[struct{}]{}, err
	}
	decision, err := r.decision(ctx, publication.Proposal.Command)
	if err != nil {
		return storage.CommandPlan[struct{}]{}, err
	}
	if !decision.Valid || decision.String != publication.Permit.Decision {
		return storage.CommandPlan[struct{}]{}, staleReview()
	}
	return storage.NewCommandPlan(struct{}{}, func(ctx context.Context) error {
		_, err := framesqlite.Connection(ctx, r.db).ExecContext(ctx, `DELETE FROM reviews WHERE proposal=?`, publication.Proposal.Command)
		return err
	})
}

func (r reviewQueue) reject(ctx context.Context, _ *sdk.ServerSession, ref skilllearning.ReviewRef) (storage.CommandPlan[struct{}], error) {
	if _, err := humanDecision(ctx); err != nil {
		return storage.CommandPlan[struct{}]{}, err
	}
	decision, err := r.decision(ctx, ref.Command)
	if err != nil {
		return storage.CommandPlan[struct{}]{}, err
	}
	if decision.Valid {
		return storage.CommandPlan[struct{}]{}, staleReview()
	}
	return storage.NewCommandPlan(struct{}{}, func(ctx context.Context) error {
		_, err := framesqlite.Connection(ctx, r.db).ExecContext(ctx, `DELETE FROM reviews WHERE proposal=?`, ref.Command)
		return err
	})
}

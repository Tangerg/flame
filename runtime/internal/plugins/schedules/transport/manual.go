package transport

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"time"

	"github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

// Captured execution remains in the private queue. Stable identities locate that
// original capture after process loss without another route or result owner.
func runNow(ctx context.Context, db *sql.DB, store Store, connect RunConnector, command pluginsdk.Command[rpc.ItemInput]) (result app.RunReference, err error) {
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil {
		return result, err
	}
	if invocation.ID != command.ID {
		return result, errors.New("schedules: manual command differs from its calling owner")
	}
	input, err := command.RequestInput()
	if err != nil {
		return result, err
	}
	identityInput, err := json.Marshal([2]string{invocation.Target.Reference, command.ID})
	if err != nil {
		return result, err
	}
	digest := sha256.Sum256(identityInput)
	suffix := hex.EncodeToString(digest[:])
	sessionID, runID := identity.SessionPrefix+suffix, identity.RunPrefix+suffix
	var captured schedule.RunRequest
	var prepared *managedmcp.PreparedCommand
	var original []byte
	err = sqlitedb.RunInTx(ctx, db, func(ctx context.Context) error {
		var err error
		prepared, original, err = managedmcp.AdmitCommand(ctx, db, rpc.RunNow, command.ID, input, func(ctx context.Context) error {
			_, err := pluginsdk.CallbackFromContext(ctx)
			return err
		})
		if err != nil {
			return err
		}
		if prepared == nil {
			return nil
		}
		if !prepared.NewlyAdmitted() {
			captured, err = store.CapturedRun(ctx, runID)
			return err
		}
		definition, err := store.Get(ctx, command.Input.ID)
		if err == nil {
			captured, err = definition.ManualRun(sessionID, runID, time.Now(), invocation)
		}
		if err == nil {
			return store.QueueManual(ctx, captured)
		}
		reply, replyErr := pluginsdk.Reply(app.RunReference{}, err, rpc.Classify)
		if replyErr != nil {
			return replyErr
		}
		original, err = json.Marshal(reply)
		if err != nil {
			return err
		}
		if err := prepared.Complete(ctx, original); err != nil {
			return err
		}
		prepared = nil
		return nil
	})
	if err != nil {
		return result, err
	}
	if prepared == nil {
		return pluginsdk.DecodeReply[app.RunReference](original)
	}
	if captured.Invocation() != invocation || captured.ScheduleID() != command.Input.ID || captured.SessionID() != sessionID {
		return result, errors.New("schedules: manual request differs from its original capture")
	}
	runs, close, err := connectRuntime(ctx, db, connect)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, close()) }()
	dispatcher, err := app.NewDispatcher(dispatchCommit{DispatchStore: store, db: db, command: prepared}, runs)
	if err != nil {
		return result, err
	}
	return dispatcher.Dispatch(ctx, captured.RunID())
}

package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/packages/go-sdk/sqlite"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	rpc "github.com/Tangerg/flame/plugins/schedules/internal/contract"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
)

func runNow(ctx context.Context, db *sql.DB, store store, connect runConnector, command pluginsdk.Command[rpc.ItemInput]) (app.RunReference, error) {
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil {
		return app.RunReference{}, err
	}
	if invocation.ID != command.ID {
		return app.RunReference{}, errors.New("schedules: manual command differs from its calling owner")
	}
	input, err := command.RequestInput()
	if err != nil {
		return app.RunReference{}, err
	}
	return captureManual(ctx, db, store, connect, manualCommand{id: command.ID, input: command.Input, admitted: input})
}

type manualCommand struct {
	id       string
	input    rpc.ItemInput
	admitted jsonvalue.Value
}

// A logical caller and its private receipt command have distinct identities.
// The queue retains both so recovery never guesses a Tool's command namespace.
func captureManual(ctx context.Context, db *sql.DB, store store, connect runConnector, command manualCommand) (result app.RunReference, err error) {
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil {
		return result, err
	}
	identityInput, err := json.Marshal([2]string{invocation.Target.Reference, command.id})
	if err != nil {
		return result, err
	}
	digest := sha256.Sum256(identityInput)
	suffix := hex.EncodeToString(digest[:])
	sessionID, runID := identity.SessionPrefix+suffix, identity.RunPrefix+suffix
	var captured schedule.RunRequest
	var prepared *pluginstorage.PreparedCommand
	var original []byte
	err = sqlite.RunInTx(ctx, db, func(ctx context.Context) error {
		var err error
		prepared, original, err = pluginstorage.AdmitCommand(ctx, db, rpc.RunNow, command.id, command.admitted, func(ctx context.Context) error {
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
		definition, err := store.Get(ctx, command.input.ID)
		if err == nil {
			captured, err = definition.ManualRun(command.id, sessionID, runID, time.Now(), invocation)
		}
		if err == nil {
			return store.QueueManual(ctx, captured)
		}
		reply, replyErr := pluginsdk.EncodeReply(app.RunReference{}, err, classify)
		if replyErr != nil {
			return replyErr
		}
		original = reply.JSON()
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
	if captured.Invocation() != invocation || captured.CommandID() != command.id || captured.ScheduleID() != command.input.ID || captured.SessionID() != sessionID {
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

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"

	framecall "github.com/Tangerg/frame/call"
	framesqlite "github.com/Tangerg/frame/sqlite"
	framevalue "github.com/Tangerg/frame/value"
)

func runNow(ctx context.Context, db framesqlite.Database, store store, connect runConnector, command pluginmcp.Command[wire.ItemInput]) (app.RunReference, error) {
	invocation, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return app.RunReference{}, err
	}
	if invocation.ID != string(command.ID) {
		return app.RunReference{}, errors.New("schedules: manual command differs from its calling owner")
	}
	input, err := command.RequestInput()
	if err != nil {
		return app.RunReference{}, err
	}
	return captureManual(ctx, db, store, connect, manualCommand{id: string(command.ID), admitted: input})
}

type manualCommand struct {
	id       string
	admitted framevalue.JSON
}

// A logical caller and its private receipt command have distinct identities.
// The queue retains both so recovery never guesses a Tool's command namespace.
func captureManual(ctx context.Context, db framesqlite.Database, store store, connect runConnector, command manualCommand) (result app.RunReference, err error) {
	var input wire.ItemInput
	if err := command.admitted.Decode(&input); err != nil {
		return result, err
	}
	invocation, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return result, err
	}
	sessionID, runID, err := manualRunIdentity(invocation, command.id)
	if err != nil {
		return result, err
	}
	admission := manualAdmission{store: store, command: command, scheduleID: input.ID, invocation: invocation, sessionID: sessionID, runID: runID}
	if err := db.RunInTx(ctx, func(ctx context.Context) error { return admission.admit(ctx, db) }); err != nil {
		return result, errors.Join(pluginmcp.ErrOutcomeUnconfirmed, err)
	}
	if admission.prepared == nil {
		return pluginmcp.DecodeReply[app.RunReference](admission.original)
	}
	captured := admission.captured
	if captured.Invocation() != invocation || captured.CommandID() != command.id || captured.ScheduleID() != input.ID || captured.SessionID() != sessionID {
		return result, errors.New("schedules: manual request differs from its original capture")
	}
	runs, close, err := connectRuntime(ctx, db, connect)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, close()) }()
	dispatcher, err := app.NewDispatcher(dispatchCommit{DispatchStore: store, db: db, command: admission.prepared}, runs)
	if err != nil {
		return result, err
	}
	return dispatcher.Dispatch(ctx, captured.RunID())
}

// manualRunIdentity derives the Session and Run a manual request opens from
// its caller and command, so a retried request names the same Run.
func manualRunIdentity(invocation framecall.Invocation, commandID string) (string, string, error) {
	identityInput, err := json.Marshal([2]string{invocation.Target.Reference, commandID})
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(identityInput)
	suffix := hex.EncodeToString(digest[:])
	return identity.SessionPrefix + suffix, identity.RunPrefix + suffix, nil
}

// manualAdmission is one manual request's transaction: it either captures a
// Run to dispatch (prepared) or settles the command with a stored reply
// (original).
type manualAdmission struct {
	store      store
	command    manualCommand
	scheduleID string
	invocation framecall.Invocation
	sessionID  string
	runID      string

	captured schedule.RunRequest
	prepared *pluginstorage.PreparedCommand
	original []byte
}

func (m *manualAdmission) admit(ctx context.Context, db framesqlite.Database) error {
	prepared, original, err := pluginstorage.AdmitCommand(ctx, db, wire.RunNow, m.command.id, m.command.admitted, func(ctx context.Context) error {
		_, err := pluginmcp.CallbackFromContext(ctx)
		return err
	})
	m.prepared, m.original = prepared, original
	if errors.Is(err, framecall.ErrConflict) {
		conflict, encodeErr := pluginmcp.EncodeReply(app.RunReference{}, framecall.ErrConflict, nil)
		m.original, m.prepared = conflict.Bytes(), nil
		return encodeErr
	}
	if err != nil || prepared == nil {
		return err
	}
	if !prepared.NewlyAdmitted() {
		m.captured, err = m.store.CapturedRun(ctx, m.runID)
		return err
	}
	definition, err := m.store.Get(ctx, m.scheduleID)
	if err == nil {
		m.captured, err = definition.ManualRun(m.command.id, m.sessionID, m.runID, time.Now(), m.invocation)
	}
	if err == nil {
		return m.store.QueueManual(ctx, m.captured)
	}
	return m.refuse(ctx, err)
}

// refuse settles a newly admitted command whose Schedule refused the run, so
// a retry replays the refusal instead of trying again.
func (m *manualAdmission) refuse(ctx context.Context, refusal error) error {
	reply, err := pluginmcp.EncodeReply(app.RunReference{}, refusal, classify)
	if err != nil {
		return err
	}
	m.original = reply.Bytes()
	if err := m.prepared.Complete(ctx, m.original); err != nil {
		return err
	}
	m.prepared = nil
	return nil
}

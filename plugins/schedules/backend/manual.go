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
	"github.com/Tangerg/flame/plugins/schedules/internal/schedule"
	"github.com/Tangerg/flame/plugins/schedules/internal/scheduling"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"

	framecall "github.com/Tangerg/frame/call"
	framesqlite "github.com/Tangerg/frame/sqlite"
	framevalue "github.com/Tangerg/frame/value"
)

func runNow(ctx context.Context, db framesqlite.Database, store store, command pluginmcp.Command[wire.ItemInput]) (scheduling.RunReference, error) {
	invocation, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return scheduling.RunReference{}, err
	}
	if invocation.ID != string(command.ID) {
		return scheduling.RunReference{}, errors.New("schedules: manual command differs from its calling owner")
	}
	input, err := command.RequestInput()
	if err != nil {
		return scheduling.RunReference{}, err
	}
	return captureManual(ctx, db, store, manualCommand{id: string(command.ID), admitted: input})
}

type manualCommand struct {
	id       string
	admitted framevalue.JSON
}

func captureManual(ctx context.Context, db framesqlite.Database, store store, command manualCommand) (result scheduling.RunReference, err error) {
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
		return pluginmcp.DecodeReply[scheduling.RunReference](admission.original)
	}
	captured := admission.captured
	if captured.Invocation() != invocation || captured.CommandID() != command.id || captured.ScheduleID() != input.ID || captured.SessionID() != sessionID {
		return result, errors.New("schedules: manual request differs from its original capture")
	}
	runs, release, err := connectRuntime(ctx, db)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, release()) }()
	dispatcher, err := scheduling.NewDispatcher(dispatchCommit{DispatchStore: store, db: db, command: admission.prepared}, runs)
	if err != nil {
		return result, err
	}
	return dispatcher.Dispatch(ctx, captured.RunID())
}

func manualRunIdentity(invocation framecall.Invocation, commandID string) (identity.SessionID, identity.RunID, error) {
	identityInput, err := json.Marshal([2]string{invocation.Target.Reference, commandID})
	if err != nil {
		return identity.SessionID{}, identity.RunID{}, err
	}
	digest := sha256.Sum256(identityInput)
	suffix := hex.EncodeToString(digest[:])
	sessionID, err := identity.NewSession(suffix)
	if err != nil {
		return identity.SessionID{}, identity.RunID{}, err
	}
	runID, err := identity.NewRun(suffix)
	return sessionID, runID, err
}

type manualAdmission struct {
	store      store
	command    manualCommand
	scheduleID string
	invocation framecall.Invocation
	sessionID  identity.SessionID
	runID      identity.RunID

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
		conflict, encodeErr := pluginmcp.EncodeReply(scheduling.RunReference{}, framecall.ErrConflict, nil)
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

func (m *manualAdmission) refuse(ctx context.Context, refusal error) error {
	reply, err := pluginmcp.EncodeReply(scheduling.RunReference{}, refusal, classify)
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

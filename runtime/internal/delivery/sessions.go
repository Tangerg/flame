package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const restoreTypeField = "restoreType"

const (
	SessionsList     Name = "sessions.list"
	SessionsGet      Name = "sessions.get"
	sessionsSnapshot Name = "sessions.snapshot"
	SessionsCreate   Name = "sessions.create"
	SessionsUpdate   Name = "sessions.update"
	SessionsDelete   Name = "sessions.delete"
	sessionsFork     Name = "sessions.fork"
	sessionsRollback Name = "sessions.rollback"
	sessionsExport   Name = "sessions.export"
	SessionsImport   Name = "sessions.import"
)

func registerSessions(registry *Registry) {
	registerSessionTrajectory(registry)
	registerExportTrajectory(registry)
	registry.query(MethodMeta{Name: SessionsList, GoName: "ListSessions"},
		func(service interface {
			ListSessions(context.Context, protocol.ListSessionsRequest) (*protocol.Page[protocol.Session], error)
		}, ctx context.Context, request protocol.ListSessionsRequest) (*protocol.Page[protocol.Session], error) {
			return service.ListSessions(ctx, request)
		})

	registry.query(MethodMeta{
		Name: SessionsGet, GoName: "GetSession",
		Errors: []string{protocol.ErrSessionNotFound.Error()},
	}, func(service interface {
		GetSession(context.Context, string) (*protocol.Session, error)
	}, ctx context.Context, request protocol.GetSessionRequest) (*protocol.Session, error) {
		return service.GetSession(ctx, request.SessionID)
	})

	registry.query(MethodMeta{
		Name: sessionsSnapshot, GoName: "GetSessionSnapshot",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		Materializes:    []Name{ItemsList, RunsList, InterruptsList, planGet, goalsGet},
		CapabilityRules: descendantsRequireChildRuns(includeDescendantsParameter),
	}, func(service interface {
		GetSessionSnapshot(context.Context, protocol.GetSessionSnapshotRequest) (*protocol.SessionSnapshot, error)
	}, ctx context.Context, request protocol.GetSessionSnapshotRequest) (*protocol.SessionSnapshot, error) {
		return service.GetSessionSnapshot(ctx, request)
	})

	registry.command(MethodMeta{
		Name: SessionsCreate, GoName: "CreateSession",
		Errors: pluginProviderErrors(protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrProjectNotFound.Error()),
	}, func(service interface {
		CreateSession(context.Context, protocol.CreateSessionRequest) (*protocol.Session, error)
	}, ctx context.Context, request protocol.CreateSessionRequest) (*protocol.Session, error) {
		return service.CreateSession(ctx, request)
	}).withOwnedReplay(atomicReplay((interface {
		commitCreateSession(context.Context, protocol.CreateSessionRequest, atomicOutcome) error
	}).commitCreateSession))

	registry.command(MethodMeta{
		Name: SessionsUpdate, GoName: "UpdateSession",
		Errors: pluginProviderErrors(protocol.ErrSessionNotFound.
			Error(), protocol.ErrRevisionConflict.
			Error(), protocol.ErrWorkspaceUnavailable.
			Error(), protocol.ErrProjectNotFound.
			Error(), protocol.ErrSessionBusy.
			Error(),
		),
		CapabilityRules: []CapabilityRule{
			{
				When:     []FieldCondition{{Field: "workspace", Operator: OperatorPresent}},
				Requires: []string{protocol.FeatureRelocate},
			},
			{
				When:     []FieldCondition{{Field: "isolated", Operator: OperatorPresent}},
				Requires: []string{protocol.FeatureIsolation},
			},
		},
	}, func(service interface {
		UpdateSession(context.Context, protocol.UpdateSessionRequest) (*protocol.Session, error)
	}, ctx context.Context, request protocol.UpdateSessionRequest) (*protocol.Session, error) {
		return service.UpdateSession(ctx, request)
	}).withOwnedReplay(atomicReplay((interface {
		commitUpdateSession(context.Context, protocol.UpdateSessionRequest, atomicOutcome) error
	}).commitUpdateSession))

	registry.commandAck(MethodMeta{
		Name: SessionsDelete, GoName: "DeleteSession",
		Errors: []string{protocol.ErrSessionNotFound.Error()},
	}, func(service interface {
		DeleteSession(context.Context, string) error
	}, ctx context.Context, request protocol.DeleteSessionRequest) error {
		return service.DeleteSession(ctx, request.SessionID)
	}).withOwnedReplay(atomicReplay((interface {
		commitDeleteSession(context.Context, protocol.DeleteSessionRequest, atomicOutcome) error
	}).commitDeleteSession))

	registry.command(MethodMeta{
		Name: sessionsFork, GoName: "ForkSession",
		Errors: pluginProviderErrors(protocol.ErrSessionNotFound.
			Error(), protocol.ErrRunNotFound.
			Error(),
		),
	}, func(service interface {
		ForkSession(context.Context, protocol.ForkSessionRequest) (*protocol.Session, error)
	}, ctx context.Context, request protocol.ForkSessionRequest) (*protocol.Session, error) {
		return service.ForkSession(ctx, request)
	}).withOwnedReplay(atomicReplay((interface {
		commitForkSession(context.Context, protocol.ForkSessionRequest, atomicOutcome) error
	}).commitForkSession))

	registry.command(MethodMeta{
		Name: sessionsRollback, GoName: "RollbackSession",
		Errors: []string{protocol.ErrSessionNotFound.
			Error(), protocol.ErrRunNotFound.
			Error(), protocol.ErrSessionBusy.
			Error(), protocol.ErrCheckpointUnavailable.
			Error(), protocol.ErrCheckpointConflict.
			Error(),
		},
		CapabilityRules: []CapabilityRule{
			{
				When:     []FieldCondition{{Field: restoreTypeField, Operator: OperatorEquals, Value: string(protocol.RestoreFiles)}},
				Requires: []string{protocol.FeatureCheckpoints},
			},
			{
				When:     []FieldCondition{{Field: restoreTypeField, Operator: OperatorEquals, Value: string(protocol.RestoreBoth)}},
				Requires: []string{protocol.FeatureCheckpoints},
			},
		},
	}, func(service interface {
		RollbackSession(context.Context, protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error)
	}, ctx context.Context, request protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
		return service.RollbackSession(ctx, request)
	}).withOwnedReplay((interface {
		replayRollback(context.Context, protocol.RollbackSessionRequest, keyedCall) protocol.OperationResult
	}).replayRollback)

	registry.query(MethodMeta{
		Name: sessionsExport, GoName: "ExportSession",
		Errors:          pluginProviderErrors(protocol.ErrSessionNotFound.Error(), protocol.ErrSessionBusy.Error(), protocol.ErrExportTooLarge.Error()),
		CapabilityRules: requires(protocol.FeatureSessionExport),
	}, func(service interface {
		ExportSession(context.Context, protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error)
	}, ctx context.Context, request protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
		return service.ExportSession(ctx, request)
	})

	registry.command(MethodMeta{
		Name: SessionsImport, GoName: "ImportSession",
		Errors:          []string{protocol.ErrSessionBusy.Error(), protocol.ErrExportTooLarge.Error()},
		CapabilityRules: requires(protocol.FeatureSessionExport),
	}, func(service interface {
		ImportSession(context.Context, protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error)
	}, ctx context.Context, request protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error) {
		return service.ImportSession(ctx, request)
	}).withOwnedReplay(atomicReplay((interface {
		commitImportSession(context.Context, protocol.ImportSessionRequest, atomicOutcome) error
	}).commitImportSession))
}

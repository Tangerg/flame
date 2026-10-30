package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

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
	sessionsImport   Name = "sessions.import"
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
		Errors:       []string{protocol.ErrSessionNotFound.Error()},
		Materializes: []Name{ItemsList, RunsList, InterruptsList, planGet, goalsGet},
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "includeDescendants", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureSubagents},
		}},
	}, func(service interface {
		GetSessionSnapshot(context.Context, protocol.GetSessionSnapshotRequest) (*protocol.SessionSnapshot, error)
	}, ctx context.Context, request protocol.GetSessionSnapshotRequest) (*protocol.SessionSnapshot, error) {
		return service.GetSessionSnapshot(ctx, request)
	})

	registry.command(MethodMeta{
		Name: SessionsCreate, GoName: "CreateSession",
		Errors: pluginProviderErrors(protocol.ErrWorkspaceUnavailable.Error()),
	}, func(service interface {
		CreateSession(context.Context, protocol.CreateSessionRequest) (*protocol.Session, error)
	}, ctx context.Context, request protocol.CreateSessionRequest) (*protocol.Session, error) {
		return service.CreateSession(ctx, request)
	})

	// Setting workspace is a relocate, which is its own capability — hence a
	// conditional rule: the rest of sessions.update stays available when relocate
	// is off, instead of the whole method disappearing.
	registry.command(MethodMeta{
		Name: SessionsUpdate, GoName: "UpdateSession",
		Errors: pluginProviderErrors(protocol.ErrSessionNotFound.
			Error(), protocol.ErrRevisionConflict.
			Error(), protocol.ErrWorkspaceUnavailable.
			Error(),
		),
		CapabilityRules: []CapabilityRule{
			{
				When:     []FieldCondition{{Field: "workspace", Operator: OperatorPresent}},
				Requires: []string{protocol.FeatureRelocate},
			},
			// Present means "true" for an optional bool, so releasing isolation
			// stays available on a build that cannot grant it — a Session carried
			// here from a host that could must remain editable.
			{
				When:     []FieldCondition{{Field: "isolated", Operator: OperatorPresent}},
				Requires: []string{protocol.FeatureIsolation},
			},
		},
	}, func(service interface {
		UpdateSession(context.Context, protocol.UpdateSessionRequest) (*protocol.Session, error)
	}, ctx context.Context, request protocol.UpdateSessionRequest) (*protocol.Session, error) {
		return service.UpdateSession(ctx, request)
	})

	registry.commandAck(MethodMeta{
		Name: SessionsDelete, GoName: "DeleteSession",
		Errors: []string{protocol.ErrSessionNotFound.Error()},
	}, func(service interface {
		DeleteSession(context.Context, string) error
	}, ctx context.Context, request protocol.DeleteSessionRequest) error {
		return service.DeleteSession(ctx, request.SessionID)
	})

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
	})

	// restoreType files/both rewind the working tree from a shadow-git snapshot,
	// which needs features.checkpoints; the default history rollback needs nothing
	// Two rules rather than one because the contract states the
	// requirement per value, and a generated schema reads them as two if/then.
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
				When:     []FieldCondition{{Field: "restoreType", Operator: OperatorEquals, Value: string(protocol.RestoreFiles)}},
				Requires: []string{protocol.FeatureCheckpoints},
			},
			{
				When:     []FieldCondition{{Field: "restoreType", Operator: OperatorEquals, Value: string(protocol.RestoreBoth)}},
				Requires: []string{protocol.FeatureCheckpoints},
			},
		},
	}, func(service interface {
		RollbackSession(context.Context, protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error)
	}, ctx context.Context, request protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
		return service.RollbackSession(ctx, request)
	})

	registry.query(MethodMeta{
		Name: sessionsExport, GoName: "ExportSession",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureSessionExport),
	}, func(service interface {
		ExportSession(context.Context, protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error)
	}, ctx context.Context, request protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
		return service.ExportSession(ctx, request)
	})

	registry.command(MethodMeta{
		Name: sessionsImport, GoName: "ImportSession",
		CapabilityRules: requires(protocol.FeatureSessionExport),
	}, func(service interface {
		ImportSession(context.Context, protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error)
	}, ctx context.Context, request protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error) {
		return service.ImportSession(ctx, request)
	})
}

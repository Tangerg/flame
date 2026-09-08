package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	SessionsList     Name = "sessions.list"
	SessionsGet      Name = "sessions.get"
	SessionsSnapshot Name = "sessions.snapshot"
	SessionsCreate   Name = "sessions.create"
	SessionsUpdate   Name = "sessions.update"
	SessionsDelete   Name = "sessions.delete"
	SessionsFork     Name = "sessions.fork"
	SessionsRollback Name = "sessions.rollback"
	SessionsExport   Name = "sessions.export"
	SessionsImport   Name = "sessions.import"
)

func registerSessions(registry *Registry) {
	registry.Query(MethodMeta{Name: SessionsList},
		(*Handler).ListSessions)

	registry.Query(MethodMeta{
		Name:   SessionsGet,
		Errors: []string{protocol.ErrSessionNotFound.Error()},
	}, func(service *Handler, ctx context.Context, request protocol.GetSessionRequest) (*protocol.Session, error) {
		return service.GetSession(ctx, request.SessionID)
	})

	registry.Query(MethodMeta{
		Name:         SessionsSnapshot,
		Errors:       []string{protocol.ErrSessionNotFound.Error()},
		Materializes: []Name{ItemsList, RunsList, InterruptsList, PlanGet, GoalsGet},
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "includeDescendants", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureSubagents},
		}},
	}, (*Handler).GetSessionSnapshot)

	registry.Command(MethodMeta{
		Name:   SessionsCreate,
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, (*Handler).CreateSession)

	// Setting workspace is a relocate, which is its own capability — hence a
	// conditional rule: the rest of sessions.update stays available when relocate
	// is off, instead of the whole method disappearing.
	registry.Command(MethodMeta{
		Name: SessionsUpdate,
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRevisionConflict.Error(),
			protocol.ErrWorkspaceUnavailable.Error(),
		},
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "workspace", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureRelocate},
		}},
	}, (*Handler).UpdateSession)

	registry.CommandAck(MethodMeta{
		Name:   SessionsDelete,
		Errors: []string{protocol.ErrSessionNotFound.Error()},
	}, func(service *Handler, ctx context.Context, request protocol.DeleteSessionRequest) error {
		return service.DeleteSession(ctx, request.SessionID)
	})

	registry.Command(MethodMeta{
		Name: SessionsFork,
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRunNotFound.Error(),
		},
	}, (*Handler).ForkSession)

	// restoreType files/both rewind the working tree from a shadow-git snapshot,
	// which needs features.checkpoints; the default history rollback needs nothing
	// Two rules rather than one because the contract states the
	// requirement per value, and a generated schema reads them as two if/then.
	registry.Command(MethodMeta{
		Name: SessionsRollback,
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRunNotFound.Error(),
			protocol.ErrSessionBusy.Error(),
			protocol.ErrCheckpointUnavailable.Error(),
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
	}, (*Handler).RollbackSession)

	registry.Query(MethodMeta{
		Name:            SessionsExport,
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureSessionExport),
	}, (*Handler).ExportSession)

	registry.Command(MethodMeta{
		Name:            SessionsImport,
		CapabilityRules: requires(protocol.FeatureSessionExport),
	}, (*Handler).ImportSession)
}

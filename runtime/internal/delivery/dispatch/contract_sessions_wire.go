package dispatch

import (
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

func registerSessionConditions(s *Shapes) {
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.Session](),
		Rules:  []ConditionalRule{{Required: []string{"createdAt", "updatedAt"}}},
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.PlanState](),
		Rules:  []ConditionalRule{{Required: []string{"updatedAt"}}},
	})

	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.UpdateSessionRequest](), Rules: modelPairingRules()})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RollbackSessionRequest](),
		Rules: []ConditionalRule{{
			When:     whenEquals("restoreType", string(protocol.RestoreFiles)),
			Required: []string{"toRunId"},
		}, {
			When:     whenEquals("restoreType", string(protocol.RestoreBoth)),
			Required: []string{"toRunId"},
		}},
	})
}

func registerSessionValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Session](),
		Constraints: append(append(append(append(requiredResourceIdentity("id"),
			[]FieldConstraint{
				{Field: "provider", Kind: ConstraintNonEmpty},
				{Field: "model", Kind: ConstraintNonEmpty},
			}...), modelSelectionIdentities()...), exactRevision()...), resourceIdentity("projectId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ListSessionsRequest](),
		Constraints: append([]FieldConstraint{{
			Field: "search", Kind: ConstraintMaxLength, Limit: session.MaximumCatalogSearchCharacters,
		}}, resourceIdentity("projectId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.CreateSessionRequest](), Constraints: resourceIdentity("projectId"),
	})
	for _, request := range []reflect.Type{
		typeOf[protocol.GetSessionRequest](),
		typeOf[protocol.GetSessionSnapshotRequest](),
		typeOf[protocol.ListSessionTrajectoryRequest](),
		typeOf[protocol.ExportTrajectoryRequest](),
		typeOf[protocol.DeleteSessionRequest](),
		typeOf[protocol.ExportSessionRequest](),
	} {
		s.valueConstraint(FieldConstraintSpec{GoType: request, Constraints: requiredResourceIdentity("sessionId")})
	}
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.ForkSessionRequest](),
		Constraints: append(requiredResourceIdentity("sessionId"), resourceIdentity("fromRunId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.RollbackSessionRequest](),
		Constraints: append(requiredResourceIdentity("sessionId"), resourceIdentity("toRunId")...),
	})

	constraints := append(requiredResourceIdentity("sessionId"), []FieldConstraint{
		{Field: "expectedRevision", Kind: ConstraintPositive},
		{Field: "expectedRevision", Kind: ConstraintMaximum, Limit: protocol.MaximumExactJSONInteger},
		{Field: "provider", Kind: ConstraintNonEmpty},
		{Field: "model", Kind: ConstraintNonEmpty},
	}...)
	constraints = append(constraints, modelSelectionIdentities()...)
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.UpdateSessionRequest](), Constraints: constraints})
}

func registerPlanValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.GetPlanRequest](), Constraints: requiredResourceIdentity("sessionId")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.Plan](), Constraints: requiredResourceIdentity("sessionId")})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.PlanState](),
		Constraints: exactRevision(),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.PlanStep](),
		Constraints: []FieldConstraint{
			{Field: "id", Kind: ConstraintNonEmpty},
			{Field: "description", Kind: ConstraintPattern, Value: `\S`},
		},
	})
}

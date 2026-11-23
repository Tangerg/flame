package dispatch

import (
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

var shapes = buildShapes()

func buildShapes() *Shapes {
	s := &Shapes{}
	registerNotifications(s)
	registerProblemUnion(s)
	registerRunUnions(s)
	registerTrajectoryUnion(s)
	registerTrajectoryObservationConstraints(s)
	registerItemUnions(s)
	registerProviderUnions(s)
	registerMCPUnions(s)
	registerPluginUnions(s)
	registerToolRefUnion(s)
	registerApprovalSubjectUnion(s)
	registerInterruptUnions(s)
	registerEventUnions(s)
	registerArtifactUnions(s)
	registerDiffUnions(s)
	registerObjectConstraints(s)
	registerCarriedShapes(s)
	registerValueConstraints(s)
	return s
}

func registerProblemUnion(s *Shapes) {
	contracts := ProblemContracts()
	variants := make([]VariantSpec, 0, len(contracts))
	for _, contract := range contracts {
		variants = append(variants, VariantSpec{
			Tag:      contract.Type,
			Required: contract.Required,
			Optional: contract.Optional,
		})
	}
	s.union(UnionSpec{
		GoType: typeOf[protocol.ProblemData](), Variants: variants,
		PatternVariant: &PatternVariantSpec{
			TagPattern:     `^plugin:[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._-]*$`,
			TypeScriptType: "`plugin:${string}/${string}`",
			Optional:       []string{"detail", "docUrl", "retryAfterSeconds"},
		},
	})
}

func registerNotifications(s *Shapes) {
	s.notification(NotificationSpec{
		Name:       NotificationPluginStream,
		ParamsType: typeOf[protocol.PluginStreamEvent](),
	})
	s.notification(NotificationSpec{
		Name:       NotificationRunEvent,
		ParamsType: typeOf[protocol.RunEvent](),
	})
	s.notification(NotificationSpec{
		Name:       NotificationRuntimeEvent,
		ParamsType: typeOf[protocol.RuntimeEventNotification](),
	})
}

func typeOf[T any]() reflect.Type { return reflect.TypeFor[T]() }

func allowedItemStatuses(statuses ...protocol.ItemStatus) []AllowedValueSet {
	values := make([]string, len(statuses))
	for index, status := range statuses {
		values[index] = string(status)
	}
	return []AllowedValueSet{{Field: "status", Values: values}}
}

func allowedStreamItemStatuses(statuses ...protocol.ItemStatus) []AllowedValueSet {
	sets := allowedItemStatuses(statuses...)
	sets[0].Field = "item.status"
	return sets
}

func allowedStreamItemKinds(kinds ...protocol.ItemType) []AllowedValueSet {
	values := make([]string, len(kinds))
	for index, kind := range kinds {
		values[index] = string(kind)
	}
	return []AllowedValueSet{{Field: "item.type", Values: values}}
}

func allowedArtifactProblemTypes(kinds ...protocol.ArtifactProblemType) []AllowedValueSet {
	values := make([]string, len(kinds))
	for index, kind := range kinds {
		values[index] = string(kind)
	}
	return []AllowedValueSet{{Field: "error.type", Values: values}}
}

func allowedMCPStatusProblems(kinds ...protocol.MCPStatusProblemType) []AllowedValueSet {
	values := make([]string, len(kinds))
	for index, kind := range kinds {
		values[index] = string(kind)
	}
	return []AllowedValueSet{{Field: "error.type", Values: values}}
}

func allowedGoalReasonCodes(codes ...protocol.GoalReasonCode) []AllowedValueSet {
	values := make([]string, len(codes))
	for index, code := range codes {
		values[index] = string(code)
	}
	return []AllowedValueSet{{Field: "reason.code", Values: values}}
}

func registerObjectConstraints(s *Shapes) {
	registerRequiredAnyConstraints(s)
	registerRunConditions(s)
	registerSessionConditions(s)
	registerHookConditions(s)
	registerPluginTerminalConditions(s)
	registerApprovalConditions(s)
	registerWorkspaceConditions(s)
	registerGoalConditions(s)
	registerItemConditions(s)
	registerMCPConditions(s)
	registerArtifactConditions(s)
}

func registerRequiredAnyConstraints(s *Shapes) {
	for _, constrained := range []struct {
		goType      reflect.Type
		requiredAny []string
	}{
		{goType: typeOf[protocol.ModelTokenLimits](), requiredAny: []string{"contextWindow", "maxInputTokens", "maxOutputTokens"}},
		{goType: typeOf[protocol.FeedbackRequest](), requiredAny: []string{"rating", "text"}},
	} {
		s.constraint(ObjectConstraintSpec{
			GoType: constrained.goType,
			Rules:  []ConditionalRule{{RequiredAny: constrained.requiredAny}},
		})
	}
}

func registerCarriedShapes(s *Shapes) {
	s.carriedShape(CarriedSpec{Carrier: "params." + protocol.RequestMetaMember, GoType: typeOf[protocol.RequestMeta]()})
}

func registerValueConstraints(s *Shapes) {
	registerCollectionValues(s)
	registerSessionValues(s)
	registerArtifactValues(s)
	registerRunValues(s)
	registerTrajectoryValues(s)
	registerPlanValues(s)
	registerWorkspaceValues(s)
	registerUsageValues(s)
	registerFeedbackValues(s)
	registerSkillValues(s)
	registerHookValues(s)
	registerApprovalValues(s)
	registerMCPValues(s)
	registerPluginValues(s)
	registerProviderValues(s)
	registerModelValues(s)
	registerToolValues(s)
	registerAuthoringContextValues(s)
	registerGoalValues(s)
	registerRuntimeValues(s)
}

func registerCollectionValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.PageContinuation](),
		Constraints: []FieldConstraint{{
			Field: "nextCursor", Kind: ConstraintMaxLength, Limit: protocol.MaximumPaginationCursorCharacters,
		}},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.PageQuery](),
		Constraints: []FieldConstraint{
			{Field: "limit", Kind: ConstraintPositive},
			{Field: "cursor", Kind: ConstraintMaxLength, Limit: protocol.MaximumPaginationCursorCharacters},
		},
	})
}

func nonEmpty[Request any](s *Shapes, fields ...string) {
	constraints := make([]FieldConstraint, 0, len(fields))
	for _, field := range fields {
		constraints = append(constraints, FieldConstraint{Field: field, Kind: ConstraintNonEmpty})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[Request](), Constraints: constraints})
}

func nonNegative[Shape any](s *Shapes, fields ...string) {
	constraints := make([]FieldConstraint, 0, len(fields))
	for _, field := range fields {
		constraints = append(constraints, FieldConstraint{Field: field, Kind: ConstraintNonNegative})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[Shape](), Constraints: constraints})
}

func exactRevision() []FieldConstraint {
	return []FieldConstraint{
		{Field: "revision", Kind: ConstraintPositive},
		{Field: "revision", Kind: ConstraintMaximum, Limit: protocol.MaximumExactJSONInteger},
	}
}

func boundedIdentity(field string, maximumCharacters int64) []FieldConstraint {
	return []FieldConstraint{
		{Field: field, Kind: ConstraintIdentity},
		{Field: field, Kind: ConstraintMaxLength, Limit: maximumCharacters},
	}
}

func requiredResourceIdentity(field string) []FieldConstraint {
	return append([]FieldConstraint{{Field: field, Kind: ConstraintNonEmpty}}, resourceIdentity(field)...)
}

func resourceIdentity(field string) []FieldConstraint {
	return boundedIdentity(field, protocol.MaximumResourceIdentityCharacters)
}

func runEventIdentity(field string) []FieldConstraint {
	return []FieldConstraint{
		{Field: field, Kind: ConstraintIdentity},
		{Field: field, Kind: ConstraintPrefix, Value: protocol.IDPrefixEvent},
		{Field: field, Kind: ConstraintMaxLength, Limit: int64(protocol.MaximumRunEventIDCharacters)},
	}
}

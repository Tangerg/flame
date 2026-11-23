package dispatch

import (
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func registerProviderUnions(s *Shapes) {
	s.union(UnionSpec{
		GoType: typeOf[protocol.ProviderConfigChange](),
		Variants: []VariantSpec{
			{Tag: string(protocol.ProviderConfigSet), Required: []string{"value"}},
			{Tag: string(protocol.ProviderConfigClear)},
		},
	})
}

func modelPairingRules() []ConditionalRule {
	return []ConditionalRule{
		{When: whenPresent("provider"), Required: []string{"model"}},
		{When: whenPresent("model"), Required: []string{"provider"}},
	}
}

func modelSelectionRules() []ConditionalRule {
	return append(modelPairingRules(), ConditionalRule{When: whenPresent("reasoningEffort"), Required: []string{"provider", "model"}})
}

func modelSelectionIdentities() []FieldConstraint {
	constraints := boundedIdentity("provider", model.MaximumProviderIdentityCharacters)
	constraints = append(constraints, boundedIdentity("model", model.MaximumModelIdentityCharacters)...)
	constraints = append(constraints,
		boundedIdentity("reasoningEffort", model.MaximumReasoningEffortCharacters)...)
	return constraints
}

func modelUsageMapIdentities(field string) []FieldConstraint {
	return []FieldConstraint{
		{Field: field, Kind: ConstraintIdentityPropertyNames},
		{Field: field, Kind: ConstraintMaxPropertyNameLength, Limit: model.MaximumModelIdentityCharacters},
	}
}

func registerUsageValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UsageBucket](),
		Constraints: []FieldConstraint{
			{Field: "key", Kind: ConstraintNonEmpty},
		},
	})
	nonNegative[protocol.UsageSummary](s, "sessions", "runs")
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SessionUsageRequest](), Constraints: requiredResourceIdentity("sessionId")})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.UsageSummaryRequest](),
		Constraints: []FieldConstraint{{Field: "sinceDays", Kind: ConstraintPositive}},
	})
}

func registerProviderValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Provider](),
		Constraints: append(append([]FieldConstraint{{Field: "id", Kind: ConstraintNonEmpty}},
			boundedIdentity("id", model.MaximumProviderIdentityCharacters)...),
			boundedIdentity("defaultEmbeddingModel", model.MaximumModelIdentityCharacters)...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateProviderRequest](),
		Constraints: append([]FieldConstraint{{Field: "provider", Kind: ConstraintNonEmpty}},
			boundedIdentity("provider", model.MaximumProviderIdentityCharacters)...),
	})
	nonEmpty[protocol.ProviderConfigChange](s, "value")
	nonEmpty[protocol.ProviderCredential](s, "masked")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.TestProviderRequest](),
		Constraints: append([]FieldConstraint{{Field: "provider", Kind: ConstraintNonEmpty}},
			boundedIdentity("provider", model.MaximumProviderIdentityCharacters)...),
	})
}

func registerModelValues(s *Shapes) {
	nonNegative[protocol.ModelPriceBand](s,
		"aboveInputTokens", "inputUsdPerMillionTokens", "outputUsdPerMillionTokens",
		"cacheReadUsdPerMillionTokens", "cacheWriteUsdPerMillionTokens",
	)
	for _, roleType := range []reflect.Type{
		typeOf[protocol.UtilityRole](),
		typeOf[protocol.EmbeddingRole](),
	} {
		s.valueConstraint(FieldConstraintSpec{
			GoType: roleType,
			Constraints: append(
				boundedIdentity("provider", model.MaximumProviderIdentityCharacters),
				boundedIdentity("model", model.MaximumModelIdentityCharacters)...,
			),
		})
	}
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.ListModelsRequest](),
		Constraints: boundedIdentity("provider", model.MaximumProviderIdentityCharacters),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Model](),
		Constraints: append(append([]FieldConstraint{
			{Field: "id", Kind: ConstraintNonEmpty},
			{Field: "provider", Kind: ConstraintNonEmpty},
		}, boundedIdentity("id", model.MaximumModelIdentityCharacters)...),
			boundedIdentity("provider", model.MaximumProviderIdentityCharacters)...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ModelCapabilities](),
		Constraints: append([]FieldConstraint{
			{Field: "reasoningLevels", Kind: ConstraintIdentityItems},
			{Field: "reasoningLevels", Kind: ConstraintMaxItemLength, Limit: model.MaximumReasoningEffortCharacters},
		}, boundedIdentity("reasoningDefaultLevel", model.MaximumReasoningEffortCharacters)...),
	})
}

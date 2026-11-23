package dispatch

import (
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
)

func registerRuntimeValues(s *Shapes) {
	nonEmpty[protocol.ClientInfo](s, "name", "version")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ServerInfo](),
		Constraints: []FieldConstraint{
			{Field: "name", Kind: ConstraintPattern, Value: `\S`},
			{Field: "version", Kind: ConstraintPattern, Value: `\S`},
			{Field: "home", Kind: ConstraintPattern, Value: `\S`},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ServerCapabilities](),
		Constraints: []FieldConstraint{
			{Field: "runEvents", Kind: ConstraintUniqueItems},
			{Field: "runtimeTopics", Kind: ConstraintUniqueItems},
			{Field: "streamingMethods", Kind: ConstraintUniqueItems},
			{Field: "streamingMethods", Kind: ConstraintPatternItems, Value: `\S`},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.RuntimeLimits](),
		Constraints: []FieldConstraint{{Field: "maxConcurrentRuns", Kind: ConstraintPositive}},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ClientCapabilities](),
		Constraints: []FieldConstraint{
			{Field: "interruptTypes", Kind: ConstraintUniqueItems},
			{Field: "excludedEphemeralEvents", Kind: ConstraintUniqueItems},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPAuthorizationAttemptLimits](),
		Constraints: []FieldConstraint{
			{Field: "retentionSeconds", Kind: ConstraintPositive},
			{Field: "retentionSeconds", Kind: ConstraintMaximum, Limit: protocol.MaximumDurationSeconds},
		},
	})

	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.RuntimeSubscribeRequest](),
		Constraints: []FieldConstraint{
			{Field: "topics", Kind: ConstraintNonEmptyItems},
			{Field: "topics", Kind: ConstraintUniqueItems},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.WatchSpec](),
		Constraints: []FieldConstraint{
			{Field: "paths", Kind: ConstraintNonEmptyItems},
			{Field: "paths", Kind: ConstraintUniqueItems},
		},
	})

	eventConstraints := []FieldConstraint{
		{Field: "sequence", Kind: ConstraintPositive},
		{Field: "sequence", Kind: ConstraintMaximum, Limit: protocol.MaximumRuntimeEventSequence},
	}
	for _, field := range []string{
		"paths", "names", "servers", "sessionIds", "runIds",
		"topics", "watchIds",
	} {
		eventConstraints = append(eventConstraints,
			FieldConstraint{Field: field, Kind: ConstraintNonEmptyItems},
			FieldConstraint{Field: field, Kind: ConstraintUniqueItems},
		)
	}
	eventConstraints = append(eventConstraints,
		FieldConstraint{Field: "sessionIds", Kind: ConstraintIdentityItems},
		FieldConstraint{Field: "sessionIds", Kind: ConstraintMaxItemLength, Limit: protocol.MaximumResourceIdentityCharacters},
		FieldConstraint{Field: "runIds", Kind: ConstraintIdentityItems},
		FieldConstraint{Field: "runIds", Kind: ConstraintMaxItemLength, Limit: protocol.MaximumResourceIdentityCharacters},
	)
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.RuntimeEvent](),
		Constraints: eventConstraints,
	})

	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.PendingInterruptSet](),
		Constraints: append(append(requiredResourceIdentity("rootRunId"), requiredResourceIdentity("sessionId")...),
			[]FieldConstraint{
				{Field: "interrupts", Kind: ConstraintNonEmptyItems},
			}...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.Interrupt](),
		Constraints: append(requiredResourceIdentity("itemId"), requiredResourceIdentity("runId")...),
	})
	// Leaf types are validation roots too, so ValidateWireTree applies their
	// constraints when they are nested in ProblemData.
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ActiveRunRef](), Constraints: requiredResourceIdentity("runId")})
	nonEmpty[protocol.CapabilityRequirement](s, "name")
	nonEmpty[protocol.FieldError](s, "field", "detail")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ProblemData](),
		Constraints: []FieldConstraint{
			{Field: "retryAfterSeconds", Kind: ConstraintPositive},
			{Field: "retryAfterSeconds", Kind: ConstraintMaximum, Limit: protocol.MaximumDurationSeconds},
			{Field: "requiredCapabilities", Kind: ConstraintNonEmptyItems},
			{Field: "requiredCapabilities", Kind: ConstraintUniqueItems},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.RunReplayLimits](),
		Constraints: []FieldConstraint{
			{Field: "maxEvents", Kind: ConstraintPositive},
			{Field: "maxBytes", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.IdempotencyLimits](),
		Constraints: []FieldConstraint{
			{Field: "retentionSeconds", Kind: ConstraintPositive},
			{Field: "retentionSeconds", Kind: ConstraintMaximum, Limit: protocol.MaximumDurationSeconds},
			{Field: "namespace", Kind: ConstraintPattern, Value: operation.ReplayNamespacePattern()},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SubscriptionLimits](),
		Constraints: []FieldConstraint{
			{Field: "maxTopics", Kind: ConstraintPositive},
			{Field: "maxWatches", Kind: ConstraintPositive},
			{Field: "maxPaths", Kind: ConstraintPositive},
			{Field: "maxDirectoryEntries", Kind: ConstraintPositive},
			{Field: "maxFileBytes", Kind: ConstraintPositive},
		},
	})
}

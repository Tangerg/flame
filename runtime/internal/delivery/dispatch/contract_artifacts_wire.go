package dispatch

import (
	"reflect"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func registerArtifactUnions(s *Shapes) {
	s.union(UnionSpec{
		GoType: typeOf[protocol.ArtifactOutcome](),
		Variants: []VariantSpec{
			{Tag: string(protocol.ArtifactOutcomeCompleted), Optional: []string{"unresolvedEffects"}},
			{
				Tag: string(protocol.ArtifactOutcomeTimedOut), Required: []string{"error"}, Optional: []string{"unresolvedEffects"},
				AllowedValues: allowedArtifactProblemTypes(protocol.ArtifactProblemTimeout),
			}, {
				Tag: string(protocol.ArtifactOutcomeFailed), Required: []string{"error"}, Optional: []string{"unresolvedEffects"},
				AllowedValues: allowedArtifactProblemTypes(protocol.ArtifactProblemInternalError, protocol.ArtifactProblemAgentStuck, protocol.ArtifactProblemRateLimited, protocol.ArtifactProblemInvalidCredentials, protocol.ArtifactProblemTimeout, protocol.ArtifactProblemProviderUnavailable, protocol.ArtifactProblemProviderRejected),
			},
			{Tag: string(protocol.ArtifactOutcomeCanceled), Optional: []string{"detail", "unresolvedEffects"}},
			{Tag: string(protocol.ArtifactOutcomeLost), Required: []string{"loss"}, Optional: []string{"unresolvedEffects"}},
		},
	})

	itemIdentityFields := []string{"id", "runId", "status"}
	createdItemFields := slices.Concat(itemIdentityFields, []string{"createdAt"})
	toolItemFields := slices.Concat(itemIdentityFields, []string{"startedAt"})
	s.union(UnionSpec{
		GoType: typeOf[protocol.ArtifactItem](),
		Variants: []VariantSpec{
			{
				Tag: string(protocol.ItemTypeUserMessage), Required: slices.Concat(createdItemFields, []string{"content"}),
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			}, {
				Tag: string(protocol.ItemTypeAgentMessage), Required: slices.Concat(createdItemFields, []string{"phase", "content"}),
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted, protocol.ItemStatusIncomplete),
			}, {
				Tag: string(protocol.ItemTypeReasoning), Required: slices.Concat(createdItemFields, []string{"text"}), Optional: []string{"redacted"},
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted, protocol.ItemStatusIncomplete),
			}, {
				Tag: string(protocol.ItemTypeQuestion), Required: slices.Concat(createdItemFields, []string{"question"}),
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			}, {
				Tag: string(protocol.ItemTypeToolCall), Required: slices.Concat(toolItemFields, []string{"tool"}), Optional: []string{"finishedAt", "durationMillis", "safetyClass", "approvalDecision", "error"},
				AllowedValues: slices.Concat(
					allowedItemStatuses(protocol.ItemStatusCompleted, protocol.ItemStatusIncomplete),
					allowedArtifactProblemTypes(protocol.ArtifactProblemInternalError, protocol.ArtifactProblemDeniedByUser, protocol.ArtifactProblemToolFailed, protocol.ArtifactProblemChildRunCanceled, protocol.ArtifactProblemToolCanceled),
				),
			}, {
				Tag: string(protocol.ItemTypeCompaction), Required: slices.Concat(createdItemFields, []string{"summary"}), Optional: []string{"droppedMessages"},
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			},
		},
	})
}

func registerArtifactConditions(s *Shapes) {
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactRun](),
		Rules: append([]ConditionalRule{
			{
				Required: []string{"createdAt", "finishedAt", "updatedAt"},
			},
			{
				When:      whenPresent("spawnedByItemId"),
				Forbidden: []string{"capabilities", "messageMark"},
			},
		}, childLineageRules()...),
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactSession](),
		Rules:  []ConditionalRule{{Required: []string{"createdAt", "updatedAt"}}},
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactToolResult](),
		Rules:  []ConditionalRule{{Required: []string{"createdAt"}}},
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactProblem](),
		Rules:  artifactProblemRetryRules(),
	})
}

func artifactProblemRetryRules() []ConditionalRule {
	withoutRetry := []protocol.ArtifactProblemType{protocol.ArtifactProblemInternalError, protocol.ArtifactProblemAgentStuck, protocol.ArtifactProblemInvalidCredentials, protocol.ArtifactProblemProviderRejected, protocol.ArtifactProblemDeniedByUser, protocol.ArtifactProblemToolFailed, protocol.ArtifactProblemChildRunCanceled, protocol.ArtifactProblemToolCanceled}
	rules := make([]ConditionalRule, 0, len(withoutRetry))
	for _, kind := range withoutRetry {
		rules = append(rules, ConditionalRule{
			When:      whenEquals("type", string(kind)),
			Forbidden: []string{"retryAfterSeconds"},
		})
	}
	return rules
}

func registerTrajectoryUnion(s *Shapes) {
	s.union(UnionSpec{
		GoType: typeOf[protocol.TrajectoryEntry](),
		Variants: []VariantSpec{
			{Tag: string(protocol.TrajectoryEntryRun), Required: []string{"occurredAt", "run"}},
			{Tag: string(protocol.TrajectoryEntryModel), Required: []string{"occurredAt", "model"}},
			{Tag: string(protocol.TrajectoryEntryEndpoint), Required: []string{"occurredAt", "endpoint"}},
			{Tag: string(protocol.TrajectoryEntryItem), Required: []string{"occurredAt", "item"}},
		},
	})
}

func registerTrajectoryObservationConstraints(s *Shapes) {
	endpointRules := []ConditionalRule{
		{When: whenPresent("runId"), Required: []string{"sessionId"}},
	}
	for _, evidence := range modelref.EndpointEvidenceRules() {
		rule := ConditionalRule{When: whenEquals("state", string(evidence.State))}
		for _, field := range []struct {
			name    string
			present bool
		}{
			{"settledAt", evidence.Settled}, {"response", evidence.Response}, {"failure", evidence.Failure},
		} {
			if field.present {
				rule.Required = append(rule.Required, field.name)
			} else {
				rule.Forbidden = append(rule.Forbidden, field.name)
			}
		}
		if !evidence.Response {
			rule.Forbidden = append(rule.Forbidden, "costUsd")
		}
		endpointRules = append(endpointRules, rule)
	}
	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.ModelEndpointInvocation](), Rules: endpointRules})
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ModelInvocation](),
		Rules: []ConditionalRule{
			{When: []delivery.FieldCondition{{Field: "state", Operator: delivery.OperatorEquals, Value: string(protocol.ModelInvocationFailed)}, {Field: "usage", Operator: delivery.OperatorPresent}}, Required: []string{"knownResponse"}},
			{When: whenEquals("state", string(protocol.ModelInvocationStarted)), Forbidden: []string{"settledAt", "usage", "firstOutputLatencyMillis", "knownResponse"}},
			{When: whenEquals("state", string(protocol.ModelInvocationCompleted)), Required: []string{"settledAt"}, Forbidden: []string{"knownResponse"}},
			{When: whenEquals("state", string(protocol.ModelInvocationFailed)), Required: []string{"settledAt"}},
			{When: whenEquals("state", string(protocol.ModelInvocationUnknown)), Required: []string{"settledAt"}, Forbidden: []string{"usage", "firstOutputLatencyMillis", "knownResponse"}},
		},
	})
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ToolAttempt](),
		Rules: []ConditionalRule{
			{When: whenEquals("state", string(protocol.ToolAttemptStarted)), Forbidden: []string{"settledAt"}},
			{When: whenEquals("state", string(protocol.ToolAttemptCompleted)), Required: []string{"settledAt"}},
			{When: whenEquals("state", string(protocol.ToolAttemptIncomplete)), Required: []string{"settledAt"}},
		},
	})
}

func registerArtifactValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SessionTrajectory](),
		Constraints: []FieldConstraint{
			{Field: "schemaVersion", Kind: ConstraintMinimum, Limit: protocol.SessionTrajectoryVersion},
			{Field: "schemaVersion", Kind: ConstraintMaximum, Limit: protocol.SessionTrajectoryVersion},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ArtifactSession](),
		Constraints: append(append(requiredResourceIdentity("id"), []FieldConstraint{
			{Field: "provider", Kind: ConstraintNonEmpty},
			{Field: "model", Kind: ConstraintNonEmpty},
		}...), modelSelectionIdentities()...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ArtifactRun](),
		Constraints: append(append(append(append(append(append(requiredResourceIdentity("id"),
			requiredResourceIdentity("sessionId")...), resourceIdentity("spawnedByItemId")...),
			resourceIdentity("parentRunId")...), resourceIdentity("rootRunId")...),
			[]FieldConstraint{
				{Field: "provider", Kind: ConstraintNonEmpty},
				{Field: "model", Kind: ConstraintNonEmpty},
			}...),
			append(modelSelectionIdentities(),
				FieldConstraint{Field: "contextTokens", Kind: ConstraintNonNegative},
			)...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ArtifactItem](),
		Constraints: append(append(requiredResourceIdentity("id"), requiredResourceIdentity("runId")...),
			FieldConstraint{Field: "droppedMessages", Kind: ConstraintNonNegative},
			FieldConstraint{Field: "durationMillis", Kind: ConstraintNonNegative},
			FieldConstraint{Field: "durationMillis", Kind: ConstraintMaximum, Limit: protocol.MaximumDurationMilliseconds},
		),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ArtifactToolResult](),
		Constraints: append([]FieldConstraint{
			{Field: "id", Kind: ConstraintPattern, Value: transcript.ToolResultIDPattern},
			{Field: "body", Kind: ConstraintNonEmpty},
		}, requiredResourceIdentity("itemId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SessionArtifact](),
		Constraints: []FieldConstraint{
			{Field: "version", Kind: ConstraintMinimum, Limit: protocol.SessionArtifactVersion},
			{Field: "version", Kind: ConstraintMaximum, Limit: protocol.SessionArtifactVersion},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ArtifactProblem](),
		Constraints: []FieldConstraint{
			{Field: "retryAfterSeconds", Kind: ConstraintPositive},
			{Field: "retryAfterSeconds", Kind: ConstraintMaximum, Limit: protocol.MaximumDurationSeconds},
		},
	})
}

func registerTrajectoryValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.UsageCoverage](), Constraints: []FieldConstraint{
		{Field: "calls", Kind: ConstraintNonNegative}, {Field: "reportedCalls", Kind: ConstraintNonNegative}, {Field: "pricedCalls", Kind: ConstraintNonNegative}, {Field: "startedCalls", Kind: ConstraintNonNegative}, {Field: "completedCalls", Kind: ConstraintNonNegative}, {Field: "failedCalls", Kind: ConstraintNonNegative}, {Field: "unknownCalls", Kind: ConstraintNonNegative},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.RecordedUsage](), Constraints: []FieldConstraint{{Field: "recordedCostUsd", Kind: ConstraintNonNegative}}})

	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ListModelInvocationsRequest](), Constraints: requiredResourceIdentity("runId")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ListModelEndpointsRequest](), Constraints: []FieldConstraint{
		{Field: "parentCallId", Kind: ConstraintNonEmpty}, {Field: "parentCallId", Kind: ConstraintIdentity}, {Field: "parentCallId", Kind: ConstraintMaxLength, Limit: modelref.MaximumInvocationIDCharacters},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ModelEndpointInvocation](), Constraints: append(append(resourceIdentity("sessionId"), resourceIdentity("runId")...), []FieldConstraint{
		{Field: "costUsd", Kind: ConstraintNonNegative},
		{Field: "id", Kind: ConstraintIdentity}, {Field: "id", Kind: ConstraintNonEmpty}, {Field: "provider", Kind: ConstraintNonEmpty}, {Field: "model", Kind: ConstraintNonEmpty}, {Field: "request", Kind: ConstraintNonEmpty},
	}...)})
	for _, owner := range []reflect.Type{typeOf[protocol.ModelInvocation](), typeOf[protocol.ToolAttempt]()} {
		constraints := append(requiredResourceIdentity("runId"), requiredResourceIdentity("segmentId")...)
		constraints = append(constraints,
			FieldConstraint{Field: "callId", Kind: ConstraintPattern, Value: identity.URISafeASCIIPattern},
			FieldConstraint{Field: "callId", Kind: ConstraintMaxLength, Limit: runs.MaximumIdentityBytes},
		)
		if owner == typeOf[protocol.ToolAttempt]() {
			constraints = append(constraints, requiredResourceIdentity("itemId")...)
		}
		if owner == typeOf[protocol.ModelInvocation]() {
			constraints = append(constraints, FieldConstraint{Field: "firstOutputLatencyMillis", Kind: ConstraintNonNegative})
		}
		s.valueConstraint(FieldConstraintSpec{GoType: owner, Constraints: constraints})
	}
	nonNegative[protocol.ModelInvocationUsage](s, "inputTokens", "outputTokens", "cacheReadTokens", "cacheWriteTokens", "reasoningTokens")
}

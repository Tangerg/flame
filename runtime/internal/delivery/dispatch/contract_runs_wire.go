package dispatch

import (
	"reflect"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func registerRunUnions(s *Shapes) {
	s.union(UnionSpec{
		GoType: typeOf[protocol.CancelRunResponse](),
		Variants: []VariantSpec{
			{Tag: string(protocol.CancelRunRoot), Required: []string{"run"}},
			{Tag: string(protocol.CancelRunChild), Required: []string{"run", "rootRun"}},
		},
	})

	s.union(UnionSpec{
		GoType: typeOf[protocol.RunAcceptance](),
		Variants: []VariantSpec{
			{Tag: string(protocol.RunAcceptanceNotRecorded), Optional: []string{"candidateUserItemId"}},
			{Tag: string(protocol.RunAcceptanceAccepted), Required: []string{"runId", "segmentId", "sessionId", "run"}, Optional: []string{"userItemId", "input", "appliedItem"}},
			{Tag: string(protocol.RunAcceptanceRetired), Required: []string{"runId", "segmentId", "sessionId"}, Optional: []string{"userItemId", "input"}},
		},
	})

	s.union(UnionSpec{
		GoType:   typeOf[protocol.RunOutcome](),
		Variants: runOutcomeVariants(),
	})
}

func runOutcomeVariants() []VariantSpec {
	variants := []VariantSpec{
		{Tag: string(protocol.OutcomeCompleted)},
		{Tag: string(protocol.OutcomeTimedOut), Required: []string{"error"}},
		{Tag: string(protocol.OutcomeFailed), Required: []string{"error"}},
		{Tag: string(protocol.OutcomeCanceled), Optional: []string{"detail"}},
		{Tag: string(protocol.OutcomeLost), Required: []string{"loss"}},
	}
	for index := range variants {
		variants[index].Optional = append(variants[index].Optional, "unresolvedEffects")
	}
	return variants
}

func registerInterruptUnions(s *Shapes) {
	identity := []string{"itemId", "runId"}
	s.union(UnionSpec{
		GoType: typeOf[protocol.Interrupt](),
		Variants: []VariantSpec{
			{
				Tag:      string(protocol.InterruptApproval),
				Required: append(slices.Clone(identity), "payload.tool"),
				Optional: []string{"payload.risk", "payload.reason", "payload.rememberable"},
			},
			{Tag: string(protocol.InterruptQuestion), Required: append(slices.Clone(identity), "payload.question")},
		},
	})

	s.union(UnionSpec{
		GoType: typeOf[protocol.InterruptResponseValue](),
		Variants: []VariantSpec{
			{
				Tag:      string(protocol.InterruptResponseApproval),
				Required: []string{"decision"},
				Optional: []string{"remember", "editedArgs", "reason"},
			},
			{Tag: string(protocol.InterruptResponseAnswer), Required: []string{"answers"}},
		},
	})
}

func registerEventUnions(s *Shapes) {
	s.union(UnionSpec{
		GoType:    typeOf[protocol.StreamEvent](),
		Forbidden: []string{"durable"},
		Variants: []VariantSpec{
			{Tag: string(protocol.StreamSegmentStarted), Required: []string{"run"}},
			{Tag: string(protocol.StreamSegmentProgress), Required: []string{"progress"}},
			{Tag: string(protocol.StreamSegmentFinished), Required: []string{"run"}, Optional: []string{"interrupts"}},
			{Tag: string(protocol.StreamItemStarted), Required: []string{"item"}},
			{Tag: string(protocol.StreamItemDelta), Required: []string{"itemId", "delta"}},
			{Tag: string(protocol.StreamItemCompleted), Required: []string{"item"}},
			{Tag: string(protocol.StreamPlanUpdated), Required: []string{"plan"}},
		},
	})

	s.union(UnionSpec{
		GoType: typeOf[protocol.RuntimeEvent](),
		Variants: []VariantSpec{
			{Tag: string(protocol.RuntimeFilesChanged), Required: []string{"sequence", "paths"}, Optional: []string{"watchId", "workspace"}},
			{Tag: string(protocol.RuntimeSkillsChanged), Required: []string{"sequence"}, Optional: []string{"names"}},
			{Tag: string(protocol.RuntimePluginsChanged), Required: []string{"sequence"}},
			{Tag: string(protocol.RuntimeMCPChanged), Required: []string{"sequence"}, Optional: []string{"servers"}},
			{Tag: string(protocol.RuntimePluginDataChanged), Required: []string{"sequence"}},
			{Tag: string(protocol.RuntimeSessionsChanged), Required: []string{"sequence"}, Optional: []string{"sessionIds"}},
			{Tag: string(protocol.RuntimeRunsChanged), Required: []string{"sequence"}, Optional: []string{"runIds", "sessionIds"}},
			{Tag: string(protocol.RuntimePlanChanged), Required: []string{"sequence"}, Optional: []string{"sessionIds"}},
			{Tag: string(protocol.RuntimeGoalsChanged), Required: []string{"sequence"}, Optional: []string{"sessionIds"}},
			{Tag: string(protocol.RuntimeInterruptsChanged), Required: []string{"sequence"}, Optional: []string{"runIds", "sessionIds"}},
			{Tag: string(protocol.RuntimeHooksChanged), Required: []string{"sequence"}},
			{Tag: string(protocol.RuntimeModelsChanged), Required: []string{"sequence"}},
			{Tag: string(protocol.RuntimeApprovalsChanged), Required: []string{"sequence"}},

			{Tag: string(protocol.RuntimeResync), Required: []string{"sequence", "topics"}, Optional: []string{"watchIds"}},
		},
	})
}

func registerRunConditions(s *Shapes) {
	s.union(UnionSpec{
		GoType: typeOf[protocol.Activity](),
		Variants: []VariantSpec{
			{Tag: string(protocol.ActivityCallingModel)},
			{Tag: string(protocol.ActivityModelCallFailed)},
			{Tag: string(protocol.ActivityApplyingUserInput)},
			{Tag: string(protocol.ActivityToolCall), Required: []string{"itemId", "tool"}},
		},
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunProgress](),
		Rules: []ConditionalRule{{
			RequiredAny: []string{"step", "usage", "contextTokens", "activity"},
		}},
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunEvent](),
		Rules:  []ConditionalRule{{Required: []string{"timestamp"}}},
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.StreamEvent](),
		Rules: []ConditionalRule{{
			When: whenEquals("type", string(protocol.StreamItemStarted)),
			AllowedValues: slices.Concat(
				allowedStreamItemKinds(protocol.ItemTypeAgentMessage, protocol.ItemTypeReasoning, protocol.ItemTypeToolCall),
				allowedStreamItemStatuses(protocol.ItemStatusRunning),
			),
		}, {
			When:          whenEquals("type", string(protocol.StreamItemCompleted)),
			AllowedValues: allowedStreamItemStatuses(protocol.ItemStatusCompleted, protocol.ItemStatusIncomplete),
		}, {
			When:     whenEquals("type", string(protocol.StreamPlanUpdated)),
			Required: []string{"plan.state"},
		}},
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.StartRunRequest](),
		Rules: slices.Concat([]ConditionalRule{
			{RequiredAny: []string{"sessionId", "newSession"}},
			{When: whenPresent("newSession"), Forbidden: []string{"sessionId"}},
		}, modelSelectionRules()),
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunSummary](),
		Rules: append([]ConditionalRule{{
			Required: []string{"provider", "model", "createdAt"},
		}, {
			When:     whenEquals("status", string(protocol.RunStatusFinished)),
			Required: []string{"outcome", "finishedAt"},
		}, {
			When:      whenEquals("status", string(protocol.RunStatusRunning)),
			Forbidden: []string{"outcome", "finishedAt"},
		}, {
			When:      whenEquals("status", string(protocol.RunStatusWaiting)),
			Forbidden: []string{"outcome", "finishedAt"},
		}}, childLineageRules()...),
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunRef](),
		Rules: []ConditionalRule{{
			When:     whenEquals("status", string(protocol.RunStatusRunning)),
			Required: []string{"activeSegmentId"},
		}, {
			When:      whenEquals("status", string(protocol.RunStatusWaiting)),
			Forbidden: []string{"activeSegmentId"},
		}, {
			When:      whenEquals("status", string(protocol.RunStatusFinished)),
			Forbidden: []string{"activeSegmentId"},
		}, {
			When:      whenPresent("spawnedByItemId"),
			Forbidden: []string{"capabilities"},
		}},
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.PendingInterruptSet](),
		Rules: []ConditionalRule{{
			Required: []string{"rootRunId", "sessionId", "interrupts", "createdAt"},
		}},
	})
}

func childLineageRules() []ConditionalRule {
	edges := []string{"spawnedByItemId", "parentRunId", "rootRunId"}
	rules := make([]ConditionalRule, 0, len(edges))
	for index, edge := range edges {
		others := append(append([]string{}, edges[:index]...), edges[index+1:]...)
		rules = append(rules, ConditionalRule{
			When:     whenPresent(edge),
			Required: others,
		})
	}
	return rules
}

func registerRunValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ContentBlock](),
		Constraints: []FieldConstraint{
			{Field: "text", Kind: ConstraintPattern, Value: `\S`},
			{Field: "mime", Kind: ConstraintPattern, Value: `^image/`},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.ToolInvocation](),
		Constraints: []FieldConstraint{{Field: "name", Kind: ConstraintPattern, Value: `\S`}},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.RunSummary](),
		Constraints: append(append(append(append(append(append(requiredResourceIdentity("id"),
			requiredResourceIdentity("sessionId")...), resourceIdentity("spawnedByItemId")...),
			resourceIdentity("parentRunId")...), resourceIdentity("rootRunId")...),
			[]FieldConstraint{
				{Field: "provider", Kind: ConstraintNonEmpty},
				{Field: "model", Kind: ConstraintNonEmpty},
			}...),
			modelSelectionIdentities()...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.RunEvent](),
		Constraints: append(append(requiredResourceIdentity("runId"), requiredResourceIdentity("segmentId")...),
			runEventIdentity("eventId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SubscribeRunResponse](),
		Constraints: append(append(requiredResourceIdentity("runId"), requiredResourceIdentity("segmentId")...),
			runEventIdentity("headEventId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Item](),
		Constraints: append(append(requiredResourceIdentity("id"), requiredResourceIdentity("runId")...),
			FieldConstraint{Field: "durationMillis", Kind: ConstraintNonNegative},
			FieldConstraint{Field: "durationMillis", Kind: ConstraintMaximum, Limit: protocol.MaximumDurationMilliseconds},
			FieldConstraint{Field: "droppedMessages", Kind: ConstraintNonNegative},
		),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.InterruptResponse](),
		Constraints: requiredResourceIdentity("itemId"),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.RunRef](),
		Constraints: append(resourceIdentity("activeSegmentId"),
			FieldConstraint{Field: "contextTokens", Kind: ConstraintNonNegative}),
	})
	nonNegative[protocol.RunProgress](s, "step", "contextTokens")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.RunMetrics](),
		Constraints: []FieldConstraint{
			{Field: "steps", Kind: ConstraintNonNegative},
			{Field: "activeDurationMillis", Kind: ConstraintNonNegative},
			{Field: "activeDurationMillis", Kind: ConstraintMaximum, Limit: protocol.MaximumDurationMilliseconds},
		},
	})
	nonNegative[protocol.ModelUsage](s,
		"inputTokens", "outputTokens", "cacheReadTokens", "cacheWriteTokens", "reasoningTokens", "costUsd",
	)
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.Usage](),
		Constraints: modelUsageMapIdentities("byModel"),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.RunCapabilities](),
		Constraints: []FieldConstraint{
			{Field: "requiredFeatures", Kind: ConstraintUniqueItems},
			{Field: "interruptTypes", Kind: ConstraintUniqueItems},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.StartRunRequest](),
		Constraints: append(append(resourceIdentity("sessionId"), []FieldConstraint{
			{Field: "input", Kind: ConstraintNonEmptyItems},
		}...), modelSelectionIdentities()...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.RunSessionOpening](),
		Constraints: append(requiredResourceIdentity("sessionId"), requiredResourceIdentity("runId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GenerationParams](),
		Constraints: []FieldConstraint{
			{Field: "temperature", Kind: ConstraintNonNegative},
			{Field: "temperature", Kind: ConstraintMaximum, Limit: 2},
			{Field: "maxTokens", Kind: ConstraintPositive},
			{Field: "topP", Kind: ConstraintNonNegative},
			{Field: "topP", Kind: ConstraintMaximum, Limit: 1},
			{Field: "stop", Kind: ConstraintNonEmptyItems},
			{Field: "stop", Kind: ConstraintUniqueItems},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ModelTokenLimits](),
		Constraints: []FieldConstraint{
			{Field: "contextWindow", Kind: ConstraintPositive},
			{Field: "maxInputTokens", Kind: ConstraintPositive},
			{Field: "maxOutputTokens", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ResumeRunRequest](),
		Constraints: append(requiredResourceIdentity("runId"),
			[]FieldConstraint{
				{Field: "input", Kind: ConstraintNonEmptyItems},
			}...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Question](),
		Constraints: []FieldConstraint{
			{Field: "fields", Kind: ConstraintNonEmptyItems},
			{Field: "fields", Kind: ConstraintMaxItems, Limit: questioncontract.MaximumFields},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.QuestionField](),
		Constraints: []FieldConstraint{
			{Field: "prompt", Kind: ConstraintPattern, Value: `\S`},
			{Field: "header", Kind: ConstraintMaxLength, Limit: transcript.MaximumQuestionHeaderCharacters},
			{Field: "options", Kind: ConstraintMinItems, Limit: questioncontract.MinimumOptions},
			{Field: "options", Kind: ConstraintMaxItems, Limit: questioncontract.MaximumOptions},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.QuestionOption](),
		Constraints: []FieldConstraint{
			{Field: "label", Kind: ConstraintPattern, Value: `\S`},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.InterruptResponseValue](),
		Constraints: []FieldConstraint{{Field: "answers", Kind: ConstraintNonEmptyItems}},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.StartRunResponse](),
		Constraints: append(append(requiredResourceIdentity("runId"), requiredResourceIdentity("segmentId")...),
			requiredResourceIdentity("userItemId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ResumeRunResponse](),
		Constraints: append(append(requiredResourceIdentity("runId"), requiredResourceIdentity("segmentId")...),
			requiredResourceIdentity("userItemId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.SubscribeRunRequest](),
		Constraints: append(requiredResourceIdentity("runId"), requiredResourceIdentity("segmentId")...),
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.GetRunRequest](), Constraints: requiredResourceIdentity("runId")})
	for _, shape := range []reflect.Type{typeOf[protocol.GetRunAcceptanceRequest](), typeOf[protocol.PluginCommandRequest]()} {
		s.valueConstraint(FieldConstraintSpec{
			GoType: shape,
			Constraints: []FieldConstraint{
				{Field: "namespace", Kind: ConstraintNonEmpty},
				{Field: "namespace", Kind: ConstraintPattern, Value: operation.ReplayNamespacePattern()},
				{Field: "key", Kind: ConstraintNonEmpty},
				{Field: "key", Kind: ConstraintMaxLength, Limit: protocol.MaximumIdempotencyKeyCharacters},
			},
		})
	}
	runAcceptanceIdentity := resourceIdentity("runId")
	runAcceptanceIdentity = append(runAcceptanceIdentity, resourceIdentity("segmentId")...)
	runAcceptanceIdentity = append(runAcceptanceIdentity, resourceIdentity("sessionId")...)
	runAcceptanceIdentity = append(runAcceptanceIdentity, resourceIdentity("userItemId")...)
	runAcceptanceIdentity = append(runAcceptanceIdentity, resourceIdentity("candidateUserItemId")...)
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.RunAcceptance](), Constraints: runAcceptanceIdentity})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.CancelRunRequest](),
		Constraints: append(requiredResourceIdentity("runId"), []FieldConstraint{
			{Field: "reason", Kind: ConstraintMaxLength, Limit: runs.MaxCancellationReasonCharacters},
		}...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SteerRunRequest](),
		Constraints: append(append(requiredResourceIdentity("runId"), requiredResourceIdentity("expectedSegmentId")...), []FieldConstraint{
			{Field: "input", Kind: ConstraintNonEmptyItems},
		}...),
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SteerRunResponse](), Constraints: requiredResourceIdentity("userItemId")})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ListRunsRequest](),
		Constraints: append(resourceIdentity("sessionId"), []FieldConstraint{
			{Field: "statuses", Kind: ConstraintNonEmptyItems},
			{Field: "statuses", Kind: ConstraintUniqueItems},
		}...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.ListInterruptsRequest](),
		Constraints: append(resourceIdentity("sessionId"), resourceIdentity("rootRunId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.ItemListScope](),
		Constraints: append(resourceIdentity("sessionId"), resourceIdentity("runId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.StreamEvent](),
		Constraints: resourceIdentity("itemId"),
	})
}

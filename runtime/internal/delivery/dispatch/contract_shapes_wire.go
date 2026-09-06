package dispatch

import (
	"reflect"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// shapes is the registered union / constraint contract.
//
// Every high-risk discriminated union in the contract is registered here. The Go
// type, runtime validator, JSON Schema, and generated TypeScript all consume the
// same declaration so a variant cannot drift in only one layer. Extension seams
// remain narrow pattern branches rather than arbitrary strings.
var shapes = buildShapes()

func buildShapes() *Shapes {
	s := &Shapes{}
	registerNotifications(s)
	registerProblemUnion(s)
	registerRunUnions(s)
	registerItemUnions(s)
	registerProviderUnions(s)
	registerMCPUnions(s)
	registerInterruptUnions(s)
	registerEventUnions(s)
	registerArtifactUnions(s)
	registerDiffUnions(s)
	registerObjectConstraints(s)
	registerCarriedShapes(s)
	registerValueConstraints(s)
	return s
}

func registerProviderUnions(s *Shapes) {
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ProviderConfigChange](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ProviderConfigSet), Required: []string{"value"}},
			{Tag: string(protocol.ProviderConfigClear)},
		},
	})
}

func registerMCPUnions(s *Shapes) {
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPHandshakeTimeout](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPHandshakeUnbounded)},
			{Tag: string(protocol.MCPHandshakeBounded), Required: []string{"seconds"}},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPConnection](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPTransportStreamableHTTP), Required: []string{"url"}, Optional: []string{"authorizationMasked", "headersMasked"}},
			{Tag: string(protocol.MCPTransportStdio), Required: []string{"command"}, Optional: []string{"args", "envMasked", "dir"}},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPConnectionInput](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPTransportStreamableHTTP), Required: []string{"url"}, Optional: []string{"authorization", "headers"}},
			{Tag: string(protocol.MCPTransportStdio), Required: []string{"command"}, Optional: []string{"args", "env", "dir"}},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPAuthorizationChange](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPSecretSet), Required: []string{"value"}},
			{Tag: string(protocol.MCPSecretClear)},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPHeadersChange](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPSecretSet), Required: []string{"value"}},
			{Tag: string(protocol.MCPSecretClear)},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPEnvironmentChange](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPSecretSet), Required: []string{"value"}},
			{Tag: string(protocol.MCPSecretClear)},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPServerState](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPServerDisabled)},
			{Tag: string(protocol.MCPServerDisconnected)},
			{Tag: string(protocol.MCPServerConnecting)},
			{Tag: string(protocol.MCPServerConnected), Required: []string{"toolCount"}},
			{Tag: string(protocol.MCPServerFailed), Required: []string{"error"}},
			{Tag: string(protocol.MCPServerNeedsAuth), Required: []string{"error"}},
		},
	})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.MCPAuthorizationAttemptStatus](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPAuthorizationAttemptPending)},
			{Tag: string(protocol.MCPAuthorizationAttemptSucceeded)},
			{Tag: string(protocol.MCPAuthorizationAttemptFailed), Required: []string{"error"}},
			{Tag: string(protocol.MCPAuthorizationAttemptCanceled)},
		},
	})
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
		GoType: typeOf[protocol.ProblemData](), Discriminator: "type", Variants: variants,
		PatternVariant: &PatternVariantSpec{
			TagPattern:     `^plugin:[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._-]*$`,
			TypeScriptType: "`plugin:${string}/${string}`",
			Optional:       []string{"detail", "docUrl", "retryAfterSeconds"},
		},
	})
}

func registerNotifications(s *Shapes) {
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

func allowedArtifactProblemTypes(field string, kinds ...protocol.ArtifactProblemType) []AllowedValueSet {
	values := make([]string, len(kinds))
	for index, kind := range kinds {
		values[index] = string(kind)
	}
	return []AllowedValueSet{{Field: field, Values: values}}
}

func allowedGoalReasonCodes(codes ...protocol.GoalReasonCode) []AllowedValueSet {
	values := make([]string, len(codes))
	for index, code := range codes {
		values[index] = string(code)
	}
	return []AllowedValueSet{{Field: "reason.code", Values: values}}
}

func registerRunUnions(s *Shapes) {
	s.union(UnionSpec{
		GoType:        typeOf[protocol.CancelRunResponse](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.CancelRunRoot), Required: []string{"run"}},
			{Tag: string(protocol.CancelRunChild), Required: []string{"run", "rootRun"}},
		},
	})

	// A terminal says only why the run stopped; what it consumed is published
	// beside it as metrics. `detail` is the human-readable note the non-error
	// terminals may add — the error terminal's note stays on
	// error.detail, never duplicated here.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.RunOutcome](),
		Discriminator: "type",
		Variants:      runOutcomeVariants(),
	})

	// A segment stops for any reason a run does, plus the two that leave the run
	// alive. The terminal variants are the SAME list, converted — because
	// SegmentOutcome contains RunOutcome, and a second list is how a terminal comes
	// to be legal for one and not the other.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.SegmentOutcome](),
		Discriminator: "type",
		Variants: append([]VariantSpec{
			{Tag: string(protocol.SegmentInterrupt), Required: []string{"interrupts"}},
			// `suspended` adds nothing: the interrupts belong to the run that raised
			// them, so a run stopped by someone else's barrier carries none.
			{Tag: string(protocol.SegmentSuspended)},
		}, runOutcomeVariants()...),
	})
}

// runOutcomeVariants is the terminal half of both run-outcome unions. It is a
// function rather than a shared slice because a VariantSpec holds slices a caller
// could otherwise append into, and the two registrations must not be able to
// reach each other's fields.
func runOutcomeVariants() []VariantSpec {
	return []VariantSpec{
		{Tag: string(protocol.OutcomeCompleted)},
		{Tag: string(protocol.OutcomeTimedOut), Required: []string{"error"}},
		{Tag: string(protocol.OutcomeFailed), Required: []string{"error"}},
		{Tag: string(protocol.OutcomeMaxSteps), Optional: []string{"detail"}},
		{Tag: string(protocol.OutcomeMaxBudget), Optional: []string{"detail"}},
		{Tag: string(protocol.OutcomeCanceled), Optional: []string{"detail"}},
		{Tag: string(protocol.OutcomeLost), Required: []string{"error"}},
	}
}

func registerItemUnions(s *Shapes) {
	// Identity/status are shared. Time is intentionally variant-specific:
	// ToolCall uses startedAt, while every other item uses createdAt. A variant
	// declares the WHOLE frame it permits, making the two terms mutually exclusive.
	itemIdentityFields := []string{"id", "runId", "status"}
	createdItemFields := slices.Concat(itemIdentityFields, []string{"createdAt"})
	toolItemFields := slices.Concat(itemIdentityFields, []string{"startedAt"})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.Item](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{
				Tag: string(protocol.ItemTypeUserMessage), Required: slices.Concat(createdItemFields, []string{"content"}),
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			}, {
				Tag: string(protocol.ItemTypeAgentMessage), Required: createdItemFields, Optional: []string{"phase", "content"},
				AllowedValues: allowedItemStatuses(protocol.ItemStatusRunning, protocol.ItemStatusCompleted),
			}, {
				Tag: string(protocol.ItemTypeReasoning), Required: createdItemFields, Optional: []string{"text", "redacted"},
				AllowedValues: allowedItemStatuses(protocol.ItemStatusRunning, protocol.ItemStatusCompleted),
			}, {
				Tag: string(protocol.ItemTypeQuestion), Required: slices.Concat(createdItemFields, []string{"question"}),
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			}, {
				Tag: string(protocol.ItemTypeToolCall), Required: slices.Concat(toolItemFields, []string{"tool"}), Optional: []string{"finishedAt", "durationMillis", "safetyClass", "approvalDecision", "error"},
				AllowedValues: allowedItemStatuses(protocol.ItemStatusRunning, protocol.ItemStatusCompleted, protocol.ItemStatusIncomplete),
			}, {
				Tag: string(protocol.ItemTypeCompaction), Required: slices.Concat(createdItemFields, []string{"summary"}), Optional: []string{"droppedMessages"},
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			},
		},
	})

	// Every delta is ephemeral and every one has a named authoritative landing
	// in the contract. toolArguments is partial JSON TEXT, not an object — the
	// parsed value only exists on the completed item.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ItemDelta](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.DeltaContent), Required: []string{"text"}},
			{Tag: string(protocol.DeltaReasoning), Required: []string{"text"}},
			{Tag: string(protocol.DeltaToolArguments), Required: []string{"argumentsTextDelta"}},
			{Tag: string(protocol.DeltaToolOutput), Required: []string{"text"}},
		},
	})

	// Three short registries, so a gap says which vocabulary its name belongs to. The
	// variants carry the same field set on purpose: what differs is what `name` MEANS,
	// and each registry publishes its own values rather than restating them here.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.CapabilityRequirement](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.RequirementFeature), Required: []string{"name"}},
			{Tag: string(protocol.RequirementInterruptType), Required: []string{"name"}},
			{Tag: string(protocol.RequirementRuntimeTopic), Required: []string{"name"}},
		},
	})

	// What a page of items is a page OF. The two subjects are exclusive, not two
	// optional filters: a frame naming both would need a precedence rule to resolve,
	// and a precedence rule is where the request and the answer start to disagree.
	// Only run scope may ask for descendants — the session timeline already holds
	// every descendant, so the flag would narrow nothing there.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ItemListScope](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ItemScopeSession), Required: []string{"sessionId"}},
			{Tag: string(protocol.ItemScopeRun), Required: []string{"runId"}, Optional: []string{"includeDescendants"}},
		},
	})

	s.union(UnionSpec{
		GoType:        typeOf[protocol.ContentBlock](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ContentBlockText), Required: []string{"text"}},
			// Images are inline: mime + raw base64, no data: prefix, no upload channel.
			{Tag: string(protocol.ContentBlockImage), Required: []string{"mime", "data"}},
		},
	})

	for _, fieldType := range []reflect.Type{
		typeOf[protocol.QuestionField](),
		typeOf[protocol.ArtifactQuestionField](),
	} {
		s.union(UnionSpec{
			GoType:        fieldType,
			Discriminator: "type",
			Variants: []VariantSpec{
				{Tag: string(protocol.QuestionFieldText), Required: []string{"prompt"}, Optional: []string{"header"}},
				{Tag: string(protocol.QuestionFieldChoice), Required: []string{"prompt", "options"}, Optional: []string{"header", "multiple", "allowCustom"}},
			},
		})
	}
}

func registerInterruptUnions(s *Shapes) {
	// The variant fields live inside `payload`, so the spec addresses them by
	// dotted path. Rendering a pending interrupt needs no second request. Each variant
	// carries the identity pair: which item is waiting, and which run asked.
	identity := []string{"itemId", "runId"}
	s.union(UnionSpec{
		GoType:        typeOf[protocol.Interrupt](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{
				Tag:      string(protocol.InterruptApproval),
				Required: append(slices.Clone(identity), "payload.tool"),
				Optional: []string{"payload.risk", "payload.reason", "payload.rememberable"},
			},
			{Tag: string(protocol.InterruptQuestion), Required: append(slices.Clone(identity), "payload.question")},
		},
	})

	// editedArgs is one-shot by design: a remembered rule matches by the call's
	// subject, never by a one-off argument rewrite.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.InterruptResponseValue](),
		Discriminator: "type",
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
		GoType:        typeOf[protocol.StreamEvent](),
		Discriminator: "type",
		Forbidden:     []string{"durable"},
		Variants: []VariantSpec{
			{Tag: string(protocol.StreamSegmentStarted), Required: []string{"run"}},
			{Tag: string(protocol.StreamSegmentProgress), Required: []string{"progress"}},
			{Tag: string(protocol.StreamSegmentFinished), Required: []string{"outcome", "metrics", "contextTokens"}},
			{Tag: string(protocol.StreamItemStarted), Required: []string{"item"}},
			{Tag: string(protocol.StreamItemDelta), Required: []string{"itemId", "delta"}},
			{Tag: string(protocol.StreamItemCompleted), Required: []string{"item"}},
			{Tag: string(protocol.StreamPlanUpdated), Required: []string{"plan"}},
		},
	})

	// Every variant is an invalidation: `sequence` plus the ids that moved. A variant
	// may NOT carry the resource's new value or become a second source of truth for
	// the authoritative query after a dropped frame.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.RuntimeEvent](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.RuntimeFilesChanged), Required: []string{"sequence", "paths"}, Optional: []string{"watchId", "workspace"}},
			{Tag: string(protocol.RuntimeSkillsChanged), Required: []string{"sequence"}, Optional: []string{"names"}},
			{Tag: string(protocol.RuntimeMCPChanged), Required: []string{"sequence"}, Optional: []string{"serverIds"}},
			{Tag: string(protocol.RuntimeSchedulesChanged), Required: []string{"sequence"}, Optional: []string{"scheduleIds"}},
			{Tag: string(protocol.RuntimeSessionsChanged), Required: []string{"sequence"}, Optional: []string{"sessionIds"}},
			{Tag: string(protocol.RuntimeRunsChanged), Required: []string{"sequence"}, Optional: []string{"runIds", "sessionIds"}},
			{Tag: string(protocol.RuntimePlanChanged), Required: []string{"sequence"}, Optional: []string{"sessionIds"}},
			{Tag: string(protocol.RuntimeGoalsChanged), Required: []string{"sequence"}, Optional: []string{"sessionIds"}},
			{Tag: string(protocol.RuntimeInterruptsChanged), Required: []string{"sequence"}, Optional: []string{"runIds", "sessionIds"}},
			{Tag: string(protocol.RuntimeKnowledgeChanged), Required: []string{"sequence"}},
			{Tag: string(protocol.RuntimeHooksChanged), Required: []string{"sequence"}},
			{Tag: string(protocol.RuntimeModelsChanged), Required: []string{"sequence"}},
			{Tag: string(protocol.RuntimeApprovalsChanged), Required: []string{"sequence"}},
			{Tag: string(protocol.RuntimeAgentMemoryChanged), Required: []string{"sequence"}},
			// Resync names what went stale rather than saying "everything": a client that
			// subscribed broadly should not reload unrelated resources because one watch
			// overflowed.
			{Tag: string(protocol.RuntimeResync), Required: []string{"sequence", "topics"}, Optional: []string{"watchIds"}},
		},
	})
}

func registerArtifactUnions(s *Shapes) {
	// An artifact's terminal vocabulary is deliberately NOT the live RunOutcome
	// union: it cannot carry the live-only interrupt outcome, because a parked
	// waiting executor state is Runtime-instance-local and does not travel.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ArtifactOutcome](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ArtifactOutcomeCompleted)},
			{
				Tag: string(protocol.ArtifactOutcomeTimedOut), Required: []string{"error"},
				AllowedValues: allowedArtifactProblemTypes("error.type", protocol.ArtifactProblemTimeout),
			}, {
				Tag: string(protocol.ArtifactOutcomeFailed), Required: []string{"error"},
				AllowedValues: allowedArtifactProblemTypes(
					"error.type",
					protocol.ArtifactProblemInternalError,
					protocol.ArtifactProblemAgentStuck,
					protocol.ArtifactProblemRateLimited,
					protocol.ArtifactProblemInvalidAPIKey,
					protocol.ArtifactProblemTimeout,
					protocol.ArtifactProblemProviderUnavailable,
					protocol.ArtifactProblemProviderRejected,
				),
			},
			{Tag: string(protocol.ArtifactOutcomeMaxSteps), Optional: []string{"detail"}},
			{Tag: string(protocol.ArtifactOutcomeMaxBudget), Optional: []string{"detail"}},
			{Tag: string(protocol.ArtifactOutcomeCanceled), Optional: []string{"detail"}},
			{
				Tag: string(protocol.ArtifactOutcomeLost), Required: []string{"error"},
				AllowedValues: allowedArtifactProblemTypes("error.type", protocol.ArtifactProblemRunLost),
			},
		},
	})

	itemIdentityFields := []string{"id", "runId", "status"}
	createdItemFields := slices.Concat(itemIdentityFields, []string{"createdAt"})
	toolItemFields := slices.Concat(itemIdentityFields, []string{"startedAt"})
	s.union(UnionSpec{
		GoType:        typeOf[protocol.ArtifactItem](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{
				Tag: string(protocol.ItemTypeUserMessage), Required: slices.Concat(createdItemFields, []string{"content"}),
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			}, {
				Tag: string(protocol.ItemTypeAgentMessage), Required: slices.Concat(createdItemFields, []string{"phase", "content"}),
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			}, {
				Tag: string(protocol.ItemTypeReasoning), Required: slices.Concat(createdItemFields, []string{"text"}), Optional: []string{"redacted"},
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			}, {
				Tag: string(protocol.ItemTypeQuestion), Required: slices.Concat(createdItemFields, []string{"question"}),
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			}, {
				Tag: string(protocol.ItemTypeToolCall), Required: slices.Concat(toolItemFields, []string{"tool"}), Optional: []string{"finishedAt", "durationMillis", "safetyClass", "approvalDecision", "error"},
				AllowedValues: slices.Concat(
					allowedItemStatuses(protocol.ItemStatusCompleted, protocol.ItemStatusIncomplete),
					allowedArtifactProblemTypes(
						"error.type",
						protocol.ArtifactProblemInternalError,
						protocol.ArtifactProblemDeniedByUser,
						protocol.ArtifactProblemToolFailed,
						protocol.ArtifactProblemChildRunCanceled,
						protocol.ArtifactProblemToolCanceled,
					),
				),
			}, {
				Tag: string(protocol.ItemTypeCompaction), Required: slices.Concat(createdItemFields, []string{"summary"}), Optional: []string{"droppedMessages"},
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			},
		},
	})

	s.union(UnionSpec{
		GoType:        typeOf[protocol.ArtifactContentBlock](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.ContentBlockText), Required: []string{"text"}},
			{Tag: string(protocol.ContentBlockImage), Required: []string{"mime", "data"}},
		},
	})
}

func registerDiffUnions(s *Shapes) {
	// A diff row's godoc has always described a union — a hunk carries text, a context
	// row carries both line numbers, an added row only the right one — and clients
	// modeled it as one. Nothing said so on the wire, so the generated shape permitted
	// a row carrying a hunk's text AND both line numbers at once.
	s.union(UnionSpec{
		GoType:        typeOf[protocol.DiffRow](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: string(protocol.DiffRowHunk), Required: []string{"text"}},
			// The line numbers are `omitempty` because one flat struct serves four
			// tags, so an added row must be able to drop leftLine. They are REQUIRED
			// here anyway: a context row carries both, and a unified diff numbers
			// lines from 1, so the zero omitempty drops never occurs.
			{Tag: string(protocol.DiffRowContext), Required: []string{"code", "leftLine", "rightLine"}},
			{Tag: string(protocol.DiffRowAdded), Required: []string{"code", "rightLine"}},
			{Tag: string(protocol.DiffRowDeleted), Required: []string{"code", "leftLine"}},
		},
	})
}

func registerObjectConstraints(s *Shapes) {
	for _, constrained := range []struct {
		goType      reflect.Type
		requiredAny []string
	}{
		{goType: typeOf[protocol.GoalBudget](), requiredAny: []string{"maxRuns", "maxCostUsd", "maxSteps"}},
		{goType: typeOf[protocol.RunLimits](), requiredAny: []string{"maxTotalTokens", "maxSteps", "maxBudgetUsd"}},
		{goType: typeOf[protocol.ArtifactRunLimits](), requiredAny: []string{"maxTotalTokens", "maxSteps", "maxBudgetUsd"}},
		{goType: typeOf[protocol.ModelTokenLimits](), requiredAny: []string{"contextWindow", "maxInputTokens", "maxOutputTokens"}},
		{goType: typeOf[protocol.FeedbackRequest](), requiredAny: []string{"rating", "text"}},
	} {
		s.constraint(ObjectConstraintSpec{
			GoType: constrained.goType,
			Rules:  []ConditionalRule{{RequiredAny: constrained.requiredAny}},
		})
	}

	// A progress frame is a preview of one or more independently advancing facts,
	// never a heartbeat with an empty object. Stating the alternatives as one set
	// preserves that several of them may legitimately arrive together.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunProgress](),
		Rules: []ConditionalRule{{
			RequiredAny: []string{"step", "usage", "contextTokens", "activity"},
		}},
	})

	// Every stream frame is an ordered observation. A zero timestamp would make
	// replayed and live frames indistinguishable from an uninitialized envelope.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunEvent](),
		Rules:  []ConditionalRule{{Required: []string{"timestamp"}}},
	})

	// These are the public projection of the aggregate's immutable origin and
	// latest replacement time. Neither has an uninitialized live meaning.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.Session](),
		Rules:  []ConditionalRule{{Required: []string{"createdAt", "updatedAt"}}},
	})

	// A PlanState is a committed replacement, so its revision and commit time
	// are one value rather than independently optional metadata.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.PlanState](),
		Rules:  []ConditionalRule{{Required: []string{"updatedAt"}}},
	})

	hookRules := []ConditionalRule{{
		RequiredAny: []string{"command", "inject"},
	}, {
		When:      []delivery.FieldCondition{{Field: "command", Operator: delivery.OperatorPresent}},
		Forbidden: []string{"inject"},
	}, {
		When:      []delivery.FieldCondition{{Field: "inject", Operator: delivery.OperatorPresent}},
		Forbidden: []string{"command", "matcher", "timeoutMillis"},
	}}
	for _, event := range []protocol.HookEvent{
		protocol.HookEventUserPromptSubmit,
		protocol.HookEventSessionStart,
		protocol.HookEventSubagentStart,
		protocol.HookEventSubagentStop,
		protocol.HookEventPreCompact,
		protocol.HookEventStop,
		protocol.HookEventNotification,
	} {
		hookRules = append(hookRules, ConditionalRule{
			When:      []delivery.FieldCondition{{Field: "event", Operator: delivery.OperatorEquals, Value: string(event)}},
			Forbidden: []string{"matcher"},
		})
	}
	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.HookInfo](), Rules: hookRules})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ApprovalRule](),
		Rules: []ConditionalRule{{
			When:     []delivery.FieldCondition{{Field: "scope", Operator: delivery.OperatorEquals, Value: string(protocol.ApprovalRuleScopeProject)}},
			Required: []string{"dir"},
		}, {
			When:      []delivery.FieldCondition{{Field: "scope", Operator: delivery.OperatorEquals, Value: string(protocol.ApprovalRuleScopeSession)}},
			Forbidden: []string{"dir"},
		}, {
			When:      []delivery.FieldCondition{{Field: "scope", Operator: delivery.OperatorEquals, Value: string(protocol.ApprovalRuleScopeGlobal)}},
			Forbidden: []string{"dir"},
		}},
	})

	knowledgeTargetRules := []ConditionalRule{{
		When:      []delivery.FieldCondition{{Field: "scope", Operator: delivery.OperatorEquals, Value: string(protocol.KnowledgeScopeHome)}},
		Forbidden: []string{"workspace"},
	}, {
		When:     []delivery.FieldCondition{{Field: "scope", Operator: delivery.OperatorEquals, Value: string(protocol.KnowledgeScopeCWD)}},
		Required: []string{"workspace"},
	}, {
		When:     []delivery.FieldCondition{{Field: "scope", Operator: delivery.OperatorEquals, Value: string(protocol.KnowledgeScopeProjectRoot)}},
		Required: []string{"workspace"},
	}}
	for _, target := range []reflect.Type{
		typeOf[protocol.GetKnowledgeRequest](),
		typeOf[protocol.UpdateKnowledgeRequest](),
	} {
		s.constraint(ObjectConstraintSpec{GoType: target, Rules: knowledgeTargetRules})
	}

	// A file end line is meaningful only as the end of a window that starts at
	// an explicit line. Structured-diff row budgets likewise have no meaning on
	// the raw patch branch.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ReadFileRequest](),
		Rules: []ConditionalRule{{
			When:     []delivery.FieldCondition{{Field: "endLine", Operator: delivery.OperatorPresent}},
			Required: []string{"startLine"},
		}},
	})
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.FileContent](),
		Rules: []ConditionalRule{{
			When:     []delivery.FieldCondition{{Field: "startLine", Operator: delivery.OperatorPresent}},
			Required: []string{"endLine"},
		}, {
			When:     []delivery.FieldCondition{{Field: "endLine", Operator: delivery.OperatorPresent}},
			Required: []string{"startLine"},
		}},
	})
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.GetDiffRequest](),
		Rules: []ConditionalRule{{
			When:      []delivery.FieldCondition{{Field: "format", Operator: delivery.OperatorEquals, Value: string(protocol.DiffFormatRaw)}},
			Forbidden: []string{"limit"},
		}},
	})
	workspaceChangeRules := []ConditionalRule{{
		When:     []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.FileStatusRenamed)}},
		Required: []string{"previousPath"},
	}, {
		When:      []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.FileStatusAdded)}},
		Forbidden: []string{"previousPath"},
	}, {
		When:      []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.FileStatusModified)}},
		Forbidden: []string{"previousPath"},
	}, {
		When:      []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.FileStatusDeleted)}},
		Forbidden: []string{"previousPath"},
	}, {
		When:      []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.FileStatusUntracked)}},
		Forbidden: []string{"previousPath"},
	}, {
		When:      []delivery.FieldCondition{{Field: "binary", Operator: delivery.OperatorPresent}},
		Forbidden: []string{"added", "removed"},
	}}
	for _, shape := range []reflect.Type{
		typeOf[protocol.WorkspaceFileChange](),
		typeOf[protocol.FileDiff](),
	} {
		s.constraint(ObjectConstraintSpec{GoType: shape, Rules: workspaceChangeRules})
	}

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.StreamEvent](),
		Rules: []ConditionalRule{{
			When: []delivery.FieldCondition{{
				Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.StreamItemStarted),
			}},
			AllowedValues: slices.Concat(
				allowedStreamItemKinds(protocol.ItemTypeAgentMessage, protocol.ItemTypeReasoning, protocol.ItemTypeToolCall),
				allowedStreamItemStatuses(protocol.ItemStatusRunning),
			),
		}, {
			When: []delivery.FieldCondition{{
				Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.StreamItemCompleted),
			}},
			AllowedValues: allowedStreamItemStatuses(
				protocol.ItemStatusCompleted,
				protocol.ItemStatusIncomplete,
			),
		}, {
			When: []delivery.FieldCondition{{
				Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.StreamPlanUpdated),
			}},
			Required: []string{"plan.state"},
		}},
	})

	// Value selections are either wholly absent (inherit the surrounding
	// default) or name one exact model. Reasoning effort belongs to that exact
	// identity and therefore cannot be supplied on its own.
	modelSelectionRules := []ConditionalRule{{
		When:     []delivery.FieldCondition{{Field: "provider", Operator: delivery.OperatorPresent}},
		Required: []string{"model"},
	}, {
		When:     []delivery.FieldCondition{{Field: "model", Operator: delivery.OperatorPresent}},
		Required: []string{"provider"},
	}, {
		When:     []delivery.FieldCondition{{Field: "reasoningEffort", Operator: delivery.OperatorPresent}},
		Required: []string{"provider", "model"},
	}}
	for _, target := range []reflect.Type{
		typeOf[protocol.StartRunRequest](),
		typeOf[protocol.StartGoalRequest](),
		typeOf[protocol.CreateScheduleRequest](),
		typeOf[protocol.Schedule](),
	} {
		rules := modelSelectionRules
		if target == typeOf[protocol.Schedule]() {
			rules = slices.Concat([]ConditionalRule{{Required: []string{"createdAt"}}}, rules)
		}
		s.constraint(ObjectConstraintSpec{
			GoType: target,
			Rules:  rules,
		})
	}

	// Patch selections may update reasoning effort alone against the stored
	// exact model, but changing the model identity remains an atomic pair.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.UpdateSessionRequest](),
		Rules: []ConditionalRule{{
			When:     []delivery.FieldCondition{{Field: "provider", Operator: delivery.OperatorPresent}},
			Required: []string{"model"},
		}, {
			When:     []delivery.FieldCondition{{Field: "model", Operator: delivery.OperatorPresent}},
			Required: []string{"provider"},
		}},
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.Goal](),
		Rules: []ConditionalRule{{
			Required: []string{"createdAt", "updatedAt"},
		}, {
			When:      []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.GoalActive)}},
			Forbidden: []string{"reason"},
		}, {
			When:      []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.GoalCompleting)}},
			Forbidden: []string{"reason"},
		}, {
			When:     []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.GoalPaused)}},
			Required: []string{"reason"},
			AllowedValues: allowedGoalReasonCodes(
				protocol.GoalReasonStoppedByUser,
				protocol.GoalReasonRuntimeRestarted,
				protocol.GoalReasonRunStartFailed,
				protocol.GoalReasonAwaitingInput,
				protocol.GoalReasonTerminalOutcomeMissing,
				protocol.GoalReasonRunNotCompleted,
			),
		}, {
			When:     []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.GoalBlocked)}},
			Required: []string{"reason"},
			AllowedValues: allowedGoalReasonCodes(
				protocol.GoalReasonRunBudgetReached,
				protocol.GoalReasonCostBudgetReached,
				protocol.GoalReasonStepBudgetReached,
				protocol.GoalReasonPricingUnavailable,
				protocol.GoalReasonBlockedByModel,
			),
		}},
	})
	goalReasonRules := []ConditionalRule{
		{
			When:     []delivery.FieldCondition{{Field: "code", Operator: delivery.OperatorEquals, Value: string(protocol.GoalReasonRunNotCompleted)}},
			Required: []string{"detail"},
		}, {
			When:     []delivery.FieldCondition{{Field: "code", Operator: delivery.OperatorEquals, Value: string(protocol.GoalReasonBlockedByModel)}},
			Required: []string{"detail"},
		},
	}
	for _, code := range []protocol.GoalReasonCode{
		protocol.GoalReasonStoppedByUser,
		protocol.GoalReasonRuntimeRestarted,
		protocol.GoalReasonRunStartFailed,
		protocol.GoalReasonAwaitingInput,
		protocol.GoalReasonTerminalOutcomeMissing,
		protocol.GoalReasonRunBudgetReached,
		protocol.GoalReasonCostBudgetReached,
		protocol.GoalReasonStepBudgetReached,
		protocol.GoalReasonPricingUnavailable,
	} {
		goalReasonRules = append(goalReasonRules, ConditionalRule{
			When:      []delivery.FieldCondition{{Field: "code", Operator: delivery.OperatorEquals, Value: string(code)}},
			Forbidden: []string{"detail"},
		})
	}
	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.GoalReason](), Rules: goalReasonRules})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.UpdateScheduleRequest](),
		Rules: []ConditionalRule{{
			When:     []delivery.FieldCondition{{Field: "provider", Operator: delivery.OperatorPresent}},
			Required: []string{"model"},
		}, {
			When:     []delivery.FieldCondition{{Field: "model", Operator: delivery.OperatorPresent}},
			Required: []string{"provider"},
		}, {
			When:      []delivery.FieldCondition{{Field: "workspaceMode", Operator: delivery.OperatorEquals, Value: string(protocol.ScheduleWorkspaceDefault)}},
			Forbidden: []string{"workspace"},
		}},
	})

	terminalToolRules := []ConditionalRule{{
		When: []delivery.FieldCondition{
			{Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.ItemTypeToolCall)},
			{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusCompleted)},
		},
		Required:  []string{"finishedAt", "durationMillis"},
		Forbidden: []string{"error"},
	}, {
		When: []delivery.FieldCondition{
			{Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.ItemTypeToolCall)},
			{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusIncomplete)},
		},
		Required: []string{"finishedAt"},
	}}
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactItem](),
		Rules:  terminalToolRules,
	})

	runtimeItemRules := slices.Concat(terminalToolRules, []ConditionalRule{{
		When: []delivery.FieldCondition{
			{Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.ItemTypeToolCall)},
			{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusRunning)},
		},
		Forbidden: []string{"finishedAt", "durationMillis"},
	}, {
		When: []delivery.FieldCondition{
			{Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.ItemTypeAgentMessage)},
			{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusRunning)},
		},
		Forbidden: []string{"phase"},
	}, {
		When: []delivery.FieldCondition{
			{Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.ItemTypeAgentMessage)},
			{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusCompleted)},
		},
		Required: []string{"phase", "content"},
	}, {
		When: []delivery.FieldCondition{
			{Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.ItemTypeReasoning)},
			{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusCompleted)},
		},
		Required: []string{"text"},
	}})
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.Item](),
		Rules:  runtimeItemRules,
	})

	for _, target := range []reflect.Type{
		typeOf[protocol.AgentMemoryListRequest](),
		typeOf[protocol.AgentMemoryAddRequest](),
	} {
		s.constraint(ObjectConstraintSpec{
			GoType: target,
			Rules: []ConditionalRule{{
				When:     []delivery.FieldCondition{{Field: "scope", Operator: delivery.OperatorEquals, Value: string(protocol.AgentMemoryScopeProject)}},
				Required: []string{"workspace"},
			}, {
				When:      []delivery.FieldCondition{{Field: "scope", Operator: delivery.OperatorEquals, Value: string(protocol.AgentMemoryScopeUser)}},
				Forbidden: []string{"workspace"},
			}},
		})
	}
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.AgentMemoryUpdateRequest](),
		Rules:  []ConditionalRule{{RequiredAny: []string{"content", "pinned"}}},
	})
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.AgentMemoryItem](),
		Rules: []ConditionalRule{{
			Required: []string{"createdAt", "updatedAt"},
		}, {
			When: []delivery.FieldCondition{{
				Field: "origin", Operator: delivery.OperatorEquals, Value: string(protocol.AgentMemoryOriginUser),
			}},
			AllowedValues: []AllowedValueSet{{Field: "status", Values: []string{string(protocol.AgentMemoryStatusActive)}}},
		}},
	})
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RollbackSessionRequest](),
		Rules: []ConditionalRule{{
			When: []delivery.FieldCondition{{
				Field: "restoreType", Operator: delivery.OperatorEquals, Value: string(protocol.RestoreFiles),
			}},
			Required: []string{"toRunId"},
		}, {
			When: []delivery.FieldCondition{{
				Field: "restoreType", Operator: delivery.OperatorEquals, Value: string(protocol.RestoreBoth),
			}},
			Required: []string{"toRunId"},
		}},
	})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.MCPAuthorizationAttempt](),
		Rules: []ConditionalRule{{
			Required: []string{"createdAt"},
		}, {
			When:      []delivery.FieldCondition{{Field: "status.type", Operator: delivery.OperatorEquals, Value: string(protocol.MCPAuthorizationAttemptPending)}},
			Forbidden: []string{"finishedAt"},
		}, {
			When:     []delivery.FieldCondition{{Field: "status.type", Operator: delivery.OperatorEquals, Value: string(protocol.MCPAuthorizationAttemptSucceeded)}},
			Required: []string{"finishedAt"},
		}, {
			When:     []delivery.FieldCondition{{Field: "status.type", Operator: delivery.OperatorEquals, Value: string(protocol.MCPAuthorizationAttemptFailed)}},
			Required: []string{"finishedAt"},
		}, {
			When:     []delivery.FieldCondition{{Field: "status.type", Operator: delivery.OperatorEquals, Value: string(protocol.MCPAuthorizationAttemptCanceled)}},
			Required: []string{"finishedAt"},
		}},
	})

	// A finished Run explains itself, and a run that has not finished does not
	// pretend to. Without the first rule `status:"finished"` with no outcome is
	// representable and a client cannot tell "it ended" from "it ended somehow";
	// without the others, a waiting run could carry a terminal reason and a client
	// would stop offering to resume it.
	//
	// These are the SUMMARY's rules, so they hold wherever a summary travels — the
	// page-level runs of items.list as much as a RunRef, which embeds it.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunSummary](),
		Rules: append([]ConditionalRule{{
			Required: []string{"provider", "model", "createdAt"},
		}, {
			When:     []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.RunStatusFinished)}},
			Required: []string{"outcome", "finishedAt"},
		}, {
			When:      []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.RunStatusRunning)}},
			Forbidden: []string{"outcome", "finishedAt"},
		}, {
			When:      []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.RunStatusWaiting)}},
			Forbidden: []string{"outcome", "finishedAt"},
		}}, childLineageRules()...),
	})

	// A RunRef adds the control field, and it exists exactly while a segment is
	// executing: without the first rule a running run can arrive with nothing to
	// attach to, and without the second a client can attach to a stream that
	// already ended.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.RunRef](),
		Rules: []ConditionalRule{{
			When:     []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.RunStatusRunning)}},
			Required: []string{"activeSegmentId"},
		}, {
			When:      []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.RunStatusWaiting)}},
			Forbidden: []string{"activeSegmentId"},
		}, {
			When:      []delivery.FieldCondition{{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.RunStatusFinished)}},
			Forbidden: []string{"activeSegmentId"},
		}},
	})

	// A pending set with no interrupts is not a thing to resume — it would leave
	// the client polling a run that will never move.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.PendingInterruptSet](),
		Rules: []ConditionalRule{{
			Required: []string{"rootRunId", "sessionId", "interrupts", "createdAt"},
		}},
	})

	// The archive's runs obey the same child-edge rule the live summaries do: all
	// three edges or none. A child additionally carries NO protocol profile — it
	// reads its root's, and a child with one of its own would import a run claiming
	// a contract nothing negotiated.
	//
	// The other half — a ROOT must carry one — is not here: "root" is the absence of
	// those edges, and absence is not something a ConditionalRule can condition on. It
	// is an aggregate invariant of the import, checked where the archive is turned
	// into a session.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactRun](),
		Rules: append([]ConditionalRule{
			{
				// Artifacts contain terminal Runs only, so all three lifecycle
				// boundaries are durable facts needed by Run restoration.
				Required: []string{"createdAt", "finishedAt", "updatedAt"},
			},
			{
				When:      []delivery.FieldCondition{{Field: "spawnedByItemId", Operator: delivery.OperatorPresent}},
				Forbidden: []string{"protocolProfile"},
			},
		}, childLineageRules()...),
	})

	// A portable Session retains the aggregate's origin and latest replacement
	// time; importing an uninitialized timestamp cannot reconstruct that owner.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactSession](),
		Rules:  []ConditionalRule{{Required: []string{"createdAt", "updatedAt"}}},
	})

	// A portable offloaded result is a durable blob; its creation time is part
	// of the record restored into the destination store.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactToolResult](),
		Rules:  []ConditionalRule{{Required: []string{"createdAt"}}},
	})

	// ArtifactProblem is shared by Run outcomes and ToolCall transcript items,
	// but retry policy belongs only to the three transient Run classifications.
	// Refuse the field for every permanent and Tool classification so the common
	// wire envelope cannot manufacture a fact neither owning domain accepts.
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactProblem](),
		Rules:  artifactProblemRetryRules(),
	})
}

func artifactProblemRetryRules() []ConditionalRule {
	withoutRetry := []protocol.ArtifactProblemType{
		protocol.ArtifactProblemInternalError,
		protocol.ArtifactProblemRunLost,
		protocol.ArtifactProblemAgentStuck,
		protocol.ArtifactProblemInvalidAPIKey,
		protocol.ArtifactProblemProviderRejected,
		protocol.ArtifactProblemDeniedByUser,
		protocol.ArtifactProblemToolFailed,
		protocol.ArtifactProblemChildRunCanceled,
		protocol.ArtifactProblemToolCanceled,
	}
	rules := make([]ConditionalRule, 0, len(withoutRetry))
	for _, kind := range withoutRetry {
		rules = append(rules, ConditionalRule{
			When:      []delivery.FieldCondition{{Field: "type", Operator: delivery.OperatorEquals, Value: string(kind)}},
			Forbidden: []string{"retryAfterSeconds"},
		})
	}
	return rules
}

// childLineageRules say the three child edges are all-or-none: a run either
// carries every one of them or is a root. Stated as one rule per edge
// rather than "root forbids them", because presence is the only thing a
// ConditionalRule can condition on and each edge is the condition for the other two.
//
// The contract's other half — that neither RunId equals the run's own id — is
// NOT here. JSON Schema cannot compare two fields, so the generated validators
// cannot enforce this rule uniformly. It is an identity invariant of the
// child-creation transaction. It is registered as a system invariant and proved
// by admission and durable-adapter fixtures; fusing an inequality into a
// conditional rule would be one primitive doing two jobs.
func childLineageRules() []ConditionalRule {
	edges := []string{"spawnedByItemId", "parentRunId", "rootRunId"}
	rules := make([]ConditionalRule, 0, len(edges))
	for index, edge := range edges {
		others := append(append([]string{}, edges[:index]...), edges[index+1:]...)
		rules = append(rules, ConditionalRule{
			When:     []delivery.FieldCondition{{Field: edge, Operator: delivery.OperatorPresent}},
			Required: others,
		})
	}
	return rules
}

func registerCarriedShapes(s *Shapes) {
	// `params._meta` is stripped before typed params are decoded, so the walk cannot
	// reach it, yet every client constructs it.
	s.carriedShape(CarriedSpec{Carrier: "params._meta", GoType: typeOf[protocol.RequestMeta]()})
}

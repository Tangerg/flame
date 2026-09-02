package dispatch

import (
	"reflect"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
	skilldomain "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/protocol"
)

// The value constraints of wire shapes.
//
// They were hand-written request checks or output assumptions, which meant the
// schema and runtime could disagree with no mechanical signal. Declared here, one
// statement generates the Go validator, JSON Schema and TypeScript validator.
//
// The kinds are the ones a JSON type does not already imply: a string that may not
// be empty, a number that may not be zero, an array that may not be sent empty, an
// array that may not repeat. Closed-enum membership is derived from the enum's own
// declared value set, not restated here.

func registerValueConstraints(s *Shapes) {
	registerCollectionValues(s)
	registerSessionValues(s)
	registerArtifactValues(s)
	registerRunValues(s)
	registerPlanValues(s)
	registerWorkspaceValues(s)
	registerUsageValues(s)
	registerSkillValues(s)
	registerHookValues(s)
	registerApprovalValues(s)
	registerMCPValues(s)
	registerProviderValues(s)
	registerModelValues(s)
	registerToolValues(s)
	registerKnowledgeValues(s)
	registerAuthoringContextValues(s)
	registerAgentMemoryValues(s)
	registerScheduleValues(s)
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

// nonEmpty builds the common spec: these fields are ids or text that must be there.
func nonEmpty[Request any](s *Shapes, fields ...string) {
	constraints := make([]FieldConstraint, 0, len(fields))
	for _, field := range fields {
		constraints = append(constraints, FieldConstraint{Field: field, Kind: ConstraintNonEmpty})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[Request](), Constraints: constraints})
}

// nonNegative builds the common accounting spec. Token counts, costs, step
// counts and durations are facts already consumed; a negative value is not an
// alternate representation of zero.
func nonNegative[Shape any](s *Shapes, fields ...string) {
	constraints := make([]FieldConstraint, 0, len(fields))
	for _, field := range fields {
		constraints = append(constraints, FieldConstraint{Field: field, Kind: ConstraintNonNegative})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[Shape](), Constraints: constraints})
}

func exactPositiveInteger(field string) []FieldConstraint {
	return []FieldConstraint{
		{Field: field, Kind: ConstraintPositive},
		{Field: field, Kind: ConstraintMaximum, Limit: protocol.MaximumExactJSONInteger},
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

func requiredScheduleIdentity(field string) []FieldConstraint {
	return append([]FieldConstraint{{Field: field, Kind: ConstraintNonEmpty}}, scheduleIdentity(field)...)
}

func scheduleIdentity(field string) []FieldConstraint {
	return append(boundedIdentity(field, protocol.MaximumResourceIdentityCharacters), FieldConstraint{
		Field: field, Kind: ConstraintPrefix, Value: protocol.IDPrefixSchedule,
	})
}

func modelSelectionIdentities(providerField, modelField, reasoningEffortField string) []FieldConstraint {
	constraints := boundedIdentity(providerField, protocol.MaximumProviderIdentityCharacters)
	constraints = append(constraints, boundedIdentity(modelField, protocol.MaximumModelIdentityCharacters)...)
	constraints = append(constraints, boundedIdentity(reasoningEffortField, protocol.MaximumReasoningEffortIdentityCharacters)...)
	return constraints
}

func modelUsageMapIdentities(field string) []FieldConstraint {
	return []FieldConstraint{
		{Field: field, Kind: ConstraintIdentityPropertyNames},
		{Field: field, Kind: ConstraintMaxPropertyNameLength, Limit: protocol.MaximumModelIdentityCharacters},
	}
}

func registerSessionValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Session](),
		Constraints: append(append(append(requiredResourceIdentity("id"),
			[]FieldConstraint{
				{Field: "provider", Kind: ConstraintNonEmpty},
				{Field: "model", Kind: ConstraintNonEmpty},
			}...), modelSelectionIdentities("provider", "model", "reasoningEffort")...), exactPositiveInteger("revision")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ListSessionsRequest](),
		Constraints: []FieldConstraint{{
			Field: "search", Kind: ConstraintMaxLength, Limit: session.MaximumCatalogSearchCharacters,
		}},
	})
	for _, request := range []reflect.Type{
		typeOf[protocol.GetSessionRequest](),
		typeOf[protocol.GetSessionSnapshotRequest](),
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

	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateSessionRequest](),
		Constraints: append(append(requiredResourceIdentity("sessionId"), []FieldConstraint{
			{Field: "expectedRevision", Kind: ConstraintPositive},
			{Field: "expectedRevision", Kind: ConstraintMaximum, Limit: protocol.MaximumExactJSONInteger},
			{Field: "provider", Kind: ConstraintNonEmpty},
			{Field: "model", Kind: ConstraintNonEmpty},
		}...), modelSelectionIdentities("provider", "model", "reasoningEffort")...),
	})

	// Import is RESTORE semantics — the session comes back under the id it was
	// exported with — so an artifact with no id names no session to restore.
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ImportSessionRequest](), Constraints: requiredResourceIdentity("artifact.session.id"),
	})
}

func registerArtifactValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ArtifactSession](),
		Constraints: append(append(requiredResourceIdentity("id"), []FieldConstraint{
			{Field: "provider", Kind: ConstraintNonEmpty},
			{Field: "model", Kind: ConstraintNonEmpty},
		}...), modelSelectionIdentities("provider", "model", "reasoningEffort")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ArtifactRun](),
		Constraints: append(append(append(append(append(requiredResourceIdentity("id"),
			requiredResourceIdentity("sessionId")...), resourceIdentity("spawnedByItemId")...),
			resourceIdentity("parentRunId")...), resourceIdentity("rootRunId")...),
			append(modelSelectionIdentities("provider", "model", "reasoningEffort"),
				FieldConstraint{Field: "messageMark", Kind: ConstraintNonNegative},
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
		GoType:      typeOf[protocol.ArtifactToolResult](),
		Constraints: requiredResourceIdentity("itemId"),
	})
	// Import accepts exactly the archive revision this development build emits.
	// Publishing the version as an unconstrained integer would make generated
	// clients promise support the runtime deliberately refuses.
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SessionArtifact](),
		Constraints: []FieldConstraint{
			{Field: "version", Kind: ConstraintMinimum, Limit: protocol.SessionArtifactVersion},
			{Field: "version", Kind: ConstraintMaximum, Limit: protocol.SessionArtifactVersion},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ArtifactRunMetrics](),
		Constraints: []FieldConstraint{
			{Field: "steps", Kind: ConstraintNonNegative},
			{Field: "activeDurationMillis", Kind: ConstraintNonNegative},
			{Field: "activeDurationMillis", Kind: ConstraintMaximum, Limit: protocol.MaximumDurationMilliseconds},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ArtifactUsage](),
		Constraints: append([]FieldConstraint{
			{Field: "inputTokens", Kind: ConstraintNonNegative},
			{Field: "outputTokens", Kind: ConstraintNonNegative},
			{Field: "cacheReadTokens", Kind: ConstraintNonNegative},
			{Field: "cacheWriteTokens", Kind: ConstraintNonNegative},
			{Field: "reasoningTokens", Kind: ConstraintNonNegative},
			{Field: "costUsd", Kind: ConstraintNonNegative},
		}, modelUsageMapIdentities("byModel")...),
	})
	nonNegative[protocol.ArtifactModelUsage](s,
		"inputTokens", "outputTokens", "cacheReadTokens", "cacheWriteTokens", "reasoningTokens", "costUsd",
	)
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ArtifactProblem](),
		Constraints: []FieldConstraint{
			{Field: "retryAfterSeconds", Kind: ConstraintPositive},
			{Field: "retryAfterSeconds", Kind: ConstraintMaximum, Limit: protocol.MaximumDurationSeconds},
		},
	})
}

func registerRunValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.ContentBlock](),
		Constraints: []FieldConstraint{{Field: "text", Kind: ConstraintPattern, Value: `\S`}},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.RunSummary](),
		Constraints: append(append(append(append(append(requiredResourceIdentity("id"),
			requiredResourceIdentity("sessionId")...), resourceIdentity("spawnedByItemId")...),
			resourceIdentity("parentRunId")...), resourceIdentity("rootRunId")...),
			modelSelectionIdentities("provider", "model", "reasoningEffort")...),
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
		GoType: typeOf[protocol.RunProtocolProfile](),
		Constraints: []FieldConstraint{
			{Field: "requiredFeatures", Kind: ConstraintUniqueItems},
			{Field: "interruptTypes", Kind: ConstraintUniqueItems},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.StartRunRequest](),
		Constraints: append(append(requiredResourceIdentity("sessionId"), []FieldConstraint{
			{Field: "input", Kind: ConstraintNonEmptyItems},
		}...), modelSelectionIdentities("provider", "model", "reasoningEffort")...),
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
	for _, limits := range []any{protocol.RunLimits{}, protocol.ArtifactRunLimits{}} {
		s.valueConstraint(FieldConstraintSpec{
			GoType: reflect.TypeOf(limits),
			Constraints: []FieldConstraint{
				{Field: "maxTotalTokens", Kind: ConstraintPositive},
				{Field: "maxSteps", Kind: ConstraintPositive},
				{Field: "maxBudgetUsd", Kind: ConstraintPositive},
			},
		})
	}
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
	for _, questionType := range []reflect.Type{
		typeOf[protocol.Question](),
		typeOf[protocol.ArtifactQuestion](),
	} {
		s.valueConstraint(FieldConstraintSpec{
			GoType: questionType,
			Constraints: []FieldConstraint{
				{Field: "fields", Kind: ConstraintNonEmptyItems},
				{Field: "fields", Kind: ConstraintMaxItems, Limit: transcript.MaximumQuestionFields},
			},
		})
	}
	for _, fieldType := range []reflect.Type{
		typeOf[protocol.QuestionField](),
		typeOf[protocol.ArtifactQuestionField](),
	} {
		s.valueConstraint(FieldConstraintSpec{
			GoType: fieldType,
			Constraints: []FieldConstraint{
				{Field: "prompt", Kind: ConstraintPattern, Value: `\S`},
				{Field: "header", Kind: ConstraintMaxLength, Limit: transcript.MaximumQuestionHeaderCharacters},
				{Field: "options", Kind: ConstraintMinItems, Limit: 2},
				{Field: "options", Kind: ConstraintMaxItems, Limit: transcript.MaximumQuestionOptions},
			},
		})
	}
	for _, optionType := range []reflect.Type{
		typeOf[protocol.QuestionOption](),
		typeOf[protocol.ArtifactQuestionOption](),
	} {
		s.valueConstraint(FieldConstraintSpec{
			GoType: optionType,
			Constraints: []FieldConstraint{
				{Field: "label", Kind: ConstraintPattern, Value: `\S`},
			},
		})
	}
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
	// Subscribe and steer both address a SEGMENT: naming only the run would let the
	// runtime pick whichever one is live, which is how a client silently ends up
	// folding — or steering — an execution it never saw.
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.SubscribeRunRequest](),
		Constraints: append(requiredResourceIdentity("runId"), requiredResourceIdentity("segmentId")...),
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.GetRunRequest](), Constraints: requiredResourceIdentity("runId")})
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
	// The scope is required and its tag decides everything else about the read, so a
	// scope with no tag is a request that never said what it wanted.
	nonEmpty[protocol.ListItemsRequest](s, "scope.type")
	// An omitted status filter already means "every status", so an empty array is
	// the one thing it cannot mean, and a repeat asks a set for something a set
	// does not have.
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
		GoType: typeOf[protocol.StreamEvent](),
		Constraints: append(resourceIdentity("itemId"),
			FieldConstraint{Field: "contextTokens", Kind: ConstraintNonNegative}),
	})
}

func registerPlanValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.GetPlanRequest](), Constraints: requiredResourceIdentity("sessionId")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.Plan](), Constraints: requiredResourceIdentity("sessionId")})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.PlanState](),
		Constraints: exactPositiveInteger("revision"),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.PlanStep](),
		Constraints: []FieldConstraint{
			{Field: "id", Kind: ConstraintNonEmpty},
			{Field: "description", Kind: ConstraintPattern, Value: `\S`},
		},
	})
}

func registerWorkspaceValues(s *Shapes) {
	nonEmpty[protocol.WorkspaceRef](s, "path")
	nonNegative[protocol.WorkspaceSummary](s, "sessionCount")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.FileContent](),
		Constraints: []FieldConstraint{
			{Field: "totalLines", Kind: ConstraintPositive},
			{Field: "startLine", Kind: ConstraintPositive},
			{Field: "endLine", Kind: ConstraintPositive},
		},
	})
	nonNegative[protocol.FileEntry](s, "sizeBytes")
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.FileLine](),
		Constraints: []FieldConstraint{{Field: "lineNumber", Kind: ConstraintPositive}},
	})
	nonNegative[protocol.GrepResult](s, "total")
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.GrepMatch](),
		Constraints: []FieldConstraint{{Field: "lineNumber", Kind: ConstraintPositive}},
	})
	nonNegative[protocol.FileDiff](s, "added", "removed")
	nonNegative[protocol.WorkspaceFileChange](s, "added", "removed")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.DiffRow](),
		Constraints: []FieldConstraint{
			{Field: "leftLine", Kind: ConstraintPositive},
			{Field: "rightLine", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GetDiffRequest](),
		Constraints: []FieldConstraint{
			{Field: "limit", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GetFileHeadRequest](),
		Constraints: []FieldConstraint{
			{Field: "path", Kind: ConstraintNonEmpty},
			{Field: "lines", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ReadFileRequest](),
		Constraints: []FieldConstraint{
			{Field: "path", Kind: ConstraintNonEmpty},
			{Field: "startLine", Kind: ConstraintPositive},
			{Field: "endLine", Kind: ConstraintPositive},
			{Field: "maxBytes", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GrepRequest](),
		Constraints: []FieldConstraint{
			{Field: "query", Kind: ConstraintNonEmpty},
			{Field: "limit", Kind: ConstraintPositive},
		},
	})
}

func registerUsageValues(s *Shapes) {
	nonNegative[protocol.UsageBucket](s, "runs")
	nonNegative[protocol.UsageSummary](s, "sessions", "runs")
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SessionUsageRequest](), Constraints: requiredResourceIdentity("sessionId")})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.UsageSummaryRequest](),
		Constraints: []FieldConstraint{{Field: "sinceDays", Kind: ConstraintPositive}},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.FeedbackRequest](),
		Constraints: append(append(resourceIdentity("sessionId"), resourceIdentity("runId")...), resourceIdentity("itemId")...),
	})
}

func registerSkillValues(s *Shapes) {
	const nonBlankText = `\S`
	skillName := []FieldConstraint{{Field: "name", Kind: ConstraintPattern, Value: nonBlankText}}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SkillNameRequest](), Constraints: skillName})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.Skill](), Constraints: skillName})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ManagedSkill](), Constraints: skillName})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SkillProposalRef](),
		Constraints: append(skillName,
			FieldConstraint{Field: "revision", Kind: ConstraintPattern, Value: skilldomain.ProposalRevisionPattern},
		),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SkillProposal](),
		Constraints: []FieldConstraint{
			{Field: "name", Kind: ConstraintPattern, Value: nonBlankText},
			{Field: "revision", Kind: ConstraintPattern, Value: skilldomain.ProposalRevisionPattern},
			{Field: "description", Kind: ConstraintPattern, Value: nonBlankText},
			{Field: "instructions", Kind: ConstraintPattern, Value: nonBlankText},
		},
	})
}

func registerHookValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.HookInfo](),
		Constraints: []FieldConstraint{
			{Field: "source", Kind: ConstraintNonEmpty},
			{Field: "timeoutMillis", Kind: ConstraintNonNegative},
			{Field: "timeoutMillis", Kind: ConstraintMaximum, Limit: hooks.MaxTimeoutMillis},
		},
	})
	nonEmpty[protocol.SetHookTrustRequest](s, "projectRoot")
}

func registerApprovalValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ListApprovalRulesRequest](), Constraints: requiredResourceIdentity("sessionId")})
	nonEmpty[protocol.ForgetApprovalRuleRequest](s, "id")
}

func registerMCPValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPServerState](),
		Constraints: []FieldConstraint{
			{Field: "toolCount", Kind: ConstraintNonNegative},
			{Field: "toolCount", Kind: ConstraintMaximum, Limit: mcpserver.MaxRemoteToolsPerServer},
		},
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.MCPServerRequest](), Constraints: mcpServerIdentity("server")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.CreateMCPAuthorizationAttemptRequest](), Constraints: mcpServerIdentity("server")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.MCPListToolsRequest](), Constraints: mcpServerIdentity("server")})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPServer](),
		Constraints: append(append(mcpServerIdentity("name"), mcpRemoteToolItems("disabledTools")...),
			mcpRemoteToolItems("autoApproveTools")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.MCPTool](),
		Constraints: append(mcpServerIdentity("server"), mcpRemoteToolIdentity("name")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPAuthorizationAttemptRequest](),
		Constraints: []FieldConstraint{{
			Field: "attemptId", Kind: ConstraintPattern, Value: mcpapp.AuthorizationAttemptIDPattern,
		}},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPAuthorizationAttempt](),
		Constraints: append([]FieldConstraint{
			{Field: "id", Kind: ConstraintPattern, Value: mcpapp.AuthorizationAttemptIDPattern},
		}, mcpServerIdentity("server")...),
	})
	nonEmpty[protocol.MCPConnection](s, "url", "command")
	nonEmpty[protocol.MCPConnectionInput](s, "url", "command")
	nonEmpty[protocol.MCPAuthorizationChange](s, "value")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPHandshakeTimeout](),
		Constraints: []FieldConstraint{
			{Field: "seconds", Kind: ConstraintPositive},
			{Field: "seconds", Kind: ConstraintMaximum, Limit: protocol.MaximumDurationSeconds},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPHeadersChange](),
		Constraints: []FieldConstraint{
			{Field: "value", Kind: ConstraintNonEmptyProperties},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPEnvironmentChange](),
		Constraints: []FieldConstraint{
			{Field: "value", Kind: ConstraintNonEmptyProperties},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPServerCandidate](),
		Constraints: append(append(mcpServerIdentity("name"), mcpRemoteToolItems("disabledTools")...),
			mcpRemoteToolItems("autoApproveTools")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateMCPServerRequest](),
		Constraints: append(append(mcpServerIdentity("server"), mcpRemoteToolItems("disabledTools")...),
			mcpRemoteToolItems("autoApproveTools")...),
	})
}

func mcpServerIdentity(field string) []FieldConstraint {
	return []FieldConstraint{
		{Field: field, Kind: ConstraintMaxLength, Limit: mcpserver.MaximumServerNameCharacters},
		{Field: field, Kind: ConstraintPattern, Value: mcpserver.ServerNamePattern},
	}
}

func mcpRemoteToolIdentity(field string) []FieldConstraint {
	return []FieldConstraint{
		{Field: field, Kind: ConstraintMaxLength, Limit: mcpserver.MaximumRemoteToolNameCharacters},
		{Field: field, Kind: ConstraintPattern, Value: mcpserver.RemoteToolNamePattern},
	}
}

func mcpRemoteToolItems(field string) []FieldConstraint {
	return []FieldConstraint{
		{Field: field, Kind: ConstraintMaxItems, Limit: mcpserver.MaxRemoteToolsPerServer},
		{Field: field, Kind: ConstraintUniqueItems},
		{Field: field, Kind: ConstraintMaxItemLength, Limit: mcpserver.MaximumRemoteToolNameCharacters},
		{Field: field, Kind: ConstraintPatternItems, Value: mcpserver.RemoteToolNamePattern},
	}
}

func registerProviderValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Provider](),
		Constraints: append(append([]FieldConstraint{{Field: "id", Kind: ConstraintNonEmpty}},
			boundedIdentity("id", protocol.MaximumProviderIdentityCharacters)...),
			boundedIdentity("defaultEmbeddingModel", protocol.MaximumModelIdentityCharacters)...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateProviderRequest](),
		Constraints: append([]FieldConstraint{{Field: "provider", Kind: ConstraintNonEmpty}},
			boundedIdentity("provider", protocol.MaximumProviderIdentityCharacters)...),
	})
	nonEmpty[protocol.ProviderConfigChange](s, "value")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.TestProviderRequest](),
		Constraints: append([]FieldConstraint{{Field: "provider", Kind: ConstraintNonEmpty}},
			boundedIdentity("provider", protocol.MaximumProviderIdentityCharacters)...),
	})
}

func registerModelValues(s *Shapes) {
	nonNegative[protocol.ModelPricing](s,
		"inputUsdPerMillionTokens", "outputUsdPerMillionTokens",
		"cacheReadUsdPerMillionTokens", "cacheWriteUsdPerMillionTokens",
	)
	for _, roleType := range []reflect.Type{
		typeOf[protocol.UtilityRole](),
		typeOf[protocol.EmbeddingRole](),
	} {
		s.valueConstraint(FieldConstraintSpec{
			GoType: roleType,
			Constraints: append(
				boundedIdentity("provider", protocol.MaximumProviderIdentityCharacters),
				boundedIdentity("model", protocol.MaximumModelIdentityCharacters)...,
			),
		})
	}
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.ListModelsRequest](),
		Constraints: boundedIdentity("provider", protocol.MaximumProviderIdentityCharacters),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Model](),
		Constraints: append(append([]FieldConstraint{
			{Field: "id", Kind: ConstraintNonEmpty},
			{Field: "provider", Kind: ConstraintNonEmpty},
		}, boundedIdentity("id", protocol.MaximumModelIdentityCharacters)...),
			boundedIdentity("provider", protocol.MaximumProviderIdentityCharacters)...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ModelCapabilities](),
		Constraints: append([]FieldConstraint{
			{Field: "reasoningLevels", Kind: ConstraintIdentityItems},
			{Field: "reasoningLevels", Kind: ConstraintMaxItemLength, Limit: protocol.MaximumReasoningEffortIdentityCharacters},
		}, boundedIdentity("reasoningDefaultLevel", protocol.MaximumReasoningEffortIdentityCharacters)...),
	})
}

func registerToolValues(s *Shapes) {
	nonEmpty[protocol.InvokeToolRequest](s, "name")
}

func registerKnowledgeValues(s *Shapes) {
	nonEmpty[protocol.KnowledgeEntry](s, "revision")
	nonEmpty[protocol.UpdateKnowledgeRequest](s, "expectedRevision")
}

func registerAuthoringContextValues(s *Shapes) {
	const nonBlankText = `\S`
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.AgentDoc](),
		Constraints: []FieldConstraint{{Field: "path", Kind: ConstraintPattern, Value: nonBlankText}},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Recipe](),
		Constraints: []FieldConstraint{
			{Field: "name", Kind: ConstraintPattern, Value: nonBlankText},
			{Field: "body", Kind: ConstraintPattern, Value: nonBlankText},
			{Field: "source", Kind: ConstraintPattern, Value: nonBlankText},
		},
	})
}

func registerAgentMemoryValues(s *Shapes) {
	const nonBlankContent = `\S`
	agentMemoryItemIdentity := func(field string) []FieldConstraint {
		return []FieldConstraint{
			{Field: field, Kind: ConstraintNonEmpty},
			{Field: field, Kind: ConstraintMaxLength, Limit: int64(agentmemory.MaximumItemIDCharacters)},
			{Field: field, Kind: ConstraintPattern, Value: agentmemory.ItemIDPattern},
		}
	}
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.AgentMemoryItemRequest](),
		Constraints: agentMemoryItemIdentity("id"),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.AgentMemoryReviewRequest](),
		Constraints: agentMemoryItemIdentity("id"),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.AgentMemoryItem](),
		Constraints: append(append(agentMemoryItemIdentity("id"), resourceIdentity("sessionId")...), []FieldConstraint{
			{Field: "content", Kind: ConstraintPattern, Value: nonBlankContent},
			{Field: "content", Kind: ConstraintMaxLength, Limit: agentmemory.MaxContentCharacters},
		}...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.AgentMemoryUpdateRequest](),
		Constraints: append(agentMemoryItemIdentity("id"), []FieldConstraint{
			{Field: "content", Kind: ConstraintPattern, Value: nonBlankContent},
			{Field: "content", Kind: ConstraintMaxLength, Limit: agentmemory.MaxContentCharacters},
		}...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.AgentMemoryAddRequest](),
		Constraints: []FieldConstraint{
			{Field: "content", Kind: ConstraintPattern, Value: nonBlankContent},
			{Field: "content", Kind: ConstraintMaxLength, Limit: agentmemory.MaxContentCharacters},
		},
	})
}

func registerScheduleValues(s *Shapes) {
	const nonBlankInstructions = `\S`
	scheduleConstraints := append(requiredScheduleIdentity("id"), exactPositiveInteger("revision")...)
	scheduleConstraints = append(scheduleConstraints,
		FieldConstraint{Field: "instructions", Kind: ConstraintPattern, Value: nonBlankInstructions})
	scheduleConstraints = append(scheduleConstraints,
		modelSelectionIdentities("provider", "model", "reasoningEffort")...)
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.Schedule](),
		Constraints: scheduleConstraints,
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.CreateScheduleRequest](),
		Constraints: append([]FieldConstraint{
			{Field: "instructions", Kind: ConstraintPattern, Value: nonBlankInstructions},
			{Field: "cron", Kind: ConstraintNonEmpty},
		}, modelSelectionIdentities("provider", "model", "reasoningEffort")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateScheduleRequest](),
		Constraints: append(append(requiredScheduleIdentity("id"), []FieldConstraint{
			{Field: "expectedRevision", Kind: ConstraintPositive},
			{Field: "expectedRevision", Kind: ConstraintMaximum, Limit: protocol.MaximumExactJSONInteger},
			{Field: "instructions", Kind: ConstraintPattern, Value: nonBlankInstructions},
			{Field: "cron", Kind: ConstraintNonEmpty},
		}...), modelSelectionIdentities("provider", "model", "reasoningEffort")...),
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.DeleteScheduleRequest](), Constraints: requiredScheduleIdentity("id")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.RunScheduleNowRequest](), Constraints: requiredScheduleIdentity("id")})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.RunScheduleNowResponse](),
		Constraints: append(requiredResourceIdentity("sessionId"), requiredResourceIdentity("runId")...),
	})
}

func registerGoalValues(s *Shapes) {
	const nonBlankObjective = `\S`
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Goal](),
		Constraints: append(append(requiredResourceIdentity("sessionId"), []FieldConstraint{
			{Field: "objective", Kind: ConstraintPattern, Value: nonBlankObjective},
			{Field: "provider", Kind: ConstraintNonEmpty},
			{Field: "model", Kind: ConstraintNonEmpty},
		}...), modelSelectionIdentities("provider", "model", "reasoningEffort")...),
	})
	nonNegative[protocol.GoalUsage](s, "runs", "costUsd", "steps")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.StartGoalRequest](),
		Constraints: append(append(requiredResourceIdentity("sessionId"), []FieldConstraint{
			{Field: "objective", Kind: ConstraintPattern, Value: nonBlankObjective},
		}...), modelSelectionIdentities("provider", "model", "reasoningEffort")...),
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.GoalRequest](), Constraints: requiredResourceIdentity("sessionId")})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateGoalRequest](),
		Constraints: append(requiredResourceIdentity("sessionId"),
			FieldConstraint{Field: "objective", Kind: ConstraintPattern, Value: nonBlankObjective}),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GoalBudget](),
		Constraints: []FieldConstraint{
			{Field: "maxRuns", Kind: ConstraintPositive},
			{Field: "maxCostUsd", Kind: ConstraintPositive},
			{Field: "maxSteps", Kind: ConstraintPositive},
		},
	})
}

func registerRuntimeValues(s *Shapes) {
	nonEmpty[protocol.ClientInfo](s, "name", "version")
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

	// A subscription names a set. Absence and an empty set both describe no stream,
	// while duplicates claim a set distinction that does not exist.
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.RuntimeSubscribeRequest](),
		Constraints: []FieldConstraint{
			{Field: "topics", Kind: ConstraintNonEmptyItems},
			{Field: "topics", Kind: ConstraintUniqueItems},
		},
	})

	// Sequence zero is the sentinel before the hub assigns a frame. Every array is
	// a narrowing set: when present it names at least one unique resource. Variant
	// registration separately decides which sets are required or forbidden.
	eventConstraints := []FieldConstraint{
		{Field: "sequence", Kind: ConstraintPositive},
		{Field: "sequence", Kind: ConstraintMaximum, Limit: protocol.MaximumRuntimeEventSequence},
	}
	for _, field := range []string{
		"paths", "names", "serverIds", "scheduleIds", "sessionIds", "runIds",
		"topics", "watchIds",
	} {
		eventConstraints = append(eventConstraints,
			FieldConstraint{Field: field, Kind: ConstraintNonEmptyItems},
			FieldConstraint{Field: field, Kind: ConstraintUniqueItems},
		)
	}
	eventConstraints = append(eventConstraints,
		FieldConstraint{Field: "scheduleIds", Kind: ConstraintIdentityItems},
		FieldConstraint{Field: "scheduleIds", Kind: ConstraintMaxItemLength, Limit: protocol.MaximumResourceIdentityCharacters},
		FieldConstraint{Field: "scheduleIds", Kind: ConstraintPrefixItems, Value: protocol.IDPrefixSchedule},
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
	// A set is owned by its root, while every Interrupt names the Run that raised
	// it. Empty ids satisfy JSON's string type but identify neither resource, so
	// both the live segment outcome and cold interrupt read must reject them.
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.Interrupt](),
		Constraints: append(requiredResourceIdentity("itemId"), requiredResourceIdentity("runId")...),
	})
	// Structured problems are useful only when their leaves identify something.
	// Register the leaf types as validation roots too, so ValidateWireTree applies
	// their string and enum constraints when they are nested in ProblemData.
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
			{Field: "namespace", Kind: ConstraintPattern, Value: runtimeidentity.IdempotencyNamespacePattern},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SubscriptionLimits](),
		Constraints: []FieldConstraint{
			{Field: "maxTopics", Kind: ConstraintPositive},
			{Field: "maxWatches", Kind: ConstraintPositive},
		},
	})
}

package dispatch

import (
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	skilldomain "github.com/Tangerg/flame/runtime/internal/domain/skills"
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

func registerSessionValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Session](),
		Constraints: append(append(append(requiredResourceIdentity("id"),
			[]FieldConstraint{
				{Field: "provider", Kind: ConstraintNonEmpty},
				{Field: "model", Kind: ConstraintNonEmpty},
			}...), modelSelectionIdentities()...), exactPositiveInteger("revision")...),
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
		GoType: typeOf[protocol.ArtifactProblem](),
		Constraints: []FieldConstraint{
			{Field: "retryAfterSeconds", Kind: ConstraintPositive},
			{Field: "retryAfterSeconds", Kind: ConstraintMaximum, Limit: protocol.MaximumDurationSeconds},
		},
	})
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
		GoType: typeOf[protocol.RunProtocolProfile](),
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
			{Field: "fields", Kind: ConstraintMaxItems, Limit: transcript.MaximumQuestionFields},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.QuestionField](),
		Constraints: []FieldConstraint{
			{Field: "prompt", Kind: ConstraintPattern, Value: `\S`},
			{Field: "header", Kind: ConstraintMaxLength, Limit: transcript.MaximumQuestionHeaderCharacters},
			{Field: "options", Kind: ConstraintMinItems, Limit: 2},
			{Field: "options", Kind: ConstraintMaxItems, Limit: transcript.MaximumQuestionOptions},
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
	// Subscribe and steer both address a SEGMENT: naming only the run would let the
	// runtime pick whichever one is live, which is how a client silently ends up
	// folding — or steering — an execution it never saw.
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
	// The run identity is present only for the opened and retired variants; the
	// union owns that requiredness, so these are identity-when-present guards.
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
		GoType:      typeOf[protocol.StreamEvent](),
		Constraints: resourceIdentity("itemId"),
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
	nonEmpty[protocol.WorkspaceInfo](s, "projectRoot")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.WorkspaceSummary](),
		Constraints: []FieldConstraint{
			{Field: "name", Kind: ConstraintNonEmpty},
			{Field: "sessionCount", Kind: ConstraintNonNegative},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.FileContent](),
		Constraints: []FieldConstraint{
			{Field: "totalLines", Kind: ConstraintPositive},
			{Field: "startLine", Kind: ConstraintPositive},
			{Field: "endLine", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.FileEntry](),
		Constraints: []FieldConstraint{
			{Field: "path", Kind: ConstraintNonEmpty},
			{Field: "name", Kind: ConstraintNonEmpty},
			{Field: "sizeBytes", Kind: ConstraintNonNegative},
		},
	})
	nonNegative[protocol.GrepResult](s, "total")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GrepMatch](),
		Constraints: []FieldConstraint{
			{Field: "path", Kind: ConstraintNonEmpty},
			{Field: "lineNumber", Kind: ConstraintPositive},
		},
	})
	workspaceChangeConstraints := []FieldConstraint{
		{Field: "path", Kind: ConstraintNonEmpty},
		{Field: "added", Kind: ConstraintNonNegative},
		{Field: "removed", Kind: ConstraintNonNegative},
	}
	for _, shape := range []reflect.Type{
		typeOf[protocol.FileDiff](),
		typeOf[protocol.WorkspaceFileChange](),
	} {
		s.valueConstraint(FieldConstraintSpec{GoType: shape, Constraints: workspaceChangeConstraints})
	}
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.DiffBaseline](),
		Constraints: []FieldConstraint{{Field: "commit", Kind: ConstraintPattern, Value: `^([0-9a-f]{40}|[0-9a-f]{64})$`}},
	})
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

func registerFeedbackValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.FeedbackEntry](),
		Constraints: append(append(resourceIdentity("sessionId"), resourceIdentity("runId")...), resourceIdentity("itemId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.FeedbackRequest](),
		Constraints: append(
			append(append(resourceIdentity("sessionId"), resourceIdentity("runId")...), resourceIdentity("itemId")...),
			FieldConstraint{Field: "text", Kind: ConstraintPattern, Value: `\S`},
		),
	})
}

func registerSkillValues(s *Shapes) {
	const nonBlankText = `\S`
	skillName := []FieldConstraint{{Field: "name", Kind: ConstraintPattern, Value: nonBlankText}}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SkillNameRequest](), Constraints: skillName})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SkillDetailRequest](), Constraints: skillName})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SkillDiagnostic](), Constraints: []FieldConstraint{{Field: "name", Kind: ConstraintPattern, Value: nonBlankText}, {Field: "detail", Kind: ConstraintNonEmpty}}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SkillDetail](), Constraints: []FieldConstraint{
		{Field: "path", Kind: ConstraintNonEmpty}, {Field: "revision", Kind: ConstraintPattern, Value: `^[a-f0-9]{64}$`},
	}})
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
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ToolRef](), Constraints: append(mcpRemoteToolIdentity("name"), FieldConstraint{Field: "endpoint", Kind: ConstraintPattern, Value: tool.ModelNamePattern()})})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SetApprovalRuleRequest](), Constraints: resourceIdentity("sessionId")})

	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ApprovalRule](),
		Constraints: append(requiredResourceIdentity("id"),
			FieldConstraint{Field: "modelName", Kind: ConstraintPattern, Value: tool.ModelNamePattern()}),
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ListApprovalRulesRequest](), Constraints: resourceIdentity("sessionId")})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ListApprovalRulesResult](),
		Constraints: []FieldConstraint{{
			Field: "rules", Kind: ConstraintMaxItems, Limit: approval.MaximumVisibleRules,
		}},
	})
	nonEmpty[protocol.ForgetApprovalRuleRequest](s, "id")
}

func registerMCPValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.MCPOrigin](),
		Constraints: []FieldConstraint{{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()}},
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.MCPServerID](), Constraints: mcpServerName("name")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.MCPToolExposure](), Constraints: mcpRemoteToolItems("disabledTools")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SetMCPToolExposureRequest](), Constraints: mcpRemoteToolIdentity("name")})

	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPServerState](),
		Constraints: []FieldConstraint{
			{Field: "toolCount", Kind: ConstraintNonNegative},
			{Field: "toolCount", Kind: ConstraintMaximum, Limit: mcpserver.MaxRemoteToolsPerServer},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.MCPTool](),
		Constraints: append(mcpRemoteToolIdentity("name"), FieldConstraint{Field: "modelName", Kind: ConstraintPattern, Value: tool.ModelNamePattern()}),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPAuthorizationAttemptRequest](),
		Constraints: []FieldConstraint{{
			Field: "attemptId", Kind: ConstraintPattern, Value: mcpapp.AuthorizationAttemptIDPattern(),
		}},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPAuthorizationAttempt](),
		Constraints: []FieldConstraint{
			{Field: "id", Kind: ConstraintPattern, Value: mcpapp.AuthorizationAttemptIDPattern()},
		},
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
		GoType:      typeOf[protocol.MCPServerCandidate](),
		Constraints: mcpServerName("name"),
	})
}

func mcpServerName(field string) []FieldConstraint {
	return []FieldConstraint{{Field: field, Kind: ConstraintPattern, Value: mcpserver.ServerNamePattern()}}
}

func mcpRemoteToolIdentity(field string) []FieldConstraint {
	return []FieldConstraint{{Field: field, Kind: ConstraintPattern, Value: mcpserver.RemoteToolNamePattern()}}
}

func mcpRemoteToolItems(field string) []FieldConstraint {
	return []FieldConstraint{
		{Field: field, Kind: ConstraintMaxItems, Limit: mcpserver.MaxRemoteToolsPerServer},
		{Field: field, Kind: ConstraintUniqueItems},
		{Field: field, Kind: ConstraintPatternItems, Value: mcpserver.RemoteToolNamePattern()},
	}
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

func registerToolValues(s *Shapes) {
	nonEmpty[protocol.InvokeToolRequest](s, "name")
}

func registerAuthoringContextValues(s *Shapes) {
	const nonBlankText = `\S`
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.AgentDoc](),
		Constraints: []FieldConstraint{{Field: "path", Kind: ConstraintPattern, Value: nonBlankText}},
	})
}

func registerGoalValues(s *Shapes) {
	const nonBlankObjective = `\S`
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Goal](),
		Constraints: append(append(requiredResourceIdentity("sessionId"), []FieldConstraint{
			{Field: "objective", Kind: ConstraintPattern, Value: nonBlankObjective},
		}...), modelSelectionIdentities()...),
	})
	nonNegative[protocol.GoalUsage](s, "runs", "costUsd", "steps")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.StartGoalRequest](),
		Constraints: append(append(requiredResourceIdentity("sessionId"), []FieldConstraint{
			{Field: "objective", Kind: ConstraintPattern, Value: nonBlankObjective},
		}...), modelSelectionIdentities()...),
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.GoalRequest](), Constraints: requiredResourceIdentity("sessionId")})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateGoalRequest](),
		Constraints: append(requiredResourceIdentity("sessionId"),
			FieldConstraint{Field: "objective", Kind: ConstraintPattern, Value: nonBlankObjective}),
	})
}

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

	// A subscription names a set. Absence and an empty set both describe no stream,
	// while duplicates claim a set distinction that does not exist.
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

	// Sequence zero is the sentinel before the hub assigns a frame. Every array is
	// a narrowing set: when present it names at least one unique resource. Variant
	// registration separately decides which sets are required or forbidden.
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
	// A set is owned by its root, while every Interrupt names the Run that raised
	// it. Empty ids satisfy JSON's string type but identify neither resource, so
	// both the live segment.finished frame and cold interrupt read must reject them.
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

func registerTrajectoryValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.UsageCoverage](), Constraints: []FieldConstraint{
		{Field: "calls", Kind: ConstraintNonNegative}, {Field: "reportedCalls", Kind: ConstraintNonNegative}, {Field: "pricedCalls", Kind: ConstraintNonNegative}, {Field: "startedCalls", Kind: ConstraintNonNegative}, {Field: "completedCalls", Kind: ConstraintNonNegative}, {Field: "failedCalls", Kind: ConstraintNonNegative}, {Field: "unknownCalls", Kind: ConstraintNonNegative},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.RecordedUsage](), Constraints: []FieldConstraint{{Field: "recordedCostUsd", Kind: ConstraintNonNegative}}})

	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ListModelInvocationsRequest](), Constraints: requiredResourceIdentity("runId")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ListModelEndpointsRequest](), Constraints: []FieldConstraint{
		{Field: "parentCallId", Kind: ConstraintNonEmpty}, {Field: "parentCallId", Kind: ConstraintIdentity}, {Field: "parentCallId", Kind: ConstraintMaxLength, Limit: 256},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ModelEndpointInvocation](), Constraints: append(append(resourceIdentity("sessionId"), resourceIdentity("runId")...), []FieldConstraint{
		{Field: "costUSD", Kind: ConstraintNonNegative},
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

func registerPluginValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginClientComponent](), Constraints: []FieldConstraint{
		{Field: "id", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
		{Field: "contribution", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
		{Field: "material", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.OpenPluginResourcesRequest](), Constraints: []FieldConstraint{{Field: "componentId", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern}}})
	for _, shape := range []reflect.Type{typeOf[protocol.PluginPoint](), typeOf[protocol.PluginPointValue](), typeOf[protocol.ResolvePluginPointRequest]()} {
		constraints := []FieldConstraint{{Field: "contract", Kind: ConstraintNonEmpty}}
		if shape != typeOf[protocol.ResolvePluginPointRequest]() {
			constraints = append(constraints, FieldConstraint{Field: "id", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern})
		}
		if shape == typeOf[protocol.PluginPointValue]() {
			constraints = append(constraints, FieldConstraint{Field: "value", Kind: ConstraintNonEmpty})
		}
		if shape == typeOf[protocol.ResolvePluginPointRequest]() {
			constraints = append(constraints, FieldConstraint{Field: "attributes", Kind: ConstraintNonEmpty})
		}
		s.valueConstraint(FieldConstraintSpec{GoType: shape, Constraints: constraints})
	}
	for _, shape := range []reflect.Type{typeOf[protocol.PluginPointReference](), typeOf[protocol.PluginPointBinding]()} {
		constraints := []FieldConstraint{
			{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()},
			{Field: "contribution", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
		}
		if shape == typeOf[protocol.PluginPointBinding]() {
			constraints = append(constraints, FieldConstraint{Field: "digest", Kind: ConstraintPattern, Value: identity.DigestPattern()}, FieldConstraint{Field: "value", Kind: ConstraintNonEmpty})
		}
		s.valueConstraint(FieldConstraintSpec{GoType: shape, Constraints: constraints})
	}
	for _, shape := range []reflect.Type{typeOf[protocol.ResolvePluginRequirementsRequest](), typeOf[protocol.PluginOperationBinding]()} {
		constraints := []FieldConstraint{
			{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()},
			{Field: "digest", Kind: ConstraintPattern, Value: identity.DigestPattern()},
			{Field: "contribution", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
		}
		if shape == typeOf[protocol.PluginOperationBinding]() {
			constraints = append(constraints, FieldConstraint{Field: "contract", Kind: ConstraintNonEmpty})
		}
		s.valueConstraint(FieldConstraintSpec{GoType: shape, Constraints: constraints})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginChangeNotice](), Constraints: []FieldConstraint{{Field: "contribution", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern}}})
	for _, shape := range []reflect.Type{typeOf[protocol.PluginRequirement](), typeOf[protocol.PluginCollaborator]()} {
		constraints := []FieldConstraint{{Field: "id", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern}}
		if shape == typeOf[protocol.PluginRequirement]() {
			constraints = append(constraints, FieldConstraint{Field: "contract", Kind: ConstraintNonEmpty})
		}
		s.valueConstraint(FieldConstraintSpec{GoType: shape, Constraints: constraints})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.CallPluginRequest](), Constraints: []FieldConstraint{
		{Field: "contract", Kind: ConstraintNonEmpty},
		{Field: "input", Kind: ConstraintNonEmpty},
		{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()},
		{Field: "digest", Kind: ConstraintPattern, Value: identity.DigestPattern()},
		{Field: "contribution", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
		{Field: "method", Kind: ConstraintNonEmpty},
		{Field: "method", Kind: ConstraintMaxLength, Limit: 256},
		{Field: "method", Kind: ConstraintIdentity},
		{Field: "target.scope", Kind: ConstraintNonEmpty},
		{Field: "target.scope", Kind: ConstraintIdentity},
		{Field: "target.scope", Kind: ConstraintMaxLength, Limit: 256},
		{Field: "target.reference", Kind: ConstraintNonEmpty},
		{Field: "target.reference", Kind: ConstraintPattern, Value: `^[^\p{Cc}]*$`},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginCallFailure](), Constraints: []FieldConstraint{
		{Field: "code", Kind: ConstraintNonEmpty},
		{Field: "code", Kind: ConstraintMaxLength, Limit: 256},
		{Field: "message", Kind: ConstraintNonEmpty},
	}})
	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.CallPluginResponse](), Rules: []ConditionalRule{
		{RequiredAny: []string{"output", "failure"}},
		{When: []delivery.FieldCondition{{Field: "failure", Operator: delivery.OperatorPresent}}, Forbidden: []string{"output"}},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.CallPluginResponse](), Constraints: []FieldConstraint{{Field: "output", Kind: ConstraintNonEmpty}}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginStreamEvent](), Constraints: []FieldConstraint{{Field: "value", Kind: ConstraintNonEmpty}}})
	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.PluginStreamEvent](), Rules: []ConditionalRule{
		{When: []delivery.FieldCondition{{Field: "kind", Operator: delivery.OperatorEquals, Value: string(protocol.PluginStreamChunk)}}, Required: []string{"value"}, Forbidden: []string{"failure"}},
		{When: []delivery.FieldCondition{{Field: "kind", Operator: delivery.OperatorEquals, Value: string(protocol.PluginStreamResult)}}, Required: []string{"value"}, Forbidden: []string{"failure"}},
		{When: []delivery.FieldCondition{{Field: "kind", Operator: delivery.OperatorEquals, Value: string(protocol.PluginStreamFailure)}}, Required: []string{"failure"}, Forbidden: []string{"value"}},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginContribution](), Constraints: append(mcpServerName("server"), FieldConstraint{Field: "id", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern}, FieldConstraint{Field: "export", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern}, FieldConstraint{Field: "contract", Kind: ConstraintNonEmpty})})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginRendererSelector](), Constraints: []FieldConstraint{{Field: "name", Kind: ConstraintNonEmpty}, {Field: "name", Kind: ConstraintMaxLength, Limit: plugin.MaxRendererSelectorNameBytes}}})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.PluginThemeColors](),
		Constraints: []FieldConstraint{
			{Field: "background", Kind: ConstraintPattern, Value: plugin.ThemeColorPattern()},
			{Field: "foreground", Kind: ConstraintPattern, Value: plugin.ThemeColorPattern()},
			{Field: "accent", Kind: ConstraintPattern, Value: plugin.ThemeColorPattern()},
			{Field: "muted", Kind: ConstraintPattern, Value: plugin.ThemeColorPattern()},
			{Field: "border", Kind: ConstraintPattern, Value: plugin.ThemeColorPattern()},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.PluginRealization](),
		Constraints: []FieldConstraint{
			{Field: "unavailableBackends", Kind: ConstraintMinItems, Limit: 1},
			{Field: "unavailableBackends", Kind: ConstraintUniqueItems},
			{Field: "unavailableBackends", Kind: ConstraintPatternItems, Value: mcpserver.ServerNamePattern()},
		},
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginServerDeclaration](), Constraints: mcpServerName("name")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginInput](), Constraints: mcpServerName("server")})
	for _, shape := range []reflect.Type{typeOf[protocol.SelectPluginProviderRequest](), typeOf[protocol.PluginProviderSelection]()} {
		constraints := []FieldConstraint{
			FieldConstraint{Field: "contribution", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
			FieldConstraint{Field: "role", Kind: ConstraintNonEmpty},
			FieldConstraint{Field: "role", Kind: ConstraintMaxLength, Limit: 256},
			FieldConstraint{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()},
		}
		s.valueConstraint(FieldConstraintSpec{GoType: shape, Constraints: constraints})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginCompositionMember](), Constraints: []FieldConstraint{
		{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()},
		{Field: "contribution", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
	}})
	for _, shape := range []reflect.Type{typeOf[protocol.PluginComposition](), typeOf[protocol.SetPluginCompositionRequest](), typeOf[protocol.ResolvePluginCompositionRequest]()} {
		constraints := []FieldConstraint{{Field: "contract", Kind: ConstraintNonEmpty}}
		if shape != typeOf[protocol.ResolvePluginCompositionRequest]() {
			constraints = append(constraints, FieldConstraint{Field: "members", Kind: ConstraintUniqueItems})
		}
		s.valueConstraint(FieldConstraintSpec{GoType: shape, Constraints: constraints})
	}
	for _, shape := range []reflect.Type{typeOf[protocol.StagePluginRequest](), typeOf[protocol.PluginRequest](), typeOf[protocol.PluginReleaseRequest](), typeOf[protocol.SetPluginEnablementRequest](), typeOf[protocol.ConfigurePluginRequest]()} {
		constraints := []FieldConstraint{{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()}}
		switch shape {
		case typeOf[protocol.PluginReleaseRequest](), typeOf[protocol.ConfigurePluginRequest]():
			constraints = append(constraints, FieldConstraint{Field: "digest", Kind: ConstraintPattern, Value: identity.DigestPattern()})
		}
		switch shape {
		case typeOf[protocol.StagePluginRequest]():
			constraints = append(constraints, FieldConstraint{Field: "source", Kind: ConstraintNonEmpty}, FieldConstraint{Field: "source", Kind: ConstraintMaxLength, Limit: 4096})
		}
		s.valueConstraint(FieldConstraintSpec{GoType: shape, Constraints: constraints})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.InstallPluginRequest](), Constraints: []FieldConstraint{{Field: "source", Kind: ConstraintNonEmpty}, {Field: "source", Kind: ConstraintMaxLength, Limit: 4096}}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginValueChange](), Constraints: []FieldConstraint{{Field: "value", Kind: ConstraintMaxLength, Limit: 8192}}})
}

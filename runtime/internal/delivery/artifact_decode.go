package delivery

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/scope/core/chat"
)

func invalidArtifact(path, format string, args ...any) error {
	detail := fmt.Sprintf(format, args...)
	return protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("%s: %s", path, detail))
}

func decodeIndexed[W, T any](prefix string, encoded []W, decode func(path string, value W) (T, error)) ([]T, error) {
	values := make([]T, 0, len(encoded))
	for index, value := range encoded {
		decoded, err := decode(fmt.Sprintf("%s[%d]", prefix, index), value)
		if err != nil {
			return nil, err
		}
		values = append(values, decoded)
	}
	return values, nil
}

func portableToolResultFromArtifact(sessionID identity.SessionID, path string, encoded protocol.ArtifactToolResult) (sessions.PortableToolResult, error) {
	id, err := transcript.ParseToolResultID(encoded.ID)
	if err != nil {
		return sessions.PortableToolResult{}, invalidArtifact(path+".id", "%v", err)
	}
	itemID, err := identity.ParseItem(encoded.ItemID)
	if err != nil {
		return sessions.PortableToolResult{}, invalidArtifact(path+".itemId", "%v", err)
	}
	return sessions.PortableToolResult{
		ItemID: itemID, ModelOnly: encoded.ModelOnly,
		Blob: transcript.ToolResultBlob{ID: id, SessionID: sessionID, Body: encoded.Body, CreatedAt: encoded.CreatedAt},
	}, nil
}

func portableArtifactFromWire(art protocol.SessionArtifact) (sessions.PortableSnapshot, error) {
	if err := protocol.ValidateWireTree(art); err != nil {
		return sessions.PortableSnapshot{}, invalidArtifact("artifact", "%v", err)
	}
	sessionID, err := identity.ParseSession(art.Session.ID)
	if err != nil {
		return sessions.PortableSnapshot{}, invalidArtifact("artifact.session.id", "%v", err)
	}
	selection, err := flamemodel.NewWithReasoningEffort(art.Session.Provider, art.Session.Model, art.Session.ReasoningEffort)
	if err == nil {
		err = selection.ValidateExact()
	}
	if err != nil {
		return sessions.PortableSnapshot{}, invalidArtifact(
			"artifact.session", "invalid model selection: %v", err,
		)
	}
	messages, err := decodeIndexed("artifact.messages", art.Messages, portableMessageFromArtifact)
	if err != nil {
		return sessions.PortableSnapshot{}, err
	}
	runs, err := decodeIndexed("artifact.runs", art.Runs, portableRunFromArtifact)
	if err != nil {
		return sessions.PortableSnapshot{}, err
	}
	items, err := decodeIndexed("artifact.items", art.Items, func(path string, encoded protocol.ArtifactItem) (transcript.Item, error) {
		return portableItemFromArtifact(sessionID, path, encoded)
	})
	if err != nil {
		return sessions.PortableSnapshot{}, err
	}
	toolResults, err := decodeIndexed("artifact.toolResults", art.ToolResults, func(path string, encoded protocol.ArtifactToolResult) (sessions.PortableToolResult, error) {
		return portableToolResultFromArtifact(sessionID, path, encoded)
	})
	if err != nil {
		return sessions.PortableSnapshot{}, err
	}
	var source extension.ContributionSource
	if art.PlanSource != nil {
		source = *art.PlanSource
		if err := source.Validate(); err != nil || art.Plan == nil {
			return sessions.PortableSnapshot{}, invalidArtifact("artifact.planSource", "a valid author and captured plan are required")
		}
	}
	plan, err := portablePlanFromArtifact(art.Plan)
	if err != nil {
		return sessions.PortableSnapshot{}, err
	}
	return sessions.PortableSnapshot{
		Session: sessions.PortableSession{
			ID: sessionID, Title: art.Session.Title, CWD: art.Session.Workspace.Path, Selection: selection,
			CreatedAt: art.Session.CreatedAt, UpdatedAt: art.Session.UpdatedAt, Favorite: art.Session.Favorite,
		},
		Messages: messages, Runs: runs, Items: items, ToolResults: toolResults, Plan: plan, PlanSource: source,
	}, nil
}

// Messages ride RawMessage, so the strict request decoder cannot see fields
// Scope's custom codecs would ignore; comparing canonical JSON before and after
// the typed decode closes that hole.
func portableMessageFromArtifact(path string, encoded jsontext.Value) (chat.Message, error) {
	var message chat.Message
	if err := json.Unmarshal(encoded, &message); err != nil {
		return chat.Message{}, invalidArtifact(path, "%v", err)
	}
	reencoded, err := json.Marshal(message, json.Deterministic(true))
	if err != nil {
		return chat.Message{}, invalidArtifact(path, "re-encode: %v", err)
	}
	input := jsontext.Value(bytes.Clone(encoded))
	if err := input.Canonicalize(); err != nil {
		return chat.Message{}, invalidArtifact(path, "canonicalize input: %v", err)
	}
	output := jsontext.Value(reencoded)
	if err := output.Canonicalize(); err != nil {
		return chat.Message{}, invalidArtifact(path, "canonicalize decoded value: %v", err)
	}
	if !bytes.Equal(input, output) {
		return chat.Message{}, invalidArtifact(path, "contains fields or values unsupported by artifact version %d", protocol.SessionArtifactVersion)
	}
	return message, nil
}

func portablePlanFromArtifact(entries []protocol.PlanStep) ([]plan.Step, error) {
	if entries == nil {
		return nil, nil
	}
	steps := make([]plan.Step, 0, len(entries))
	for index, entry := range entries {
		status, known := planStatusFromWire(entry.Status)
		if !known {
			return nil, invalidArtifact(fmt.Sprintf("artifact.plan[%d].status", index),
				"unknown value %q", entry.Status)
		}
		steps = append(steps, plan.Step{
			Description: entry.Description, Status: status,
		})
	}
	return steps, nil
}

func planStatusFromWire(status protocol.PlanStatus) (plan.Status, bool) {
	switch status {
	case protocol.PlanStatusPending:
		return plan.StatusPending, true
	case protocol.PlanStatusInProgress:
		return plan.StatusInProgress, true
	case protocol.PlanStatusCompleted:
		return plan.StatusCompleted, true
	default:
		return plan.StatusPending, false
	}
}

func portableRunFromArtifact(path string, artifact protocol.ArtifactRun) (sessions.PortableRun, error) {
	ending, err := portableEndingFromArtifact(path+".outcome", artifact.Outcome)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	effects, err := portableUnresolvedEffectsFromArtifact(path+".outcome.unresolvedEffects", artifact.Outcome.UnresolvedEffects)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	capabilities, err := portableCapabilitiesFromArtifact(path+".capabilities", artifact.Capabilities)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	metrics, err := portableMetricsFromArtifact(path+".metrics", artifact.Metrics)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	selection, err := flamemodel.NewWithReasoningEffort(artifact.Provider, artifact.Model, artifact.ReasoningEffort)
	if err != nil {
		return sessions.PortableRun{}, invalidArtifact(path, "invalid model selection: %v", err)
	}
	id, err := identity.ParseRun(artifact.ID)
	if err != nil {
		return sessions.PortableRun{}, invalidArtifact(path+".id", "%v", err)
	}
	parentRunID, err := identity.ParseOptionalRun(artifact.ParentRunID)
	if err != nil {
		return sessions.PortableRun{}, invalidArtifact(path+".parentRunId", "%v", err)
	}
	rootRunID, err := identity.ParseOptionalRun(artifact.RootRunID)
	if err != nil {
		return sessions.PortableRun{}, invalidArtifact(path+".rootRunId", "%v", err)
	}
	sessionID, err := identity.ParseSession(artifact.SessionID)
	if err != nil {
		return sessions.PortableRun{}, invalidArtifact(path+".sessionId", "%v", err)
	}
	spawnedByItemID, err := identity.ParseOptionalItem(artifact.SpawnedByItemID)
	if err != nil {
		return sessions.PortableRun{}, invalidArtifact(path+".spawnedByItemId", "%v", err)
	}
	return sessions.PortableRun{
		SessionID: sessionID, ID: id, SpawnedByItemID: spawnedByItemID,
		ParentRunID: parentRunID, RootRunID: rootRunID,
		Selection: selection, Ending: ending,
		UnresolvedEffects: effects,
		Metrics:           metrics,
		ContextTokens:     artifact.ContextTokens,
		Capabilities:      capabilities,
		CreatedAt:         artifact.CreatedAt, FinishedAt: artifact.FinishedAt,
		UpdatedAt: artifact.UpdatedAt, MessageMark: portableMessageMark(artifact.MessageMark),
	}, nil
}

func portableUnresolvedEffectsFromArtifact(path string, values []protocol.UnresolvedEffect) ([]run.UnresolvedEffect, error) {
	effects := make([]run.UnresolvedEffect, 0, len(values))
	for index, value := range values {
		var output *toolcontract.Output
		if len(value.Output) > 0 {
			output = new(toolcontract.Output)
			if err := json.Unmarshal([]byte(value.Output), output); err != nil {
				return nil, invalidArtifact(fmt.Sprintf("%s[%d].output", path, index), "%v", err)
			}
		}
		cause, known := unresolvedCauses.domain(value.Cause)
		if !known {
			return nil, invalidArtifact(fmt.Sprintf("%s[%d].cause", path, index), "unknown value %q", value.Cause)
		}
		effect, err := run.NewUnresolvedEffect(run.UnresolvedEffectConfig{
			ProcessID: value.ProcessID, EffectID: value.EffectID, Cause: cause, Reason: value.Reason, Detail: value.Detail, Output: output,
		})
		if err != nil {
			return nil, invalidArtifact(fmt.Sprintf("%s[%d]", path, index), "%v", err)
		}
		effects = append(effects, effect)
	}
	return effects, nil
}

func portableCapabilitiesFromArtifact(path string, wire *protocol.RunCapabilities) (*run.Capabilities, error) {
	if wire == nil {
		return nil, nil
	}
	var out run.Capabilities
	for _, feature := range wire.RequiredFeatures {
		switch feature {
		case protocol.RunCapabilityFeatureChildRuns:
			out.ChildRuns = true
		default:
			return nil, invalidArtifact(path+".requiredFeatures", "unknown value %q", feature)
		}
	}
	for _, declared := range wire.InterruptTypes {
		interruptType, backed := interruptTypeFromWire(declared)
		if !backed {
			return nil, invalidArtifact(path+".interruptTypes", "unknown value %q", declared)
		}
		out.InterruptTypes = append(out.InterruptTypes, interruptType)
	}
	normalized := out.Normalized()
	return &normalized, nil
}

func portableEndingFromArtifact(path string, outcome protocol.ArtifactOutcome) (run.Ending, error) {
	switch outcome.Type {
	case protocol.ArtifactOutcomeCompleted:
		return run.Completed{}, nil
	case protocol.ArtifactOutcomeCanceled:
		return run.Canceled{Detail: outcome.Detail}, nil
	case protocol.ArtifactOutcomeTimedOut:
		if outcome.Error == nil {
			return nil, invalidArtifact(path+".error", "is required")
		}
		return run.TimedOut{Detail: outcome.Error.Detail}, nil
	case protocol.ArtifactOutcomeFailed:
		failure, err := portableRunFailureFromArtifact(path+".error", outcome.Error)
		if err != nil {
			return nil, err
		}
		if failure == nil {
			return nil, invalidArtifact(path+".error", "is required")
		}
		return run.Failed{Failure: *failure}, nil
	case protocol.ArtifactOutcomeLost:
		if outcome.Loss == nil {
			return nil, invalidArtifact(path+".loss", "is required")
		}
		reason, known := lossReasons.domain(outcome.Loss.Reason)
		if !known {
			return nil, invalidArtifact(path+".loss.reason", "unknown value %q", outcome.Loss.Reason)
		}
		return run.Lost{Loss: run.Loss{Reason: reason, Detail: outcome.Loss.Detail}}, nil
	default:
		return nil, invalidArtifact(path+".type", "unknown value %q", outcome.Type)
	}
}

func portableMetricsFromArtifact(path string, artifact protocol.RunMetrics) (run.Metrics, error) {
	usage, err := portableUsageFromArtifact(artifact.Usage)
	if err != nil {
		return run.Metrics{}, invalidArtifact(path+".usage", "%v", err)
	}
	metrics, err := run.NewMetrics(
		usage,
		artifact.Steps,
		time.Duration(artifact.ActiveDurationMillis)*time.Millisecond,
	)
	if err != nil {
		return run.Metrics{}, invalidArtifact(path, "%v", err)
	}
	return metrics, nil
}

func portableUsageFromArtifact(artifact *protocol.Usage) (*accounting.Usage, error) {
	if artifact == nil {
		return nil, nil
	}
	cost, err := accounting.CostFromOptional(artifact.CostUSD)
	if err != nil {
		return nil, err
	}
	total := accounting.Totals{Tokens: accounting.Tokens{
		InputTokens: artifact.InputTokens, OutputTokens: artifact.OutputTokens,
		CacheReadTokens: artifact.CacheReadTokens, CacheWriteTokens: artifact.CacheWriteTokens,
		ReasoningTokens: artifact.ReasoningTokens,
	}, Cost: cost}
	var byModel map[string]accounting.Totals
	if len(artifact.ByModel) != 0 {
		byModel = make(map[string]accounting.Totals, len(artifact.ByModel))
		for model, values := range artifact.ByModel {
			cost, err := accounting.CostFromOptional(values.CostUSD)
			if err != nil {
				return nil, fmt.Errorf("model %q: %w", model, err)
			}
			byModel[model] = accounting.Totals{Tokens: accounting.Tokens{
				InputTokens: values.InputTokens, OutputTokens: values.OutputTokens,
				CacheReadTokens: values.CacheReadTokens, CacheWriteTokens: values.CacheWriteTokens,
				ReasoningTokens: values.ReasoningTokens,
			}, Cost: cost}
		}
	}
	usage, err := accounting.NewUsage(total, byModel)
	if err != nil {
		return nil, err
	}
	return &usage, nil
}

func portableItemFromArtifact(sessionID identity.SessionID, path string, artifact protocol.ArtifactItem) (transcript.Item, error) {
	status, err := portableItemStatus(path+".status", artifact.Status)
	if err != nil {
		return transcript.Item{}, err
	}
	kind, err := portableItemKind(path+".type", artifact.Type)
	if err != nil {
		return transcript.Item{}, err
	}
	runID, err := identity.ParseOptionalRun(artifact.RunID)
	if err != nil {
		return transcript.Item{}, invalidArtifact(path+".runId", "%v", err)
	}
	itemID, err := identity.ParseItem(artifact.ID)
	if err != nil {
		return transcript.Item{}, invalidArtifact(path+".id", "%v", err)
	}
	snapshot := transcript.ItemSnapshot{
		Identity: transcript.ItemIdentity{
			SessionID: sessionID, ItemID: itemID, RunID: runID,
			OccurredAt: artifact.CreatedAt,
		},
		Status: status,
	}
	body, err := portableItemBody(kind, status, path, artifact)
	if err != nil {
		return transcript.Item{}, err
	}
	snapshot.Body = body
	if kind == transcript.ToolCall {
		snapshot.Identity.OccurredAt = artifact.StartedAt
	}
	item, err := transcript.RestoreItem(snapshot)
	if err != nil {
		return transcript.Item{}, invalidArtifact(path, "%v", err)
	}
	return item, nil
}

func portableItemBody(kind transcript.ItemKind, status transcript.ItemStatus, path string, artifact protocol.ArtifactItem) (transcript.Body, error) {
	var content []transcript.ContentBlock
	for index, block := range artifact.Content {
		decoded, err := portableContentFromArtifact(fmt.Sprintf("%s.content[%d]", path, index), block)
		if err != nil {
			return nil, err
		}
		content = append(content, decoded)
	}
	switch kind {
	case transcript.UserMessage:
		return transcript.UserMessageBody{Content: content}, nil
	case transcript.AgentMessage:
		return transcript.AgentMessageBody{Phase: portableMessagePhase(artifact.Phase), Content: content}, nil
	case transcript.Reasoning:
		return transcript.ReasoningBody{Text: artifact.Text, Redacted: artifact.Redacted}, nil
	case transcript.Compaction:
		return transcript.CompactionBody{Summary: artifact.Summary, DroppedMessages: artifact.DroppedMessages}, nil
	case transcript.QuestionItem:
		if artifact.Question == nil {
			return nil, invalidArtifact(path+".question", "is required")
		}
		question, err := portableQuestionFromArtifact(path+".question", *artifact.Question)
		if err != nil {
			return nil, err
		}
		return transcript.QuestionBody{Question: question}, nil
	case transcript.ToolCall:
		return portableToolCallFromArtifact(path, status, artifact)
	default:
		return nil, nil
	}
}

func portableToolCallFromArtifact(path string, status transcript.ItemStatus, artifact protocol.ArtifactItem) (transcript.ToolCallBody, error) {
	if artifact.Tool == nil {
		return transcript.ToolCallBody{}, invalidArtifact(path+".tool", "is required")
	}
	invocation, err := portableToolFromArtifact(path+".tool", *artifact.Tool)
	if err != nil {
		return transcript.ToolCallBody{}, err
	}
	failure, err := portableToolFailureFromArtifact(path+".error", artifact.Error)
	if err != nil {
		return transcript.ToolCallBody{}, err
	}
	safetyClass, err := portableSafetyClass(path+".safetyClass", artifact.SafetyClass)
	if err != nil {
		return transcript.ToolCallBody{}, err
	}
	call := transcript.ToolCallBody{Invocation: invocation, SafetyClass: safetyClass, Failure: failure}
	if decision := portableItemApprovalDecision(artifact.ApprovalDecision); decision != "" {
		call.Approval = &approval.Resolution{Decision: decision}
	}
	if status != transcript.ItemRunning {
		if artifact.DurationMillis != nil {
			call.ExecutionDuration = new(time.Duration(*artifact.DurationMillis) * time.Millisecond)
		}
		call.FinishedAt = artifact.FinishedAt
	}
	return call, nil
}

func portableMessagePhase(phase protocol.MessagePhase) transcript.MessagePhase {
	switch phase {
	case protocol.MessagePhaseCommentary:
		return transcript.MessageCommentary
	case protocol.MessagePhaseFinalAnswer:
		return transcript.MessageFinalAnswer
	default:
		return transcript.MessagePhaseNone
	}
}

func portableItemApprovalDecision(decision protocol.ApprovalDecision) approval.Decision {
	switch decision {
	case protocol.ApprovalApprove:
		return approval.Allow
	case protocol.ApprovalDeny:
		return approval.Deny
	default:
		return ""
	}
}

func portableItemStatus(path string, value protocol.ItemStatus) (transcript.ItemStatus, error) {
	switch value {
	case protocol.ItemStatusRunning:
		return transcript.ItemRunning, nil
	case protocol.ItemStatusCompleted:
		return transcript.ItemCompleted, nil
	case protocol.ItemStatusIncomplete:
		return transcript.ItemIncomplete, nil
	default:
		return "", invalidArtifact(path, "unknown value %q", value)
	}
}

func portableItemKind(path string, value protocol.ItemType) (transcript.ItemKind, error) {
	kind, known := itemKinds.domain(value)
	if !known {
		return "", invalidArtifact(path, "unknown value %q", value)
	}
	return kind, nil
}

func portableContentFromArtifact(path string, artifact protocol.ContentBlock) (transcript.ContentBlock, error) {
	decoded, decodeErr := decodeContent(encodedContent{
		kind: artifact.Type, text: artifact.Text, mime: artifact.Mime, data: artifact.Data,
	})
	if decodeErr != nil {
		return transcript.ContentBlock{}, invalidArtifact(path+"."+decodeErr.field, "%s", decodeErr.detail)
	}
	return decoded, nil
}

func portableQuestionFromArtifact(path string, artifact protocol.Question) (transcript.Question, error) {
	fields := make([]transcript.QuestionField, len(artifact.Fields))
	for index, field := range artifact.Fields {
		kind, err := portableQuestionFieldKind(fmt.Sprintf("%s.fields[%d].type", path, index), field.Type)
		if err != nil {
			return transcript.Question{}, err
		}
		var options []transcript.QuestionOption
		if len(field.Options) > 0 {
			options = make([]transcript.QuestionOption, len(field.Options))
			for optionIndex, option := range field.Options {
				options[optionIndex] = transcript.QuestionOption{Label: option.Label, Description: option.Description, Preview: option.Preview}
			}
		}
		fields[index] = transcript.QuestionField{
			Prompt: field.Prompt, Header: field.Header, Kind: kind,
			Options: options, Multiple: field.Multiple, AllowCustom: field.AllowCustom,
		}
	}
	return transcript.Question{Fields: fields, Answers: transcript.CloneAnswers(artifact.Answers)}, nil
}

func portableQuestionFieldKind(path string, value protocol.QuestionFieldType) (questioncontract.Kind, error) {
	switch value {
	case protocol.QuestionFieldText:
		return questioncontract.Text, nil
	case protocol.QuestionFieldChoice:
		return questioncontract.Choice, nil
	default:
		return "", invalidArtifact(path, "unknown value %q", value)
	}
}

func portableSafetyClass(path string, value protocol.SafetyClass) (tool.SafetyClass, error) {
	switch value {
	case "":
		return "", nil
	case protocol.SafetyClassSafe:
		return tool.SafetyClassSafe, nil
	case protocol.SafetyClassWrite:
		return tool.SafetyClassWrite, nil
	case protocol.SafetyClassExec:
		return tool.SafetyClassExec, nil
	case protocol.SafetyClassNetwork:
		return tool.SafetyClassNetwork, nil
	default:
		return "", invalidArtifact(path, "unknown value %q", value)
	}
}

func portableToolFromArtifact(path string, artifact protocol.ToolInvocation) (transcript.ToolInvocation, error) {
	arguments, err := tool.ArgumentsFromMap(artifact.Arguments)
	if err != nil {
		return transcript.ToolInvocation{}, invalidArtifact(path+".arguments", "%v", err)
	}
	var result *tool.Result
	if artifact.Result != nil {
		value, err := tool.NewResult(artifact.Result)
		if err != nil {
			return transcript.ToolInvocation{}, invalidArtifact(path+".result", "%v", err)
		}
		result = &value
	}
	input := transcript.AdmittedInput(arguments)
	if artifact.ArgumentsText != "" {
		if !arguments.Equal(tool.Arguments{}) {
			return transcript.ToolInvocation{}, invalidArtifact(path+".arguments", "must be empty beside rejected argumentsText")
		}
		input = transcript.RejectedInput(artifact.ArgumentsText)
	}
	var source tool.Ref
	if artifact.Source != nil {
		source, err = toolRefFromWire(artifact.Source.Ref)
		if err != nil {
			return transcript.ToolInvocation{}, invalidArtifact(path+".source.ref", "%v", err)
		}
		if source.ModelName() != artifact.Source.ModelName || !source.MatchesPublishedName(artifact.Name) {
			return transcript.ToolInvocation{}, invalidArtifact(path+".source", "tool name does not match its captured source")
		}
	}
	return transcript.ToolInvocation{Name: artifact.Name, Source: source, Input: input, Result: result}, nil
}

func portableRunFailureFromArtifact(path string, artifact *protocol.ArtifactProblem) (*run.Failure, error) {
	if artifact == nil {
		return nil, nil
	}
	kind, known := runFailureKinds.domain(artifact.Type)
	if !known {
		return nil, invalidArtifact(path+".type", "unknown value %q", artifact.Type)
	}
	retryAfter, err := run.RetryAfterFromSeconds(artifact.RetryAfterSeconds)
	if err != nil {
		return nil, invalidArtifact(path+".retryAfterSeconds", "%v", err)
	}
	return &run.Failure{
		Kind: kind, Detail: artifact.Detail, DocURL: artifact.DocURL,
		RetryAfter: retryAfter,
	}, nil
}

func portableToolFailureFromArtifact(path string, artifact *protocol.ArtifactProblem) (*tool.Failure, error) {
	if artifact == nil {
		return nil, nil
	}
	kind, known := toolFailureKinds.domain(artifact.Type)
	if !known {
		return nil, invalidArtifact(path+".type", "unknown value %q", artifact.Type)
	}
	return &tool.Failure{
		Kind: kind, Detail: artifact.Detail, DocURL: artifact.DocURL,
	}, nil
}

func portableMessageMark(mark *int) run.MessageMark {
	if mark == nil {
		return run.UnknownMessageMark()
	}
	return run.MessageMarkAt(*mark)
}

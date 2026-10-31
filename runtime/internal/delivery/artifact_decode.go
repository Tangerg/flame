package delivery

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
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

// invalidArtifact is the protocol adapter's structural-document error. Semantic
// aggregate validation is deliberately performed by sessions.RestorePortableSession.
func invalidArtifact(path, format string, args ...any) error {
	detail := fmt.Sprintf(format, args...)
	return protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("%s: %s", path, detail))
}

// portableArtifactFromWire performs only protocol decoding and enum mapping.
// Aggregate ownership, union rules, references, terminal boundaries, and tool
// result bindings are validated by sessions.RestorePortableSession.
func portableArtifactFromWire(art protocol.SessionArtifact) (sessions.PortableSnapshot, error) {
	if err := protocol.ValidateWireTree(art); err != nil {
		return sessions.PortableSnapshot{}, invalidArtifact("artifact", "%v", err)
	}
	selection, err := modelchoice.NewWithReasoningEffort(art.Session.Provider, art.Session.Model, art.Session.ReasoningEffort)
	if err == nil {
		err = selection.ValidateExact()
	}
	if err != nil {
		return sessions.PortableSnapshot{}, invalidArtifact(
			"artifact.session", "invalid model selection: %v", err,
		)
	}
	messages := make([]chat.Message, 0, len(art.Messages))
	for index, encoded := range art.Messages {
		path := fmt.Sprintf("artifact.messages[%d]", index)
		message, decodeErr := portableMessageFromArtifact(path, encoded)
		if decodeErr != nil {
			return sessions.PortableSnapshot{}, decodeErr
		}
		messages = append(messages, message)
	}
	runs := make([]sessions.PortableRun, 0, len(art.Runs))
	for index, encoded := range art.Runs {
		run, portableRunFromArtifactErr := portableRunFromArtifact(fmt.Sprintf("artifact.runs[%d]", index), encoded)
		if portableRunFromArtifactErr != nil {
			return sessions.PortableSnapshot{}, portableRunFromArtifactErr
		}
		runs = append(runs, run)
	}
	items := make([]transcript.Item, 0, len(art.Items))
	for index, encoded := range art.Items {
		item, portableItemFromArtifactErr := portableItemFromArtifact(art.Session.ID, fmt.Sprintf("artifact.items[%d]", index), encoded)
		if portableItemFromArtifactErr != nil {
			return sessions.PortableSnapshot{}, portableItemFromArtifactErr
		}
		items = append(items, item)
	}
	toolResults := make([]sessions.PortableToolResult, 0, len(art.ToolResults))
	for index, encoded := range art.ToolResults {
		id, parseIDErr := transcript.ParseToolResultID(encoded.ID)
		if parseIDErr != nil {
			return sessions.PortableSnapshot{}, invalidArtifact(fmt.Sprintf("artifact.toolResults[%d].id", index), "%v", parseIDErr)
		}
		toolResults = append(toolResults, sessions.PortableToolResult{
			ItemID: encoded.ItemID,
			Blob: transcript.ToolResultBlob{
				ID: id, SessionID: art.Session.ID, Body: encoded.Body, CreatedAt: encoded.CreatedAt,
			},
		})
	}
	plan, err := portablePlanFromArtifact(art.Plan)
	if err != nil {
		return sessions.PortableSnapshot{}, err
	}
	return sessions.PortableSnapshot{
		Session: sessions.PortableSession{
			ID: art.Session.ID, Title: art.Session.Title, CWD: art.Session.Workspace.Path, Selection: selection,
			CreatedAt: art.Session.CreatedAt, UpdatedAt: art.Session.UpdatedAt, Favorite: art.Session.Favorite,
		},
		Messages: messages, Runs: runs, Items: items, ToolResults: toolResults, Plan: plan,
	}, nil
}

// portableMessageFromArtifact accepts only the exact current Scope message
// document. SessionArtifact.Messages uses RawMessage to keep the public
// protocol independent of Scope types, so the outer strict request decoder
// cannot see fields that Scope's custom JSON codecs would ignore. Comparing
// canonical JSON before and after the typed decode closes that escape hatch
// without treating object order, whitespace, or equivalent number spellings as
// contract differences. Open metadata and tool-result details survive the
// round trip and therefore remain valid.
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

// portablePlanFromArtifact reads the archived Plan. The artifact carries the one
// product value directly, so there is no key union or duplicate-key precedence rule.
func portablePlanFromArtifact(entries []protocol.PlanStep) ([]plan.Step, error) {
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

// planStatusFromWire maps the archived status back. It is total over the wire enum
// so an unknown value is refused instead of silently importing as pending — a task
// list whose statuses changed on import is a different plan.
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
	capabilities, err := portableCapabilitiesFromArtifact(path+".protocolProfile", artifact.ProtocolProfile)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	metrics, err := portableMetricsFromArtifact(path+".metrics", artifact.Metrics)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	selection, err := modelchoice.NewWithReasoningEffort(artifact.Provider, artifact.Model, artifact.ReasoningEffort)
	if err != nil {
		return sessions.PortableRun{}, invalidArtifact(path, "invalid model selection: %v", err)
	}
	return sessions.PortableRun{
		SessionID: artifact.SessionID, ID: artifact.ID, SpawnedByItemID: artifact.SpawnedByItemID,
		ParentRunID: artifact.ParentRunID, RootRunID: artifact.RootRunID,
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
		var output *chat.ToolOutput
		if len(value.Output) > 0 {
			output = new(chat.ToolOutput)
			if err := json.Unmarshal([]byte(value.Output), output); err != nil {
				return nil, invalidArtifact(fmt.Sprintf("%s[%d].output", path, index), "%v", err)
			}
		}
		effect, err := run.NewUnresolvedEffect(run.UnresolvedEffectConfig{
			ProcessID: value.ProcessID, EffectID: value.EffectID, Cause: value.Cause, Reason: value.Reason, Detail: value.Detail, Output: output,
		})
		if err != nil {
			return nil, invalidArtifact(fmt.Sprintf("%s[%d]", path, index), "%v", err)
		}
		effects = append(effects, effect)
	}
	return effects, nil
}

// portableCapabilitiesFromArtifact restores the run's frozen contract, or nothing when
// the archive carried none — which only a child may do, and the aggregate check
// enforces. An interrupt type this runtime cannot raise is refused rather than
// dropped: importing the run without it would silently rewrite the contract the
// archive recorded.
func portableCapabilitiesFromArtifact(path string, profile *protocol.RunProtocolProfile) (*run.Capabilities, error) {
	if profile == nil {
		return nil, nil
	}
	var out run.Capabilities
	for _, feature := range profile.RequiredFeatures {
		switch feature {
		case protocol.RunProtocolFeatureChildRuns:
			out.ChildRuns = true
		default:
			return nil, invalidArtifact(path+".requiredFeatures", "unknown value %q", feature)
		}
	}
	for _, declared := range profile.InterruptTypes {
		kind, backed := interruptTypeFromWire(declared)
		if !backed {
			return nil, invalidArtifact(path+".interruptTypes", "unknown value %q", declared)
		}
		out.InterruptTypes = append(out.InterruptTypes, kind)
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
		return run.Lost{Loss: run.Loss{Reason: run.LossReason(outcome.Loss.Reason), Detail: outcome.Loss.Detail}}, nil
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
	out := &accounting.Usage{Total: accounting.Totals{Tokens: accounting.Tokens{
		InputTokens: artifact.InputTokens, OutputTokens: artifact.OutputTokens,
		CacheReadTokens: artifact.CacheReadTokens, CacheWriteTokens: artifact.CacheWriteTokens,
		ReasoningTokens: artifact.ReasoningTokens,
	}, Cost: cost}}
	if len(artifact.ByModel) != 0 {
		out.ByModel = make(map[string]accounting.Totals, len(artifact.ByModel))
		for model, values := range artifact.ByModel {
			cost, err := accounting.CostFromOptional(values.CostUSD)
			if err != nil {
				return nil, fmt.Errorf("model %q: %w", model, err)
			}
			out.ByModel[model] = accounting.Totals{Tokens: accounting.Tokens{
				InputTokens: values.InputTokens, OutputTokens: values.OutputTokens,
				CacheReadTokens: values.CacheReadTokens, CacheWriteTokens: values.CacheWriteTokens,
				ReasoningTokens: values.ReasoningTokens,
			}, Cost: cost}
		}
	}
	return out, nil
}

func portableItemFromArtifact(sessionID, path string, artifact protocol.ArtifactItem) (transcript.Item, error) {
	status, err := portableItemStatus(path+".status", artifact.Status)
	if err != nil {
		return transcript.Item{}, err
	}
	kind, err := portableItemKind(path+".type", artifact.Type)
	if err != nil {
		return transcript.Item{}, err
	}
	snapshot := transcript.ItemSnapshot{
		Identity: transcript.ItemIdentity{
			SessionID: sessionID, ItemID: artifact.ID, RunID: artifact.RunID,
			OccurredAt: artifact.CreatedAt,
		},
		Status: status,
	}
	var content []transcript.ContentBlock
	for index, block := range artifact.Content {
		decoded, err := portableContentFromArtifact(fmt.Sprintf("%s.content[%d]", path, index), block)
		if err != nil {
			return transcript.Item{}, err
		}
		content = append(content, decoded)
	}
	switch kind {
	case transcript.UserMessage:
		snapshot.Body = transcript.UserMessageBody{Content: content}
	case transcript.AgentMessage:
		snapshot.Body = transcript.AgentMessageBody{Phase: portableMessagePhase(artifact.Phase), Content: content}
	case transcript.Reasoning:
		snapshot.Body = transcript.ReasoningBody{Text: artifact.Text, Redacted: artifact.Redacted}
	case transcript.Compaction:
		snapshot.Body = transcript.CompactionBody{Summary: artifact.Summary, DroppedMessages: artifact.DroppedMessages}
	case transcript.QuestionItem:
		if artifact.Question == nil {
			return transcript.Item{}, invalidArtifact(path+".question", "is required")
		}
		question, err := portableQuestionFromArtifact(path+".question", *artifact.Question)
		if err != nil {
			return transcript.Item{}, err
		}
		snapshot.Body = transcript.QuestionBody{Question: question}
	case transcript.ToolCall:
		call, err := portableToolCallFromArtifact(path, status, artifact)
		if err != nil {
			return transcript.Item{}, err
		}
		snapshot.Identity.OccurredAt = artifact.StartedAt
		snapshot.Body = call
	}
	item, err := transcript.RestoreItem(snapshot)
	if err != nil {
		return transcript.Item{}, invalidArtifact(path, "%v", err)
	}
	return item, nil
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
	kind := transcript.ItemKind(value)
	if !kind.Valid() {
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
	return transcript.ToolInvocation{Name: artifact.Name, Input: input, Result: result}, nil
}

func portableRunFailureFromArtifact(path string, artifact *protocol.ArtifactProblem) (*run.Failure, error) {
	if artifact == nil {
		return nil, nil
	}
	kind := run.FailureKind(artifact.Type)
	if !kind.Valid() {
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
	kind := tool.FailureKind(artifact.Type)
	if !kind.Valid() {
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

package delivery

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/protocol"
)

// artifactFromPortable maps the terminal archive projection to the versioned
// protocol document. Tool results remain the canonical values stored in the
// transcript; archive encoding does not reinterpret them.
func artifactFromPortable(portable sessions.PortableSnapshot) (protocol.SessionArtifact, error) {
	messages := make([]json.RawMessage, 0, len(portable.Messages))
	for _, message := range portable.Messages {
		encoded, err := json.Marshal(message)
		if err != nil {
			return protocol.SessionArtifact{}, fmt.Errorf("marshal message: %w", err)
		}
		messages = append(messages, encoded)
	}

	runs := make([]protocol.ArtifactRun, 0, len(portable.Runs))
	for _, run := range portable.Runs {
		encoded, err := artifactRunFromPortable(run)
		if err != nil {
			return protocol.SessionArtifact{}, err
		}
		runs = append(runs, encoded)
	}
	items := make([]protocol.ArtifactItem, 0, len(portable.Items))
	for _, item := range portable.Items {
		encoded, err := artifactItemFromTranscript(item)
		if err != nil {
			return protocol.SessionArtifact{}, err
		}
		items = append(items, encoded)
	}
	toolResults := make([]protocol.ArtifactToolResult, 0, len(portable.ToolResults))
	for _, blob := range portable.ToolResults {
		toolResults = append(toolResults, protocol.ArtifactToolResult{
			ID: blob.ID.String(), ItemID: blob.ItemID, ToolName: blob.ToolName,
			Preview: blob.Preview, Body: blob.Body, CreatedAt: blob.CreatedAt,
		})
	}
	return protocol.SessionArtifact{
		Version:  protocol.SessionArtifactVersion,
		Session:  artifactSessionFromPortable(portable.Session),
		Messages: messages, Runs: runs, Items: items, ToolResults: toolResults,
		Plan: presentPlanStepList(portable.Plan),
	}, nil
}

func artifactSessionFromPortable(value sessions.PortableSession) protocol.ArtifactSession {
	return protocol.ArtifactSession{
		ID: value.ID, Title: value.Title, Workspace: protocol.WorkspaceRef{Path: value.CWD},
		Provider: value.Selection.Provider(), Model: value.Selection.Model(),
		ReasoningEffort: value.Selection.ReasoningEffort(),
		CreatedAt:       value.CreatedAt, UpdatedAt: value.UpdatedAt, Favorite: value.Favorite,
	}
}

func artifactRunFromPortable(run sessions.PortableRun) (protocol.ArtifactRun, error) {
	outcome, err := artifactOutcomeType(run.Outcome)
	if err != nil {
		return protocol.ArtifactRun{}, fmt.Errorf("run %q outcome: %w", run.ID, err)
	}
	problem, err := artifactRunFailureFromDomain(run.Failure)
	if err != nil {
		return protocol.ArtifactRun{}, fmt.Errorf("run %q failure: %w", run.ID, err)
	}
	return protocol.ArtifactRun{
		ID: run.ID, SessionID: run.SessionID, SpawnedByItemID: run.SpawnedByItemID,
		Provider: run.Selection.Provider(), Model: run.Selection.Model(),
		ReasoningEffort: run.Selection.ReasoningEffort(),
		ParentRunID:     run.ParentRunID,
		RootRunID:       run.RootRunID,
		Limits:          presentLimits(run.Limits),
		Metrics:         presentMetrics(run.Metrics),
		ContextTokens:   run.ContextTokens,
		ProtocolProfile: presentArtifactProtocolProfile(run.Capabilities),
		Outcome: protocol.ArtifactOutcome{
			Type: outcome, Error: problem, Detail: run.Detail,
		},
		CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt,
		UpdatedAt: run.UpdatedAt, MessageMark: run.MessageMark,
	}, nil
}

// presentArtifactProtocolProfile writes the protocol contract a root Run
// published under, and
// nothing for a child — a child reads its root's, and writing a second copy is how
// the two come to disagree.
func presentArtifactProtocolProfile(capabilities *run.Capabilities) *protocol.RunProtocolProfile {
	if capabilities == nil {
		return nil
	}
	presented := presentRunProtocolProfile(*capabilities)
	return &presented
}

func artifactOutcomeType(outcome run.Outcome) (protocol.ArtifactOutcomeType, error) {
	switch outcome {
	case run.OutcomeCompleted:
		return protocol.ArtifactOutcomeCompleted, nil
	case run.OutcomeCanceled:
		return protocol.ArtifactOutcomeCanceled, nil
	case run.OutcomeTimedOut:
		return protocol.ArtifactOutcomeTimedOut, nil
	case run.OutcomeFailed:
		return protocol.ArtifactOutcomeFailed, nil
	case run.OutcomeMaxBudget:
		return protocol.ArtifactOutcomeMaxBudget, nil
	case run.OutcomeMaxSteps:
		return protocol.ArtifactOutcomeMaxSteps, nil
	case run.OutcomeLost:
		return protocol.ArtifactOutcomeLost, nil
	default:
		return "", fmt.Errorf("unknown value %q", outcome)
	}
}

func artifactRunFailureFromDomain(failure *run.Failure) (*protocol.ArtifactProblem, error) {
	if failure == nil {
		return nil, nil
	}
	kind, err := artifactRunFailureType(failure.Kind)
	if err != nil {
		return nil, err
	}
	return &protocol.ArtifactProblem{
		Type: kind, Detail: failure.Detail, DocURL: failure.DocURL,
		RetryAfterSeconds: failure.RetryAfterSeconds(),
	}, nil
}

func artifactRunFailureType(kind run.FailureKind) (protocol.ArtifactProblemType, error) {
	switch kind {
	case run.FailureInternal:
		return protocol.ArtifactProblemInternalError, nil
	case run.FailureLost:
		return protocol.ArtifactProblemRunLost, nil
	case run.FailureAgentStuck:
		return protocol.ArtifactProblemAgentStuck, nil
	case run.FailureRateLimited:
		return protocol.ArtifactProblemRateLimited, nil
	case run.FailureInvalidCredentials:
		return protocol.ArtifactProblemInvalidAPIKey, nil
	case run.FailureTimeout:
		return protocol.ArtifactProblemTimeout, nil
	case run.FailureProviderUnavailable:
		return protocol.ArtifactProblemProviderUnavailable, nil
	case run.FailureProviderRejected:
		return protocol.ArtifactProblemProviderRejected, nil
	default:
		return "", fmt.Errorf("unknown value %q", kind)
	}
}

func artifactToolFailureFromDomain(failure *tool.Failure) (*protocol.ArtifactProblem, error) {
	if failure == nil {
		return nil, nil
	}
	var kind protocol.ArtifactProblemType
	switch failure.Kind {
	case tool.FailureInternal:
		kind = protocol.ArtifactProblemInternalError
	case tool.FailureDenied:
		kind = protocol.ArtifactProblemDeniedByUser
	case tool.FailureExecution:
		kind = protocol.ArtifactProblemToolFailed
	case tool.FailureChildRunCanceled:
		kind = protocol.ArtifactProblemChildRunCanceled
	case tool.FailureCanceled:
		kind = protocol.ArtifactProblemToolCanceled
	default:
		return nil, fmt.Errorf("unknown value %q", failure.Kind)
	}
	return &protocol.ArtifactProblem{
		Type: kind, Detail: failure.Detail, DocURL: failure.DocURL,
	}, nil
}

func artifactItemFromTranscript(item transcript.Item) (protocol.ArtifactItem, error) {
	var failureRef *tool.Failure
	if failure, present := item.Failure(); present {
		failureRef = &failure
	}
	problem, err := artifactToolFailureFromDomain(failureRef)
	if err != nil {
		return protocol.ArtifactItem{}, fmt.Errorf("item %q error: %w", item.ID(), err)
	}
	out := protocol.ArtifactItem{
		ID: item.ID(), RunID: item.RunID(), Status: presentItemStatus(item.Status()),
		Type: presentItemKind(item.Kind()), Phase: presentMessagePhase(item.MessagePhase()), Text: item.Text(), Redacted: item.Redacted(),
		SafetyClass: presentSafetyClass(item.SafetyClass()), ApprovalDecision: presentItemApprovalDecision(item.ApprovalDecision()), Error: problem,
		Summary: item.Summary(), DroppedMessages: item.DroppedMessages(),
	}
	content := item.Content()
	if len(content) != 0 {
		out.Content = make([]protocol.ContentBlock, len(content))
		for index, block := range content {
			encoded, err := encodeContent(block)
			if err != nil {
				return protocol.ArtifactItem{}, fmt.Errorf("item %q content %d: %w", item.ID(), index, err)
			}
			out.Content[index] = protocol.ContentBlock{Type: encoded.kind, Text: encoded.text, Mime: encoded.mime, Data: encoded.data}
		}
	}
	if value, present := item.Question(); present {
		question := presentQuestion(value)
		out.Question = &question
	}
	if invocation, present := item.ToolInvocation(); present {
		tool := protocol.ToolInvocation{Name: invocation.Name, Arguments: invocation.Arguments.Map()}
		if invocation.Result != nil {
			tool.Result = invocation.Result.Any()
		}
		out.Tool = &tool
	}
	if item.Kind() == transcript.ToolCall {
		out.StartedAt = item.OccurredAt()
		out.FinishedAt = item.FinishedAt()
		out.DurationMillis = presentToolDurationMillis(item)
	} else {
		out.CreatedAt = item.OccurredAt()
	}
	return out, nil
}

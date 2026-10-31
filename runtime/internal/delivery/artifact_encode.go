package delivery

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// artifactFromPortable maps the terminal archive projection to the versioned
// protocol document. Tool results remain the canonical values stored in the
// transcript; archive encoding does not reinterpret them.
func artifactFromPortable(portable sessions.PortableSnapshot) (protocol.SessionArtifact, error) {
	messages := make([]jsontext.Value, 0, len(portable.Messages))
	for _, message := range portable.Messages {
		encoded, err := json.Marshal(message, json.Deterministic(true))
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
	for _, result := range portable.ToolResults {
		toolResults = append(toolResults, protocol.ArtifactToolResult{
			ID: result.Blob.ID.String(), ItemID: result.ItemID,
			Body: result.Blob.Body, CreatedAt: result.Blob.CreatedAt,
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
	outcome, err := artifactOutcomeFromEnding(run.Ending)
	if err != nil {
		return protocol.ArtifactRun{}, fmt.Errorf("run %q outcome: %w", run.ID, err)
	}
	outcome.UnresolvedEffects = presentUnresolvedEffects(run.UnresolvedEffects)
	return protocol.ArtifactRun{
		ID: run.ID, SessionID: run.SessionID, SpawnedByItemID: run.SpawnedByItemID,
		Provider: run.Selection.Provider(), Model: run.Selection.Model(),
		ReasoningEffort: run.Selection.ReasoningEffort(),
		ParentRunID:     run.ParentRunID,
		RootRunID:       run.RootRunID,
		Metrics:         presentMetrics(run.Metrics),
		ContextTokens:   run.ContextTokens,
		Capabilities:    presentArtifactCapabilities(run.Capabilities),
		Outcome:         outcome,
		CreatedAt:       run.CreatedAt, FinishedAt: run.FinishedAt,
		UpdatedAt: run.UpdatedAt, MessageMark: artifactMessageMark(run.MessageMark),
	}, nil
}

// presentArtifactCapabilities writes the protocol contract a root Run
// published under, and
// nothing for a child — a child reads its root's, and writing a second copy is how
// the two come to disagree.
func presentArtifactCapabilities(capabilities *run.Capabilities) *protocol.RunCapabilities {
	if capabilities == nil {
		return nil
	}
	presented := presentRunCapabilities(*capabilities)
	return &presented
}

func artifactOutcomeFromEnding(ending run.Ending) (protocol.ArtifactOutcome, error) {
	switch ending := ending.(type) {
	case run.Completed:
		return protocol.ArtifactOutcome{Type: protocol.ArtifactOutcomeCompleted}, nil
	case run.Canceled:
		return protocol.ArtifactOutcome{Type: protocol.ArtifactOutcomeCanceled, Detail: ending.Detail}, nil
	case run.TimedOut:
		return protocol.ArtifactOutcome{
			Type:  protocol.ArtifactOutcomeTimedOut,
			Error: &protocol.ArtifactProblem{Type: protocol.ArtifactProblemTimeout, Detail: ending.Detail},
		}, nil
	case run.Failed:
		if err := ending.Failure.Validate(); err != nil {
			return protocol.ArtifactOutcome{}, err
		}
		return protocol.ArtifactOutcome{Type: protocol.ArtifactOutcomeFailed, Error: &protocol.ArtifactProblem{
			Type: protocol.ArtifactProblemType(ending.Failure.Kind), Detail: ending.Failure.Detail, DocURL: ending.Failure.DocURL,
			RetryAfterSeconds: ending.Failure.RetryAfterSeconds(),
		}}, nil
	case run.Lost:
		return protocol.ArtifactOutcome{Type: protocol.ArtifactOutcomeLost, Loss: presentLoss(ending.Loss)}, nil
	default:
		return protocol.ArtifactOutcome{}, fmt.Errorf("unknown ending %T", ending)
	}
}

func artifactToolFailureFromDomain(failure *tool.Failure) (*protocol.ArtifactProblem, error) {
	if failure == nil {
		return nil, nil
	}
	if err := failure.Validate(); err != nil {
		return nil, err
	}
	return &protocol.ArtifactProblem{
		Type: protocol.ArtifactProblemType(failure.Kind), Detail: failure.Detail, DocURL: failure.DocURL,
	}, nil
}

func artifactItemFromTranscript(item transcript.Item) (protocol.ArtifactItem, error) {
	out := protocol.ArtifactItem{
		ID: item.ID(), RunID: item.RunID(), Status: presentItemStatus(item.Status()),
		Type: presentItemKind(item.Kind()),
	}
	payload := item.Body()
	if _, isToolCall := payload.(transcript.ToolCallBody); !isToolCall {
		out.CreatedAt = item.OccurredAt()
	}
	var content []transcript.ContentBlock
	switch payload := payload.(type) {
	case transcript.UserMessageBody:
		content = payload.Content
	case transcript.AgentMessageBody:
		out.Phase = presentMessagePhase(payload.Phase)
		content = payload.Content
	case transcript.ReasoningBody:
		out.Text, out.Redacted = payload.Text, payload.Redacted
	case transcript.QuestionBody:
		question := presentQuestion(payload.Question)
		out.Question = &question
	case transcript.CompactionBody:
		out.Summary, out.DroppedMessages = payload.Summary, payload.DroppedMessages
	case transcript.ToolCallBody:
		problem, err := artifactToolFailureFromDomain(payload.Failure)
		if err != nil {
			return protocol.ArtifactItem{}, fmt.Errorf("item %q error: %w", item.ID(), err)
		}
		out.Error = problem
		out.SafetyClass = presentSafetyClass(payload.SafetyClass)
		if payload.Approval != nil {
			out.ApprovalDecision = presentItemApprovalDecision(payload.Approval.Decision)
		}
		invocation := payload.Invocation
		rejected, _ := invocation.Input.Rejected()
		tool := protocol.ToolInvocation{Name: invocation.Name, ArgumentsText: rejected, Arguments: invocation.Input.Arguments().Map()}
		if invocation.Result != nil {
			tool.Result = invocation.Result.Any()
		}
		out.Tool = &tool
		out.StartedAt = item.OccurredAt()
		out.FinishedAt = payload.FinishedAt
		if payload.ExecutionDuration != nil {
			out.DurationMillis = new(payload.ExecutionDuration.Milliseconds())
		}
	}
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
	return out, nil
}

func artifactMessageMark(mark run.MessageMark) *int {
	count, known := mark.Count()
	if !known {
		return nil
	}
	return &count
}

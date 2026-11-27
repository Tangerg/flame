package acp

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/acp"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	allowOnce acp.PermissionOptionID = "allow-once"
	denyOnce  acp.PermissionOptionID = "deny-once"
)

var (
	errQuestionRequiresFlame = errors.New("run is waiting for input; continue it in Flame")
	errPermissionCancelled   = errors.New("the ACP client cancelled the turn during permission review")
)

func (p *projection) answerInterrupts(ctx context.Context, interrupts []conversation.Interrupt) ([]conversation.InterruptAnswer, error) {
	for _, interrupt := range interrupts {
		if _, approval := interrupt.(conversation.Approval); !approval {
			return nil, errQuestionRequiresFlame
		}
	}
	answers := make([]conversation.InterruptAnswer, 0, len(interrupts))
	for _, interrupt := range interrupts {
		approval := interrupt.(conversation.Approval)
		answer, err := p.requestPermission(ctx, approval)
		if err != nil {
			return nil, err
		}
		answers = append(answers, conversation.InterruptAnswer{ItemID: approval.ItemID, Answer: answer})
	}
	return answers, nil
}

func (p *projection) requestPermission(ctx context.Context, approval conversation.Approval) (conversation.ApprovalAnswer, error) {
	id := acp.ToolCallID(conversation.BlockKey(approval.RunID, approval.ItemID))
	if err := p.publishTool(id, approval.Tool, acp.ToolCallStatusPending); err != nil {
		return conversation.ApprovalAnswer{}, err
	}
	call := projectTool(id, approval.Tool, acp.ToolCallStatusPending)
	if approval.Detail != "" {
		call.Content = append(call.Content, &acp.Content{Content: &acp.TextContent{Text: approval.Detail}})
	}
	response, err := p.session.RequestPermission(ctx, &acp.RequestPermissionParams{
		ToolCall: toolUpdate(call),
		Options: []acp.PermissionOption{
			{OptionID: allowOnce, Name: "Allow once", Kind: acp.PermissionOptionKindAllowOnce},
			{OptionID: denyOnce, Name: "Deny once", Kind: acp.PermissionOptionKindRejectOnce},
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			return conversation.ApprovalAnswer{}, context.Cause(ctx)
		}
		return conversation.ApprovalAnswer{}, fmt.Errorf("request ACP tool permission: %w", err)
	}
	switch outcome := response.Outcome.(type) {
	case *acp.SelectedPermissionOutcome:
		if outcome.OptionID == allowOnce {
			return conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove}, nil
		}
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalDeny, Reason: "denied by the ACP client"}, nil
	case *acp.RequestPermissionOutcomeCancelled:
		return conversation.ApprovalAnswer{}, errPermissionCancelled
	default:
		return conversation.ApprovalAnswer{}, fmt.Errorf("unsupported ACP permission outcome %T", outcome)
	}
}

func (p *projection) publishTool(id acp.ToolCallID, tool *conversation.ToolCall, status acp.ToolCallStatus) error {
	previous, announced := p.tools[id]
	if announced && (previous == status || status == acp.ToolCallStatusPending ||
		previous == acp.ToolCallStatusCompleted || previous == acp.ToolCallStatusFailed) {
		return nil
	}
	call := projectTool(id, tool, status)
	var update acp.SessionUpdate = &call
	if announced {
		update = new(toolUpdate(call))
	}
	if err := p.update(update); err != nil {
		return err
	}
	p.tools[id] = status
	return nil
}

func (p *projection) toolOutputStarted(id acp.ToolCallID) error {
	if p.tools[id] != acp.ToolCallStatusPending {
		return nil
	}
	if err := p.update(&acp.ToolCallUpdate{ToolCallID: id, Status: acp.OptValue(acp.ToolCallStatusInProgress)}); err != nil {
		return err
	}
	p.tools[id] = acp.ToolCallStatusInProgress
	return nil
}

func projectTool(id acp.ToolCallID, tool *conversation.ToolCall, status acp.ToolCallStatus) acp.ToolCall {
	call := acp.ToolCall{
		ToolCallID: id, Title: cmp.Or(tool.Summary, tool.Name, "Tool"),
		Kind: acp.OptValue(toolKind(tool.Kind)), Status: acp.OptValue(status),
	}
	if tool.Name != "" {
		call.Name = acp.OptValue(tool.Name)
	}
	if len(tool.ArgumentsJSON) != 0 {
		call.RawInput = acp.OptValue(json.RawMessage(tool.ArgumentsJSON))
	}
	if len(tool.ResultJSON) != 0 {
		call.RawOutput = acp.OptValue(json.RawMessage(tool.ResultJSON))
	}
	if tool.Output != "" {
		call.Content = []acp.ToolCallContent{&acp.Content{Content: &acp.TextContent{Text: tool.Output}}}
	}
	return call
}

func toolUpdate(call acp.ToolCall) acp.ToolCallUpdate {
	update := acp.ToolCallUpdate{
		ToolCallID: call.ToolCallID, Title: acp.OptValue(call.Title), Name: call.Name,
		Kind: call.Kind, Status: call.Status, RawInput: call.RawInput, RawOutput: call.RawOutput,
	}
	if len(call.Content) != 0 {
		update.Content = acp.OptValue(call.Content)
	}
	return update
}

func toolStatus(tool *conversation.ToolCall) acp.ToolCallStatus {
	switch tool.Status {
	case conversation.ToolOK:
		return acp.ToolCallStatusCompleted
	case conversation.ToolError, conversation.ToolCanceled:
		return acp.ToolCallStatusFailed
	default:
		if !tool.StartedAt.IsZero() {
			return acp.ToolCallStatusInProgress
		}
		return acp.ToolCallStatusPending
	}
}

func toolKind(category builtin.ToolCategory) acp.ToolKind {
	switch category {
	case builtin.CategoryShell:
		return acp.ToolKindExecute
	case builtin.CategoryEdit:
		return acp.ToolKindEdit
	case builtin.CategoryRead:
		return acp.ToolKindRead
	case builtin.CategorySearch:
		return acp.ToolKindSearch
	default:
		return acp.ToolKindOther
	}
}

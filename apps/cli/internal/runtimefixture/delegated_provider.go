package runtimefixture

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

const (
	probePromptTokens     = 4
	probeCompletionTokens = 2
)

func ServeDelegatedApproval(w http.ResponseWriter, r *http.Request) {
	var request delegatedRequest
	if err := json.UnmarshalRead(r.Body, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	turn, scripted := request.turn()
	if !scripted {
		http.Error(w, "unexpected delegated approval request", http.StatusBadRequest)
		return
	}
	response := turn.response(request.Stream)
	if !request.Stream {
		w.Header().Set(headerContentType, "application/json")
		_ = json.MarshalWrite(w, response)
		return
	}
	data, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(headerContentType, "text/event-stream")
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
}

type delegatedRequest struct {
	Stream   bool `json:"stream"`
	Messages []struct {
		Role       string         `json:"role"`
		Content    jsontext.Value `json:"content"`
		ToolCallID string         `json:"tool_call_id"`
	} `json:"messages"`
	Tools []jsontext.Value `json:"tools"`
}

const headerContentType = "Content-Type"

const (
	DelegateApprovalProbe = "delegate approval probe"
	ChildApprovalProbe    = "child approval probe"
	delegateChildCallID   = "delegate_child"
	childShellCallID      = "child_shell"
)

const (
	wireRoleUser          = "user"
	wireRoleTool          = "tool"
	wireRoleAssistant     = "assistant"
	finishStop            = "stop"
	finishToolCalls       = "tool_calls"
	toolTypeFunction      = "function"
	objectCompletion      = "chat.completion"
	objectCompletionChunk = "chat.completion.chunk"
)

type delegatedTurn struct {
	content                 string
	callID, name, arguments string
}

func (r delegatedRequest) turn() (delegatedTurn, bool) {
	if len(r.Tools) == 0 {
		return delegatedTurn{content: "maintenance complete"}, true
	}
	for _, message := range slices.Backward(r.Messages) {
		switch {
		case message.Role == wireRoleTool && message.ToolCallID == childShellCallID:
			return delegatedTurn{content: "child complete"}, true
		case message.Role == wireRoleTool && message.ToolCallID == delegateChildCallID:
			return delegatedTurn{content: "root complete"}, true
		case message.Role == wireRoleUser && strings.Contains(string(message.Content), ChildApprovalProbe):
			return delegatedTurn{callID: childShellCallID, name: "shell", arguments: `{"command":"printf approved","description":"Return the approved marker"}`}, true
		case message.Role == wireRoleUser && strings.Contains(string(message.Content), DelegateApprovalProbe):
			return delegatedTurn{callID: delegateChildCallID, name: "delegate_task", arguments: `{"summary":"approval probe","instructions":"` + ChildApprovalProbe + `"}`}, true
		}
	}
	return delegatedTurn{}, false
}

type completionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int                `json:"created"`
	Model   string             `json:"model"`
	Choices []completionChoice `json:"choices"`
	Usage   completionUsage    `json:"usage"`
}

type completionChoice struct {
	Index        int                `json:"index"`
	FinishReason string             `json:"finish_reason"`
	Delta        *completionMessage `json:"delta,omitempty"`
	Message      *completionMessage `json:"message,omitempty"`
}

type completionMessage struct {
	Role      string               `json:"role"`
	Content   string               `json:"content,omitempty"`
	ToolCalls []completionToolCall `json:"tool_calls,omitempty"`
}

type completionToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type completionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (t delegatedTurn) response(stream bool) completionResponse {
	message := &completionMessage{Role: wireRoleAssistant, Content: t.content}
	finish := finishStop
	if t.name != "" {
		call := completionToolCall{ID: t.callID, Type: toolTypeFunction}
		call.Function.Name, call.Function.Arguments = t.name, t.arguments
		message, finish = &completionMessage{Role: wireRoleAssistant, ToolCalls: []completionToolCall{call}}, finishToolCalls
	}
	choice := completionChoice{FinishReason: finish, Message: message}
	object := objectCompletion
	if stream {
		choice.Delta, choice.Message, object = message, nil, objectCompletionChunk
	}
	return completionResponse{
		ID: "chatcmpl_probe", Object: object, Created: 1, Model: "deepseek-chat",
		Choices: []completionChoice{choice}, Usage: completionUsage{PromptTokens: probePromptTokens, CompletionTokens: probeCompletionTokens, TotalTokens: probePromptTokens + probeCompletionTokens},
	}
}

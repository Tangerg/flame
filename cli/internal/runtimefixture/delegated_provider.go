package runtimefixture

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// ServeDelegatedApproval exercises a root delegation and a child shell approval
// using the OpenAI-compatible provider surface.
func ServeDelegatedApproval(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Role       string         `json:"role"`
			Content    jsontext.Value `json:"content"`
			ToolCallID string         `json:"tool_call_id"`
		} `json:"messages"`
		Tools []jsontext.Value `json:"tools"`
	}
	if err := json.UnmarshalRead(r.Body, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	content, callID, name, arguments := "maintenance complete", "", "", ""
	if len(request.Tools) > 0 {
		matched := false
		for _, message := range slices.Backward(request.Messages) {
			switch {
			case message.Role == "tool" && message.ToolCallID == "child_shell":
				content = "child complete"
			case message.Role == "tool" && message.ToolCallID == "delegate_child":
				content = "root complete"
			case message.Role == "user" && strings.Contains(string(message.Content), "child approval probe"):
				callID, name, arguments = "child_shell", "shell", `{"command":"printf approved","description":"Return the approved marker"}`
			case message.Role == "user" && strings.Contains(string(message.Content), "delegate approval probe"):
				callID, name, arguments = "delegate_child", "delegate_task", `{"summary":"approval probe","instructions":"child approval probe"}`
			default:
				continue
			}
			matched = true
			break
		}
		if !matched {
			http.Error(w, "unexpected delegated approval request", http.StatusBadRequest)
			return
		}
	}
	delta := map[string]any{"role": "assistant", "content": content}
	finish := "stop"
	if name != "" {
		delta = map[string]any{
			"role": "assistant", "tool_calls": []any{map[string]any{
				"index": 0, "id": callID, "type": "function",
				"function": map[string]any{"name": name, "arguments": arguments},
			}},
		}
		finish = "tool_calls"
	}
	choice := map[string]any{"index": 0, "finish_reason": finish}
	object := "chat.completion"
	if request.Stream {
		object = "chat.completion.chunk"
		choice["delta"] = delta
	} else {
		choice["message"] = delta
	}
	body := map[string]any{
		"id": "chatcmpl_probe", "object": object, "created": 1, "model": "deepseek-chat",
		"choices": []any{choice}, "usage": map[string]int{"prompt_tokens": 4, "completion_tokens": 2, "total_tokens": 6},
	}
	if request.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		data, err := json.Marshal(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.MarshalWrite(w, body)
}

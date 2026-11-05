package model

import (
	"context"
	"crypto/sha256"
	json "encoding/json/v2"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// RequestDiagnostics is derived from a captured Scope request, not vendor wire
// bytes or a cache-hit observation. It owns no request, usage or billing state.
type RequestDiagnostics struct {
	Bytes          int
	SystemDigest   string
	ToolsDigest    string
	MessagesDigest string
	OptionsDigest  string
	MessageDigests []string
}

func DescribeRequest(request *chat.Request) (RequestDiagnostics, error) {
	if err := request.Validate(); err != nil {
		return RequestDiagnostics{}, err
	}
	encoded, err := json.Marshal(request, json.Deterministic(true))
	if err != nil {
		return RequestDiagnostics{}, err
	}
	result := RequestDiagnostics{Bytes: len(encoded), MessageDigests: make([]string, len(request.Messages))}
	var systems []chat.Message
	for index, message := range request.Messages {
		if message.Role == chat.RoleSystem {
			systems = append(systems, message)
		}
		result.MessageDigests[index], err = diagnosticDigest(message)
		if err != nil {
			return RequestDiagnostics{}, err
		}
	}
	for _, field := range []struct {
		value any
		into  *string
	}{
		{systems, &result.SystemDigest},
		{struct {
			Definitions []toolcontract.Definition `json:"definitions"`
			Choice      *chat.ToolChoice          `json:"choice,omitzero"`
		}{request.Tools, request.ToolChoice}, &result.ToolsDigest},
		{request.Messages, &result.MessagesDigest},
		{request.Options, &result.OptionsDigest},
	} {
		*field.into, err = diagnosticDigest(field.value)
		if err != nil {
			return RequestDiagnostics{}, err
		}
	}
	return result, nil
}

func diagnosticDigest(value any) (string, error) {
	encoded, err := json.Marshal(value, json.Deterministic(true))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}

// FirstChangedMessage returns the first differing zero-based Scope message, or
// -1 when the projections are identical. An appended tail starts at the old size.
func (d RequestDiagnostics) FirstChangedMessage(next RequestDiagnostics) int {
	for index := range min(len(d.MessageDigests), len(next.MessageDigests)) {
		if d.MessageDigests[index] != next.MessageDigests[index] {
			return index
		}
	}
	if len(d.MessageDigests) != len(next.MessageDigests) {
		return min(len(d.MessageDigests), len(next.MessageDigests))
	}
	return -1
}

func logRequestDiagnostics(ctx context.Context, invocation modelref.EndpointInvocation) {
	if !slog.Default().Enabled(ctx, slog.LevelDebug) {
		return
	}
	request, chatRequest := invocation.Request.(modelref.ChatEndpointRequest)
	if !chatRequest {
		return
	}
	diagnostic, err := DescribeRequest(request.Chat)
	if err != nil {
		slog.DebugContext(ctx, "model request diagnostics unavailable", "endpoint.id", invocation.ID, "error", err)
		return
	}
	slog.DebugContext(ctx, "model request projection", "endpoint.id", invocation.ID,
		"caller.id", invocation.Caller.ID, "provider", invocation.Provider, "model", invocation.Model,
		"projection", "scope_request", "request.bytes", diagnostic.Bytes,
		"system.sha256", diagnostic.SystemDigest, "tools.sha256", diagnostic.ToolsDigest,
		"messages.sha256", diagnostic.MessagesDigest, "options.sha256", diagnostic.OptionsDigest,
		"message.count", len(diagnostic.MessageDigests))
}

func logResponseDiagnostics(ctx context.Context, invocation modelref.EndpointInvocation, response *chat.Response) {
	if !slog.Default().Enabled(ctx, slog.LevelDebug) {
		return
	}
	var usage *chat.Usage
	if response != nil && response.Metadata != nil {
		usage = response.Metadata.Usage
	}
	fields := []any{"endpoint.id", invocation.ID, "usage.reported", usage != nil}
	if usage != nil {
		fields = append(fields, "input.tokens", usage.InputTokens, "output.tokens", usage.OutputTokens,
			"cache.read.reported", usage.CacheReadInputTokens != nil, "cache.write.reported", usage.CacheWriteInputTokens != nil)
		if usage.CacheReadInputTokens != nil {
			fields = append(fields, "cache.read.tokens", *usage.CacheReadInputTokens)
		}
		if usage.CacheWriteInputTokens != nil {
			fields = append(fields, "cache.write.tokens", *usage.CacheWriteInputTokens)
		}
	}
	slog.DebugContext(ctx, "model response cache observation", fields...)
}

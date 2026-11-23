package builtin

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/samber/lo"
)

const (
	defaultReadWindow = 20_000
)

const toolResultDescription = `Read omitted bytes from an earlier tool result that was
offloaded to keep the conversation small. Copy result_id from the inline
offload marker. Use offset_bytes and limit_bytes to page through the result;
each call returns at most 20000 bytes. Call this only when the existing preview
does not contain enough information.`

type ToolResultStore interface {
	Fetch(ctx context.Context, sessionID identity.SessionID, id transcript.ToolResultID) (body string, found bool, err error)
}

type toolResultReadArgs struct {
	ResultID    string `json:"result_id" jsonschema:"minLength=2,maxLength=64,pattern=^[A-Z2-7]+$" jsonschema_description:"Offloaded result identifier copied exactly from the inline marker."`
	OffsetBytes int    `json:"offset_bytes,omitzero" jsonschema:"minimum=0" jsonschema_description:"Zero-based byte offset at which to start reading. Defaults to 0."`
	LimitBytes  *int   `json:"limit_bytes,omitzero" jsonschema:"minimum=1,maximum=20000" jsonschema_description:"Maximum bytes to return. Defaults to 20000 and cannot exceed 20000."`
}

type toolResultReader struct {
	store ToolResultStore
}

func NewToolResultReader(store ToolResultStore) (toolcontract.Tool, error) {
	if lo.IsNil(store) {
		return nil, fmt.Errorf("%s: Tool-result store is required", builtincontract.ReadToolResult)
	}
	return toolcontract.NewFunc[toolResultReadArgs, string](
		toolcontract.FuncConfig{Name: string(builtincontract.ReadToolResult), Description: toolResultDescription},
		(&toolResultReader{store: store}).read,
	)
}

func (t *toolResultReader) read(ctx context.Context, a toolResultReadArgs) (string, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID.IsZero() {
		return "", pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: no active session", builtincontract.ReadToolResult))
	}
	id, err := transcript.ParseToolResultID(a.ResultID)
	if err != nil {
		return "", pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: result_id must be the uppercase base32 value from an offloaded-result marker: %w", builtincontract.ReadToolResult, err))
	}
	body, found, err := t.store.Fetch(ctx, sessionID, id)
	if err != nil {
		return "", pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: %w", builtincontract.ReadToolResult, err))
	}
	if !found {
		return "", pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: no stored tool result with result_id %s", builtincontract.ReadToolResult, a.ResultID))
	}

	limit := lo.FromPtrOr(a.LimitBytes, defaultReadWindow)
	start, end := window(body, a.OffsetBytes, limit)
	header := fmt.Sprintf("[tool result %s — %d bytes total, showing bytes %d–%d]\n", a.ResultID, len(body), start, end)
	if end < len(body) {
		header += fmt.Sprintf("[%d bytes remain; call again with {\"result_id\":%q,\"offset_bytes\":%d} for the next window]\n", len(body)-end, a.ResultID, end)
	}
	return header + body[start:end], nil
}

func window(body string, offset, limit int) (start, end int) {
	total := len(body)
	start = min(max(offset, 0), total)
	for start < total && !utf8.RuneStart(body[start]) {
		start++
	}
	end = min(start+limit, total)
	for end < total && !utf8.RuneStart(body[end]) {
		end++
	}
	return start, end
}

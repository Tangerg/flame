package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strings"
	"unicode/utf8"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

const ToolContextMetadataKey = "io.github.tangerg.flame/tool-context.v1"

// ToolContext projects the invoking owner's workspace and optional captured
// vector space. Callback authority travels only in the existing callback metadata.
type ToolContext struct {
	CWD            string `json:"cwd"`
	EmbeddingSpace string `json:"embeddingSpace,omitzero"`
}

func (c ToolContext) Validate() error {
	if strings.TrimSpace(c.CWD) == "" || !utf8.ValidString(c.CWD) || !utf8.ValidString(c.EmbeddingSpace) {
		return errors.New("plugin: captured Tool workspace is required")
	}
	return nil
}

func WithToolContext(ctx context.Context, captured ToolContext) (context.Context, error) {
	if err := captured.Validate(); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, toolContextKey{}, captured), nil
}

func ToolContextFromContext(ctx context.Context) (ToolContext, error) {
	captured, ok := ctx.Value(toolContextKey{}).(ToolContext)
	if !ok {
		return ToolContext{}, errors.New("plugin: captured Tool context is required")
	}
	return captured, captured.Validate()
}

type toolContextKey struct{}

// ToolContextFromMetadata admits the closed workspace projection at the MCP
// boundary. Other request metadata cannot become workspace or callback authority.
func ToolContextFromMetadata(metadata sdk.Meta) (ToolContext, error) {
	encoded := metadata[ToolContextMetadataKey]
	var captured ToolContext
	if err := json.Unmarshal(encoded, &captured, json.RejectUnknownMembers(true)); err != nil {
		return ToolContext{}, err
	}
	return captured, captured.Validate()
}

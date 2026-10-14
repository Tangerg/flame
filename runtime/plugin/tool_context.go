package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"maps"
	"strings"
	"unicode/utf8"

	scopemcp "github.com/Tangerg/scope/mcp"
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
	metadata := maps.Clone(scopemcp.RequestMetaFromContext(ctx))
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata[ToolContextMetadataKey] = captured
	return scopemcp.WithRequestMeta(ctx, metadata), nil
}

func ToolContextFromContext(ctx context.Context) (ToolContext, error) {
	encoded, err := json.Marshal(scopemcp.RequestMetaFromContext(ctx)[ToolContextMetadataKey])
	if err != nil {
		return ToolContext{}, err
	}
	var captured ToolContext
	if err := json.Unmarshal(encoded, &captured, json.RejectUnknownMembers(true)); err != nil {
		return ToolContext{}, err
	}
	return captured, captured.Validate()
}

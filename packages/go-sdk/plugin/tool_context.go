package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

// Project is the Run's captured Project identity, empty when its Session belongs
// to none. Project-scoped plugins key on it instead of inferring from CWD.
type ToolContext struct {
	CWD            string `json:"cwd"`
	Project        string `json:"project,omitzero"`
	EmbeddingSpace string `json:"embeddingSpace,omitzero"`
}

func (c ToolContext) Validate() error {
	if strings.TrimSpace(c.CWD) == "" || !utf8.ValidString(c.CWD) || !utf8.ValidString(c.EmbeddingSpace) {
		return errors.New("plugin: captured Tool workspace is required")
	}
	_, err := identity.ParseOptionalProject(c.Project)
	return err
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

func toolContextFromMetadata(metadata sdk.Meta) (ToolContext, error) {
	encoded := metadata[pluginmcp.ToolContextMetadataKey]
	var captured ToolContext
	if err := json.Unmarshal(encoded, &captured, json.RejectUnknownMembers(true)); err != nil {
		return ToolContext{}, err
	}
	return captured, captured.Validate()
}

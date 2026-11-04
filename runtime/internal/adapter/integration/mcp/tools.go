package mcp

import (
	"context"
	"errors"
	"fmt"

	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/core/metadata"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"

	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

const rejectedRemoteToolPlaceholder = "invalid_remote_tool"

// sourceTools lists one MCP source's model-facing tools. Isolated per source so
// a single server's tools/list failure stays its own. owner is the connection
// registry that will hold session; nil for a throwaway probe, whose
// executables are never current.
func sourceTools(ctx context.Context, owner *connections, descriptor ServerConfig, session *sdkmcp.ClientSession) ([]Executable, error) {
	server := descriptor.ID()
	source := scopemcp.ToolSource{Name: descriptor.Name.String(), Session: session}
	var remoteNameErr error
	callKey := &sourceCallMetadata{}
	tools, discoverErr := scopemcp.DiscoverTools(ctx, []scopemcp.ToolSource{source}, scopemcp.ToolDiscoveryConfig{
		PublicName: func(_ string, toolName string) string {
			remoteName, err := mcpserver.ParseRemoteToolName(toolName)
			if err != nil {
				if remoteNameErr == nil {
					remoteNameErr = err
				}
				// Scope requires a provider-safe label before it returns wrappers.
				// The captured identity error wins below, so this label can never
				// enter either live catalog.
				return rejectedRemoteToolPlaceholder
			}
			ref, _ := domaintool.MCP(server, remoteName)
			return ref.DiscoveryName()
		},
		ConcurrencyPolicy: scopemcp.AnnotatedReadOnlyConcurrencyPolicy,
		RequestMeta: func(ctx context.Context, _ toolcontract.Invocation) sdkmcp.Meta {
			captured, _ := ctx.Value(callKey).(sdkmcp.Meta)
			return sdkmcp.Meta(metadata.Map(captured).Clone())
		},
	})
	if remoteNameErr != nil {
		return nil, fmt.Errorf("mcp: validate tool from server %q: %w", server, remoteNameErr)
	}
	if discoverErr != nil {
		return nil, discoverErr
	}
	config := descriptor.Clone()
	config.OAuthHandler = nil
	result := make([]Executable, len(tools))
	for index, executable := range tools {
		result[index] = Executable{Tool: executable, config: config, owner: owner, session: session, callKey: callKey}
	}
	if err := validateSourceToolMaterial(server, result); err != nil {
		return nil, err
	}
	return result, nil
}

func validateSourceToolMaterial(server mcpserver.ID, tools []Executable) error {
	if err := mcpserver.ValidateRemoteToolCount(len(tools)); err != nil {
		return fmt.Errorf("mcp: validate tools from server %q: %w", server, err)
	}
	for _, tool := range tools {
		ref, found, err := IdentifyTool(tool)
		if err != nil {
			return fmt.Errorf("mcp: validate tool from server %q: %w", server, err)
		}
		if !found {
			return fmt.Errorf("mcp: tool from server %q has no MCP identity", server)
		}
		_, remote, _ := ref.MCP()
		binding, err := toolcontract.Bind(tool)
		if err != nil {
			return fmt.Errorf("mcp: admit tool %q from server %q: %w", remote, server, err)
		}
		definition := binding.Contract().Definition()
		if err := mcpserver.ValidateRemoteToolDescription(definition.Description); err != nil {
			return fmt.Errorf("mcp: validate tool %q from server %q: %w", definition.Name, server, err)
		}
	}
	return nil
}

// Executable retains the configuration and the session that admitted it. It
// cannot advance registry configuration or the connection's live OAuth
// credentials.
// sourceCallMetadata is a private transport key bound to this discovery. The
// executable supplies explicitly captured wire parameters for one synchronous
// call; no ambient context metadata or current host identity is consulted.
type sourceCallMetadata struct{ marker byte }

type Executable struct {
	toolcontract.Tool
	config         ServerConfig
	owner          *connections
	session        *sdkmcp.ClientSession
	held           *BorrowedSession
	catalogVersion uint64
	callKey        *sourceCallMetadata
	callMetadata   sdkmcp.Meta
}

func (t Executable) Call(ctx context.Context, invocation toolcontract.Invocation) (_ toolcontract.Result, err error) {
	borrowed, err := t.BorrowSession(ctx)
	if err != nil {
		return toolcontract.Result{}, err
	}
	defer func() { err = errors.Join(err, borrowed.Close()) }()
	callCtx := borrowed.Context()
	if t.callKey != nil {
		callCtx = context.WithValue(callCtx, t.callKey, t.callMetadata)
	}
	return t.Tool.Call(callCtx, invocation)
}

// Current reports whether this executable's admitted catalog revision still
// belongs to its source's live session. A frozen Run manifest outlives catalog
// changes, reconnects and refusals; it cannot silently adopt a newer contract.
func (t Executable) Current() bool {
	if t.held != nil {
		return t.held.ctx.Err() == nil
	}
	return t.owner != nil && t.owner.current(t.config.ID(), t.session, t.catalogVersion)
}

// WithCallMetadata creates one call's immutable metadata projection. Its
// caller must retain and release any callback grant for the whole call.
func (t Executable) WithCallMetadata(params sdkmcp.ParamsBase) Executable {
	combined := metadata.Map(t.callMetadata).Clone()
	if combined == nil {
		combined = make(metadata.Map)
	}
	for key, value := range params.Meta {
		combined[key] = append([]byte(nil), value...)
	}
	t.callMetadata = sdkmcp.Meta(combined)
	return t
}

func (t Executable) MCPSource() (CapturedSource, bool) {
	captured := CapturedSource{Config: t.SourceConfig()}
	if identity, ok := t.Tool.(remoteToolIdentity); ok {
		_, captured.RemoteName = identity.MCPToolIdentity()
	}
	return captured, true
}
func (t Executable) ConcurrencyPolicy() func(toolcontract.Invocation) (string, bool) {
	if declared, ok := t.Tool.(interaction.ConcurrentTool); ok {
		return declared.ConcurrencyPolicy()
	}
	return nil
}
func (t Executable) SourceFingerprint() framevalue.Digest { return t.config.AuthorityFingerprint() }
func (t Executable) SourceConfig() ServerConfig           { return t.config.Clone() }

// BorrowSession holds this executable's original Use while callback grants are
// being prepared and used. The caller closes it after revoking those grants.
func (t Executable) BorrowSession(ctx context.Context) (*BorrowedSession, error) {
	if t.held != nil {
		if t.held.ctx.Err() != nil {
			return nil, ErrUnavailable
		}
		work, err := t.held.managed.use.Lifetime().Enter(ctx)
		if err != nil {
			return nil, err
		}
		borrowed := &BorrowedSession{ctx: work.Context(), session: t.session, work: work, managed: t.held.managed}
		borrowed.ctx = context.WithValue(borrowed.ctx, borrowContextKey{}, borrowed)
		return borrowed, nil
	}
	if !t.Current() {
		return nil, ErrUnavailable
	}
	return t.owner.borrowSession(ctx, t.session)
}

func (t Executable) InstallationActivation() (plugin.Activation, error) {
	if t.held != nil {
		return t.held.Activation()
	}
	if t.owner == nil {
		return plugin.Activation{}, ErrUnavailable
	}
	return t.owner.activation(t.session)
}

package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/core/metadata"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

const rejectedRemoteToolPlaceholder = "invalid_remote_tool"

func sourceTools(ctx context.Context, owner *connections, descriptor ServerConfig, session *sdk.ClientSession) ([]Executable, error) {
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
				return rejectedRemoteToolPlaceholder
			}
			ref, _ := domaintool.MCP(server, remoteName)
			return ref.DiscoveryName()
		},
		// An installed package's read-only declaration is its own Tool contract;
		// any other server's annotation is an untrusted hint.
		EffectPolicy: func(_, _ string, annotations sdk.ToolAnnotations) toolcontract.Effect {
			if _, installed := server.Origin().Installation(); installed && annotations.ReadOnlyHint {
				return toolcontract.EffectSafe
			}
			return toolcontract.EffectUnsafe
		},
		ConcurrencyPolicy: scopemcp.AnnotatedReadOnlyConcurrencyPolicy,
		RequestMeta: func(ctx context.Context, _ toolcontract.Invocation) sdk.Meta {
			captured, _ := ctx.Value(callKey).(sdk.Meta)
			return sdk.Meta(metadata.Map(captured).Clone())
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

type sourceCallMetadata struct{ _ byte }

type Executable struct {
	toolcontract.Tool
	config         ServerConfig
	owner          *connections
	session        *sdk.ClientSession
	held           *BorrowedSession
	catalogVersion uint64
	callKey        *sourceCallMetadata
	callMetadata   sdk.Meta
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

func (t Executable) Current() bool {
	if t.held != nil {
		return t.held.ctx.Err() == nil
	}
	return t.owner != nil && t.owner.current(t.config.ID(), t.session, t.catalogVersion)
}

func (t Executable) WithCallMetadata(params sdk.ParamsBase) Executable {
	combined := metadata.Map(t.callMetadata).Clone()
	if combined == nil {
		combined = make(metadata.Map)
	}
	for key, value := range params.Meta {
		combined[key] = append([]byte(nil), value...)
	}
	t.callMetadata = sdk.Meta(combined)
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

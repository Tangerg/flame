package toolset

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/agent/interaction"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Tool decorators change execution or presentation, never the source's declared
// admission, scheduling, mutation, or connection authority. These are explicit
// consumer contracts; unrelated wrapper internals are not traversed.
type declaredTool struct {
	toolcontract.Tool
	declarations toolcontract.Tool
	identity     toolIdentity
	authority    sourceAuthority
	activation   installationActivation
}

func preserveDeclarations(wrapped, original toolcontract.Tool) toolcontract.Tool {
	result := declaredTool{Tool: wrapped, declarations: original}
	result.identity, _ = wrapped.(toolIdentity)
	if result.identity == nil {
		result.identity, _ = original.(toolIdentity)
	}
	result.authority, _ = wrapped.(sourceAuthority)
	if result.authority == nil {
		result.authority, _ = original.(sourceAuthority)
	}
	result.activation, _ = wrapped.(installationActivation)
	if result.activation == nil {
		result.activation, _ = original.(installationActivation)
	}
	if advertising, ok := original.(interaction.AdvertisingTool); ok {
		return advertisingTool{declaredTool: result, advertising: advertising}
	}
	return result
}

func (t declaredTool) ToolRef() domaintool.Ref {
	if t.identity != nil {
		return t.identity.ToolRef()
	}
	return domaintool.Ref{}
}
func (t declaredTool) SourceFingerprint() framevalue.Digest {
	if t.authority != nil {
		return t.authority.SourceFingerprint()
	}
	return framevalue.Digest{}
}
func (t declaredTool) MCPSource() (mcp.CapturedSource, bool) { return mcp.Source(t.declarations) }
func (t declaredTool) InstallationActivation() (plugin.Activation, error) {
	if t.activation != nil {
		return t.activation.InstallationActivation()
	}
	return plugin.Activation{}, fmt.Errorf("toolset: tool has no installation activation")
}
func (t declaredTool) CallActivation(ctx context.Context, activation plugin.Activation, invocation toolcontract.Invocation) (toolcontract.Result, error) {
	source, ok := t.declarations.(activationCaller)
	if !ok {
		return toolcontract.Result{}, fmt.Errorf("toolset: tool has no retained binding")
	}
	return source.CallActivation(ctx, activation, invocation)
}
func (t declaredTool) InputValidator() func([]byte) error {
	if declaration, ok := t.declarations.(toolcontract.InputValidatingTool); ok {
		return declaration.InputValidator()
	}
	return nil
}
func (t declaredTool) ConcurrencyPolicy() func(toolcontract.Invocation) (string, bool) {
	if declaration, ok := t.declarations.(interaction.ConcurrentTool); ok {
		return declaration.ConcurrencyPolicy()
	}
	return nil
}
func (t declaredTool) ReturnsDirectResult() bool {
	declaration, ok := t.declarations.(interaction.DirectResultTool)
	return ok && declaration.ReturnsDirectResult()
}
func (t declaredTool) MutationPaths(arguments []byte) ([]string, error) {
	if declaration, ok := t.declarations.(FileMutationReporter); ok {
		return declaration.MutationPaths(arguments)
	}
	return nil, nil
}

type advertisingTool struct {
	declaredTool
	advertising interaction.AdvertisingTool
}

func (t advertisingTool) AdvertiseDeferred(ctx context.Context, invocation toolcontract.Invocation, result toolcontract.Result) ([]string, error) {
	return t.advertising.AdvertiseDeferred(ctx, invocation, result)
}
func (t advertisingTool) DeferredToolNames() []string {
	if source, ok := t.advertising.(interface{ DeferredToolNames() []string }); ok {
		return source.DeferredToolNames()
	}
	return nil
}

type activationCaller interface {
	CallActivation(context.Context, plugin.Activation, toolcontract.Invocation) (toolcontract.Result, error)
}

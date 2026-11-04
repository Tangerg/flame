package toolset

import (
	"context"
	"fmt"
	"slices"

	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type toolIdentity interface{ ToolRef() tool.Ref }
type sourceAuthority interface{ SourceFingerprint() framevalue.Digest }

// Identify resolves source-owned identity from explicit source declarations.
// A model label is never a fallback identity for an unrecognized executable.
func Identify(executable toolcontract.Tool) (tool.Ref, error) {
	ref, found, err := mcp.IdentifyTool(executable)
	if err != nil || found {
		return ref, err
	}
	identity, found := executable.(toolIdentity)
	if !found {
		return tool.Ref{}, fmt.Errorf("toolset: executable has no source identity")
	}
	ref = identity.ToolRef()
	return ref, ref.Validate()
}

func SourceFingerprint(executable toolcontract.Tool, ref tool.Ref) (framevalue.Digest, error) {
	authority, found := executable.(sourceAuthority)
	var source framevalue.Digest
	if found {
		source = authority.SourceFingerprint()
	}
	return source, ref.ValidateFingerprint(source)
}

// WithIdentity binds a local source at construction. Connected MCP
// identity and authority stay on Scope's connection-backed executable.
func WithIdentity(executable toolcontract.Tool, ref tool.Ref, source framevalue.Digest) (toolcontract.Tool, error) {
	if err := ref.ValidateFingerprint(source); err != nil {
		return nil, err
	}
	if executable == nil || executable.Definition().Name != ref.ModelName() {
		return nil, fmt.Errorf("toolset: definition differs from source reference %s", ref)
	}
	return preserveDeclarations(identifiedTool{Tool: executable, ref: ref, fingerprint: source}, executable), nil
}

type identifiedTool struct {
	toolcontract.Tool
	ref         tool.Ref
	fingerprint framevalue.Digest
}

func (t identifiedTool) ToolRef() tool.Ref                    { return t.ref }
func (t identifiedTool) SourceFingerprint() framevalue.Digest { return t.fingerprint }

func CaptureToolCaller(executable toolcontract.Tool, invocation framemcp.Invocation) (plugin.ToolCaller, error) {
	ref, err := Identify(executable)
	if err != nil {
		return plugin.ToolCaller{}, err
	}
	source, found := mcp.Source(executable)
	if !found {
		return plugin.ToolCaller{}, fmt.Errorf("toolset: installed Tool has no captured source")
	}
	config := source.SourceConfig()
	server, _, mcp := ref.MCP()
	if !mcp || server != config.ID() {
		return plugin.ToolCaller{}, fmt.Errorf("toolset: installed Tool differs from its captured server")
	}
	release, authority, installed := config.Source.Release()
	if !installed {
		return plugin.ToolCaller{}, fmt.Errorf("toolset: Tool has no installed caller")
	}
	return plugin.NewToolCaller(plugin.ToolCallerSnapshot{Invocation: invocation, Tool: ref, Release: release, Authority: authority})
}

// InstallationDependency reports the installation release an executable realizes.
// The release comes from the executable's own connection configuration, never
// from the installation's current selection, which may already have moved on.
func InstallationDependency(executable toolcontract.Tool) (plugin.Dependency, bool, error) {
	ref, err := Identify(executable)
	if err != nil {
		return plugin.Dependency{}, false, err
	}
	server, _, ok := ref.MCP()
	if !ok {
		return plugin.Dependency{}, false, nil
	}
	installation, installed := server.Origin().Installation()
	if !installed {
		return plugin.Dependency{}, false, nil
	}
	source, found := mcp.Source(executable)
	if !found {
		return plugin.Dependency{}, false, fmt.Errorf("toolset: installation tool %s has no realized release", ref)
	}
	release, _, found := source.SourceConfig().Source.Release()
	if !found {
		return plugin.Dependency{}, false, fmt.Errorf("toolset: installation tool %s has no realized release", ref)
	}
	dependency := plugin.Dependency{InstallationID: installation, Digest: release}
	if err := dependency.Validate(); err != nil {
		return plugin.Dependency{}, false, fmt.Errorf("toolset: installation tool %s: %w", ref, err)
	}
	return dependency, true, nil
}

type installationActivation interface {
	InstallationActivation() (plugin.Activation, error)
}

// The source owns the activation graph; the manifest only freezes its projection.
func InstallationActivation(executable toolcontract.Tool) (plugin.Activation, error) {
	root, installed, err := InstallationDependency(executable)
	if err != nil || !installed {
		return plugin.Activation{}, err
	}
	source, found := executable.(installationActivation)
	if !found {
		return plugin.Activation{}, fmt.Errorf("toolset: installation tool has no activation proof")
	}
	activation, err := source.InstallationActivation()
	if err != nil {
		return plugin.Activation{}, err
	}
	if err := activation.Validate(); err != nil {
		return plugin.Activation{}, err
	}
	if !slices.Contains(activation.Dependencies, root) {
		return plugin.Activation{}, fmt.Errorf("toolset: activation omits its source release")
	}
	return activation.Clone(), nil
}

// A manifest's graph identity also fences same-release reconnects whose selected
// contributions or decorator ordering changed while execution was waiting.
type frozenInstallationTool struct {
	toolcontract.Tool
	activation plugin.Activation
}

func (t frozenInstallationTool) InstallationActivation() (plugin.Activation, error) {
	return t.activation.Clone(), nil
}
func (t frozenInstallationTool) Call(ctx context.Context, invocation toolcontract.Invocation) (toolcontract.Result, error) {
	original, found := t.Tool.(activationCaller)
	if !found {
		return toolcontract.Result{}, fmt.Errorf("toolset: installed Tool does not support its retained binding")
	}
	return original.CallActivation(ctx, t.activation.Clone(), invocation)
}

package toolset

import (
	"fmt"
	"slices"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framevalue "github.com/Tangerg/frame/value"
	toolcontract "github.com/Tangerg/scope/core/tool"
	oteltool "github.com/Tangerg/scope/otel/tool"
)

type Manifest struct {
	SkillDiscovery string
	Installations  []plugin.Dependency
	Bindings       []plugin.Activation
	Resources      []plugin.Dependency
	Visible        []toolcontract.Tool
	Deferred       []toolcontract.Tool
	PendingMCP     []mcpserver.ID
	discovery      *Discovery
	close          func() error
}

func (m Manifest) Clone() Manifest {
	return Manifest{SkillDiscovery: m.SkillDiscovery,
		Installations: slices.Clone(m.Installations), Visible: slices.Clone(m.Visible),
		Bindings: cloneManifestBindings(m.Bindings), Resources: slices.Clone(m.Resources),
		Deferred:   slices.Clone(m.Deferred),
		PendingMCP: slices.Clone(m.PendingMCP),
		discovery:  m.discovery,
		close:      m.close,
	}
}

func (m Manifest) DeferredCatalog() string {
	if m.discovery == nil {
		return ""
	}
	return m.discovery.Catalog()
}

func (m Manifest) Close() error {
	if m.close == nil {
		return nil
	}
	return m.close()
}

type manifestBuilder struct {
	skillDiscovery promptsource.SkillCapture
	pendingMCP     []mcpserver.ID
	installations  []plugin.Dependency
	visible        []toolcontract.Tool
	deferred       []toolcontract.Tool
	discovery      *Discovery
	close          func() error
	err            error
}

func (m *manifestBuilder) direct(tools ...toolcontract.Tool) {
	for _, candidate := range tools {
		if candidate != nil {
			m.visible = append(m.visible, m.builtIn(candidate))
		}
	}
}

func (m *manifestBuilder) deferTools(tools ...toolcontract.Tool) {
	for _, candidate := range tools {
		if candidate != nil {
			m.deferred = append(m.deferred, m.builtIn(candidate))
		}
	}
}

func (m *manifestBuilder) manifest(telemetry oteltool.Middleware) (Manifest, error) {
	if m.err != nil {
		return Manifest{}, m.err
	}
	installations := slices.Clone(m.installations)
	var bindings []plugin.Activation
	if !m.skillDiscovery.Activation.Identity().IsZero() {
		bindings = append(bindings, m.skillDiscovery.Activation)
		installations = append(installations, m.skillDiscovery.Activation.Dependencies...)
	}
	freeze := func(tools []toolcontract.Tool) ([]toolcontract.Tool, error) {
		frozen := make([]toolcontract.Tool, len(tools))
		for index, executable := range tools {
			activation, err := InstallationActivation(executable)
			if err != nil {
				return nil, err
			}
			installations = append(installations, activation.Dependencies...)
			if !activation.Identity().IsZero() {
				if !slices.ContainsFunc(bindings, activation.Equal) {
					bindings = append(bindings, activation.Clone())
				}
				executable = frozenInstallationTool{Tool: executable, activation: activation.Clone()}
			}
			frozen[index] = toolcontract.Wrap(executable, telemetry.Call)
		}
		return frozen, nil
	}
	visible, err := freeze(m.visible)
	if err != nil {
		return Manifest{}, err
	}
	deferred, err := freeze(m.deferred)
	if err != nil {
		return Manifest{}, err
	}

	return Manifest{SkillDiscovery: m.skillDiscovery.Command, Installations: plugin.CompactDependencies(installations), Bindings: bindings, Resources: plugin.CompactDependencies(m.installations), Visible: visible, Deferred: deferred, PendingMCP: slices.Clone(m.pendingMCP), discovery: m.discovery, close: m.close}, nil
}

func cloneManifestBindings(values []plugin.Activation) []plugin.Activation {
	result := slices.Clone(values)
	for index := range result {
		result[index] = result[index].Clone()
	}
	return result
}

func (m *manifestBuilder) builtIn(executable toolcontract.Tool) toolcontract.Tool {
	ref, err := tool.BuiltIn(builtincontract.Name(executable.Definition().Name))
	if err != nil {
		m.err = err
		return executable
	}
	identified, err := WithIdentity(executable, ref, framevalue.Digest{})
	if err != nil {
		m.err = err
		return executable
	}
	return identified
}

type namedRemoteTool struct {
	toolcontract.Tool
	name string
}

func (t namedRemoteTool) Definition() toolcontract.Definition {
	definition := t.Tool.Definition()
	definition.Name = t.name
	return definition
}

func (t namedRemoteTool) Unwrap() toolcontract.Tool { return t.Tool }

func (m *manifestBuilder) bindModelNames(retained *Retained) error {
	if m.err != nil {
		return m.err
	}
	var refs []tool.Ref
	if retained != nil {
		refs = slices.Clone(retained.MCP)
	}
	for _, set := range [][]toolcontract.Tool{m.visible, m.deferred} {
		for _, executable := range set {
			ref, err := Identify(executable)
			if err != nil {
				return err
			}
			refs = append(refs, ref)
		}
	}
	conflicts := tool.NameConflicts(refs)
	assigned := make(map[string]tool.Ref)
	for _, set := range [][]toolcontract.Tool{m.visible, m.deferred} {
		for index, executable := range set {
			ref, err := Identify(executable)
			if err != nil {
				return err
			}
			name := ref.NameForCatalog(conflicts)
			if other, found := assigned[name]; found {
				return fmt.Errorf("toolset: duplicate published name %q for %s and %s", name, other, ref)
			}
			assigned[name] = ref
			if name != executable.Definition().Name {
				set[index] = namedRemoteTool{Tool: executable, name: name}
			}
		}
	}
	return nil
}

func (r *Resolver) MCPTools(server *mcpserver.ID) ([]mcpserver.AdvertisedTool, map[tool.Ref][]tool.Ref, error) {
	snapshot := r.mcpSnapshot()
	exposed, err := r.exposedMCPTools(snapshot, nil)
	if err != nil {
		return nil, nil, err
	}
	refs := make([]tool.Ref, 0, len(exposed))
	for _, executable := range exposed {
		ref, err := Identify(executable)
		if err != nil {
			return nil, nil, err
		}
		refs = append(refs, ref)
	}
	advertised := make([]mcpserver.AdvertisedTool, 0, len(snapshot))
	for _, executable := range snapshot {
		ref, _, err := mcp.IdentifyTool(executable)
		if err != nil {
			return nil, nil, err
		}
		source, remote, _ := ref.MCP()
		if server != nil && source != *server {
			continue
		}
		advertised = append(advertised, mcpserver.AdvertisedTool{Server: source, Name: remote, Definition: executable.Definition()})
	}
	return advertised, tool.NameConflicts(refs), nil
}

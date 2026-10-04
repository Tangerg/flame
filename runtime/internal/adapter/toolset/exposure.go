package toolset

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	toolcontract "github.com/Tangerg/scope/core/tool"
	oteltool "github.com/Tangerg/scope/otel/tool"
)

// Manifest is one Run's frozen, framework-neutral model Tool surface. Visible
// Tools enter the initial model manifest. Deferred Tools are already executable
// authority but remain hidden until the discovery Tool advertises their exact
// names. The slices never overlap. Its caller owns Close after all Tool calls
// have drained; copied manifests share the same resource lifetime.
type Manifest struct {
	Installations []promptsource.InstallationDependency
	Visible       []toolcontract.Tool
	Deferred      []toolcontract.Tool
	close         func() error
}

// Clone isolates the slices while retaining the same executable capabilities
// and resource lifetime. It does not grant an independent Close obligation.
func (m Manifest) Clone() Manifest {
	return Manifest{
		Installations: slices.Clone(m.Installations), Visible: slices.Clone(m.Visible),
		Deferred: slices.Clone(m.Deferred),
		close:    m.close,
	}
}

// Close releases the manifest's filesystem authority once. It is safe to call
// again, including through a clone, after the execution owner has drained calls.
func (m Manifest) Close() error {
	if m.close == nil {
		return nil
	}
	return m.close()
}

// manifestBuilder owns the one real visibility decision made while assembling a
// Run: direct tools enter the initial model manifest, while deferred tools stay
// executable but are loaded through search_tools. Unavailable tools are simply
// never added; there is no synthetic visibility state for them.
type manifestBuilder struct {
	installations []promptsource.InstallationDependency
	visible       []toolcontract.Tool
	deferred      []toolcontract.Tool
	close         func() error
	err           error
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

// manifest freezes the surface and instruments every Tool on it. Scope's Tool
// telemetry owns the call boundary's span and duration without recording model
// arguments or results, and keeps the capability chain reachable through
// Unwrap, so identity resolution still sees the original Tool.
func (m manifestBuilder) manifest(telemetry oteltool.Middleware) (Manifest, error) {
	if m.err != nil {
		return Manifest{}, m.err
	}
	visible, err := instrumentTools(telemetry, m.visible)
	if err != nil {
		return Manifest{}, err
	}
	deferred, err := instrumentTools(telemetry, m.deferred)
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{Installations: slices.Clone(m.installations), Visible: visible, Deferred: deferred, close: m.close}, nil
}

func instrumentTools(telemetry oteltool.Middleware, tools []toolcontract.Tool) ([]toolcontract.Tool, error) {
	instrumented := make([]toolcontract.Tool, 0, len(tools))
	for _, candidate := range tools {
		wrapped, err := telemetry.Wrap(candidate)
		if err != nil {
			return nil, fmt.Errorf("toolset: instrument tool %q: %w", candidate.Definition().Name, err)
		}
		instrumented = append(instrumented, wrapped)
	}
	return instrumented, nil
}

func (m *manifestBuilder) builtIn(executable toolcontract.Tool) toolcontract.Tool {
	ref, err := tool.BuiltIn(executable.Definition().Name)
	if err != nil {
		m.err = err
		return executable
	}
	identified, err := WithIdentity(executable, ref, "")
	if err != nil {
		m.err = err
		return executable
	}
	return identified
}

// Filter before constructing discovery so excluded tools cannot be advertised.
func (m *manifestBuilder) excludeCollisions(ctx context.Context) error {
	if m.err != nil {
		return m.err
	}
	refs := make([]tool.Ref, 0, len(m.visible)+len(m.deferred))
	for _, executable := range append(slices.Clone(m.visible), m.deferred...) {
		ref, err := Identify(executable)
		if err != nil {
			return err
		}
		refs = append(refs, ref)
	}
	conflicts := tool.NameConflicts(refs)
	for ref, others := range conflicts {
		slog.WarnContext(ctx, "toolset: excluded colliding remote tool", "tool", ref, "conflicts", others)
	}
	remove := func(executable toolcontract.Tool) bool {
		ref, _ := Identify(executable)
		return len(conflicts[ref]) != 0
	}
	m.visible = slices.DeleteFunc(m.visible, remove)
	m.deferred = slices.DeleteFunc(m.deferred, remove)
	return nil
}

// ToolNameConflicts projects the same exclusion rule used by each frozen manifest
// over the current remote catalog, including exposure changes and A2A sources.
func (r *Resolver) ToolNameConflicts() (map[tool.Ref][]tool.Ref, error) {
	mcpTools, err := r.mcpTools()
	if err != nil {
		return nil, err
	}
	refs := make([]tool.Ref, 0, len(mcpTools)+len(r.a2a))
	for _, executable := range append(slices.Clone(mcpTools), r.a2a...) {
		ref, err := Identify(executable)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return tool.NameConflicts(refs), nil
}

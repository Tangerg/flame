package toolset

import (
	"fmt"
	"slices"

	toolcontract "github.com/Tangerg/scope/core/tool"
	oteltool "github.com/Tangerg/scope/otel/tool"
)

// Manifest is one Run's frozen, framework-neutral model Tool surface. Visible
// Tools enter the initial model manifest. Deferred Tools are already executable
// authority but remain hidden until the discovery Tool advertises their exact
// names. The slices never overlap. Its caller owns Close after all Tool calls
// have drained; copied manifests share the same resource lifetime.
type Manifest struct {
	Visible  []toolcontract.Tool
	Deferred []toolcontract.Tool
	close    func() error
}

// Clone isolates the slices while retaining the same executable capabilities
// and resource lifetime. It does not grant an independent Close obligation.
func (m Manifest) Clone() Manifest {
	return Manifest{
		Visible:  slices.Clone(m.Visible),
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
	visible  []toolcontract.Tool
	deferred []toolcontract.Tool
	close    func() error
}

func (m *manifestBuilder) direct(tools ...toolcontract.Tool) {
	for _, candidate := range tools {
		if candidate != nil {
			m.visible = append(m.visible, candidate)
		}
	}
}

func (m *manifestBuilder) deferTools(tools ...toolcontract.Tool) {
	for _, candidate := range tools {
		if candidate != nil {
			m.deferred = append(m.deferred, candidate)
		}
	}
}

// manifest freezes the surface and instruments every Tool on it. Scope's Tool
// telemetry owns the call boundary's span and duration without recording model
// arguments or results, and keeps the capability chain reachable through
// Unwrap, so identity resolution still sees the original Tool.
func (m manifestBuilder) manifest(telemetry oteltool.Middleware) (Manifest, error) {
	visible, err := instrumentTools(telemetry, m.visible)
	if err != nil {
		return Manifest{}, err
	}
	deferred, err := instrumentTools(telemetry, m.deferred)
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{Visible: visible, Deferred: deferred, close: m.close}, nil
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

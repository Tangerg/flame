// Package failure defines structured failure details shared by the CLI's
// bounded contexts.
//
// A problem is a CLI-owned aggregate, not the runtime wire type. It reuses the
// Runtime's exact recovery leaf values while owning CLI validation and
// presentation, so agent runs, MCP management, model configuration, terminal
// presentation, and machine renderers cannot silently disagree about recovery
// metadata.
package failure

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

// Problem is a structured failure with the information a client needs to
// explain the failure and offer an appropriate recovery action.
type Problem struct {
	Type                 string                                  `json:"type"`
	Detail               string                                  `json:"detail,omitempty"`
	DocURL               string                                  `json:"docUrl,omitempty"`
	RetryAfterSeconds    int                                     `json:"retryAfterSeconds,omitempty"`
	RequiredCapabilities []runtimeprotocol.CapabilityRequirement `json:"requiredCapabilities,omitempty"`
	ActiveRun            *runtimeprotocol.ActiveRunRef           `json:"activeRun,omitempty"`
	Errors               []runtimeprotocol.FieldError            `json:"errors,omitempty"`
}

// Validate checks the portable structure without copying the runtime's
// provider-extensible problem-type registry into the CLI core.
func (p Problem) Validate() error {
	var problems []error
	if strings.TrimSpace(p.Type) == "" {
		problems = append(problems, errors.New("type is empty"))
	}
	if p.RetryAfterSeconds < 0 {
		problems = append(problems, errors.New("retry delay is negative"))
	}
	seen := make(map[runtimeprotocol.CapabilityRequirement]struct{}, len(p.RequiredCapabilities))
	for index, requirement := range p.RequiredCapabilities {
		if err := requirement.ValidateWire(); err != nil {
			problems = append(problems, fmt.Errorf("required capability %d: %w", index+1, err))
		}
		if _, duplicate := seen[requirement]; duplicate {
			problems = append(problems, fmt.Errorf("required capability %d duplicates %s:%s", index+1, requirement.Type, requirement.Name))
		}
		seen[requirement] = struct{}{}
	}
	if p.ActiveRun != nil {
		if err := p.ActiveRun.ValidateWire(); err != nil {
			problems = append(problems, err)
		}
	}
	for index, field := range p.Errors {
		if err := field.ValidateWire(); err != nil {
			problems = append(problems, fmt.Errorf("field error %d: %w", index+1, err))
		}
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("problem: %w", err)
	}
	return nil
}

// Clone returns an independently owned problem. It is safe on a nil receiver.
func (p *Problem) Clone() *Problem {
	if p == nil {
		return nil
	}
	cloned := *p
	cloned.RequiredCapabilities = slices.Clone(p.RequiredCapabilities)
	cloned.Errors = slices.Clone(p.Errors)
	if p.ActiveRun != nil {
		cloned.ActiveRun = new(*p.ActiveRun)
	}
	return &cloned
}

// Equal reports whether two optional problems carry the same information.
func (p *Problem) Equal(other *Problem) bool {
	if p == nil || other == nil {
		return p == other
	}
	if p.Type != other.Type || p.Detail != other.Detail || p.DocURL != other.DocURL ||
		p.RetryAfterSeconds != other.RetryAfterSeconds || !slices.Equal(p.RequiredCapabilities, other.RequiredCapabilities) ||
		!slices.Equal(p.Errors, other.Errors) || (p.ActiveRun == nil) != (other.ActiveRun == nil) {
		return false
	}
	return p.ActiveRun == nil || *p.ActiveRun == *other.ActiveRun
}

// Message returns the most useful concise explanation for status lines.
func (p Problem) Message(fallback string) string {
	if detail := strings.TrimSpace(p.Detail); detail != "" {
		return detail
	}
	if problemType := strings.TrimSpace(p.Type); problemType != "" {
		return problemType
	}
	return fallback
}

// String returns a complete, single-line human projection. Machine consumers
// marshal Problem directly and therefore retain the same information without
// parsing this text.
func (p Problem) String() string {
	parts := []string{p.Type}
	if detail := strings.TrimSpace(p.Detail); detail != "" {
		parts[0] += ": " + detail
	}
	if p.RetryAfterSeconds > 0 {
		parts = append(parts, fmt.Sprintf("retry after %ds", p.RetryAfterSeconds))
	}
	if p.DocURL != "" {
		parts = append(parts, "docs "+p.DocURL)
	}
	if len(p.RequiredCapabilities) > 0 {
		required := make([]string, 0, len(p.RequiredCapabilities))
		for _, capability := range p.RequiredCapabilities {
			required = append(required, string(capability.Type)+":"+capability.Name)
		}
		parts = append(parts, "requires "+strings.Join(required, ", "))
	}
	if p.ActiveRun != nil {
		parts = append(parts, fmt.Sprintf("active run %s (%s)", p.ActiveRun.RunID, p.ActiveRun.Status))
	}
	if len(p.Errors) > 0 {
		fields := make([]string, 0, len(p.Errors))
		for _, field := range p.Errors {
			fields = append(fields, field.Field+": "+field.Detail)
		}
		parts = append(parts, "fields "+strings.Join(fields, ", "))
	}
	return strings.Join(parts, " · ")
}

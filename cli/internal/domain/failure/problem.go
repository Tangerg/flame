// Package failure defines CLI presentation behavior for Runtime problems.
// Runtime Protocol remains the sole owner of the structured failure value and
// its validation contract.
package failure

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

// Validate delegates to the generated Runtime Protocol contract so the CLI
// cannot accept a problem shape the Runtime forbids.
func Validate(problem *runtimeprotocol.ProblemData) error {
	if problem == nil {
		return errors.New("problem is nil")
	}
	if err := runtimeprotocol.ValidateWireTree(*problem); err != nil {
		return fmt.Errorf("problem: %w", err)
	}
	return nil
}

// Clone returns an independently owned problem. It is safe on nil.
func Clone(problem *runtimeprotocol.ProblemData) *runtimeprotocol.ProblemData {
	if problem == nil {
		return nil
	}
	cloned := *problem
	cloned.RequiredCapabilities = slices.Clone(problem.RequiredCapabilities)
	cloned.Errors = slices.Clone(problem.Errors)
	if problem.ActiveRun != nil {
		cloned.ActiveRun = new(*problem.ActiveRun)
	}
	return &cloned
}

// Equal reports whether two optional problems carry the same information.
func Equal(left, right *runtimeprotocol.ProblemData) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.Type != right.Type || left.Detail != right.Detail || left.DocURL != right.DocURL ||
		left.RetryAfterSeconds != right.RetryAfterSeconds || !slices.Equal(left.RequiredCapabilities, right.RequiredCapabilities) ||
		!slices.Equal(left.Errors, right.Errors) || (left.ActiveRun == nil) != (right.ActiveRun == nil) {
		return false
	}
	return left.ActiveRun == nil || *left.ActiveRun == *right.ActiveRun
}

// Message returns the most useful concise explanation for status lines.
func Message(problem *runtimeprotocol.ProblemData, fallback string) string {
	if problem == nil {
		return fallback
	}
	if detail := strings.TrimSpace(problem.Detail); detail != "" {
		return detail
	}
	if problemType := strings.TrimSpace(problem.Type); problemType != "" {
		return problemType
	}
	return fallback
}

// String returns a complete, single-line human projection. Machine consumers
// marshal ProblemData directly and therefore retain the same information
// without parsing this text.
func String(problem *runtimeprotocol.ProblemData) string {
	if problem == nil {
		return ""
	}
	parts := []string{problem.Type}
	if detail := strings.TrimSpace(problem.Detail); detail != "" {
		parts[0] += ": " + detail
	}
	if problem.RetryAfterSeconds > 0 {
		parts = append(parts, fmt.Sprintf("retry after %ds", problem.RetryAfterSeconds))
	}
	if problem.DocURL != "" {
		parts = append(parts, "docs "+problem.DocURL)
	}
	if len(problem.RequiredCapabilities) > 0 {
		required := make([]string, 0, len(problem.RequiredCapabilities))
		for _, capability := range problem.RequiredCapabilities {
			required = append(required, string(capability.Type)+":"+capability.Name)
		}
		parts = append(parts, "requires "+strings.Join(required, ", "))
	}
	if problem.ActiveRun != nil {
		parts = append(parts, fmt.Sprintf("active run %s (%s)", problem.ActiveRun.RunID, problem.ActiveRun.Status))
	}
	if len(problem.Errors) > 0 {
		fields := make([]string, 0, len(problem.Errors))
		for _, field := range problem.Errors {
			fields = append(fields, field.Field+": "+field.Detail)
		}
		parts = append(parts, "fields "+strings.Join(fields, ", "))
	}
	return strings.Join(parts, " · ")
}

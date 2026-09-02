package tool

import (
	"fmt"
)

// FailureKind classifies why one Tool invocation did not produce a successful
// result. It is deliberately separate from Run failure taxonomy.
type FailureKind string

const (
	FailureInternal         FailureKind = "internal"
	FailureDenied           FailureKind = "denied_by_user"
	FailureExecution        FailureKind = "tool_failed"
	FailureChildRunCanceled FailureKind = "child_run_canceled"
	FailureCanceled         FailureKind = "tool_canceled"
)

// Valid reports whether f belongs to the durable Tool failure taxonomy.
func (f FailureKind) Valid() bool {
	switch f {
	case FailureInternal, FailureDenied, FailureExecution, FailureChildRunCanceled, FailureCanceled:
		return true
	default:
		return false
	}
}

// String returns the stable durable name of f.
func (f FailureKind) String() string {
	if !f.Valid() {
		return "unknown"
	}
	return string(f)
}

// Failure is the durable explanation attached to an incomplete ToolCall.
type Failure struct {
	Kind   FailureKind
	Detail string
	DocURL string
}

// Validate reports whether the failure is representable. Tool retry is an
// execution-policy decision rather than a durable failure fact.
func (f Failure) Validate() error {
	if !f.Kind.Valid() {
		return fmt.Errorf("tool: unknown failure kind %q", f.Kind)
	}
	return nil
}

// Equal reports whether both durable failures describe the same outcome.
func (f Failure) Equal(other Failure) bool { return f == other }

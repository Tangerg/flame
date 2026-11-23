package tool

import (
	"fmt"
)

type FailureKind string

const (
	FailureInternal         FailureKind = "internal_error"
	FailureDenied           FailureKind = "denied_by_user"
	FailureExecution        FailureKind = "tool_failed"
	FailureChildRunCanceled FailureKind = "child_run_canceled"
	FailureCanceled         FailureKind = "tool_canceled"
)

func (f FailureKind) Valid() bool {
	switch f {
	case FailureInternal, FailureDenied, FailureExecution, FailureChildRunCanceled, FailureCanceled:
		return true
	default:
		return false
	}
}

type Failure struct {
	Kind   FailureKind
	Detail string
	DocURL string
}

func (f Failure) Validate() error {
	if !f.Kind.Valid() {
		return fmt.Errorf("tool: unknown failure kind %q", f.Kind)
	}
	return nil
}

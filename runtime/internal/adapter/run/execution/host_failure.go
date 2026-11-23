package execution

import "errors"

// Scope settles any non-Failure Tool error as unknown, so this wrapper exposes no
// nested Failure or CallError: a host fault must never reach the model as the
// Tool's own result.
type hostFailureError struct{ diagnostic string }

func (h *hostFailureError) Error() string { return h.diagnostic }

func hostFailure(cause error) error {
	if cause == nil {
		return nil
	}
	var marked *hostFailureError
	if errors.As(cause, &marked) {
		return cause
	}
	return &hostFailureError{diagnostic: cause.Error()}
}

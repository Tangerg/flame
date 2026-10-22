package execution

import "errors"

// hostFailureError presents a Runtime, authorization, or hook fault to Scope as
// an unknown Tool effect. Scope settles any non-Failure error returned from a
// Tool as unknown, so this wrapper deliberately exposes no nested Failure or
// CallError: a host fault must never reach the model as the Tool's own result,
// even when the faulting collaborator also produced one. The diagnostic is
// retained for the Run's effect-failure record.
type hostFailureError struct{ diagnostic string }

func (h *hostFailureError) Error() string { return h.diagnostic }

// hostFailure marks cause as a host failure. A nil cause stays nil; an
// already-marked error is returned unchanged.
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

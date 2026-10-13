package conversation

import (
	"errors"
)

// AcceptedMutationError reports that a runtime mutation returned successfully
// but its receipt violated the client contract. Receipt retains every usable
// identity from that response so callers can clean up the accepted mutation
// without replaying user intent under a new command identity.
type AcceptedMutationError struct {
	receipt SegmentStream
	cause   error
}

func NewAcceptedMutationError(receipt SegmentStream, cause error) error {
	if cause == nil {
		return nil
	}
	return &AcceptedMutationError{receipt: receipt, cause: cause}
}

func (a *AcceptedMutationError) Error() string { return a.cause.Error() }

func (a *AcceptedMutationError) Unwrap() error { return a.cause }

// RetiredOpeningError confirms historical acceptance without granting access
// to any material that now reuses the original Run identity.
type RetiredOpeningError struct{ receipt SegmentStream }

func NewRetiredOpeningError(receipt SegmentStream) error {
	return &RetiredOpeningError{receipt: receipt}
}

func (r *RetiredOpeningError) Error() string { return "accepted run material has been retired" }

// AcceptedMutationReceipt extracts proof of acceptance, including a historical
// opening whose original material has been retired.
func AcceptedMutationReceipt(err error) (SegmentStream, bool) {
	if retired, ok := errors.AsType[*RetiredOpeningError](err); ok {
		return retired.receipt, true
	}
	accepted, ok := errors.AsType[*AcceptedMutationError](err)
	if !ok {
		return SegmentStream{}, false
	}
	return accepted.receipt, true
}

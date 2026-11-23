package conversation

import (
	"errors"
)

type acceptedMutationError struct {
	receipt SegmentStream
	cause   error
}

func NewAcceptedMutationError(receipt SegmentStream, cause error) error {
	if cause == nil {
		return nil
	}
	return &acceptedMutationError{receipt: receipt, cause: cause}
}

func (a *acceptedMutationError) Error() string { return a.cause.Error() }

func (a *acceptedMutationError) Unwrap() error { return a.cause }

type RetiredOpeningError struct{ receipt SegmentStream }

func NewRetiredOpeningError(receipt SegmentStream) error {
	return &RetiredOpeningError{receipt: receipt}
}

func (r *RetiredOpeningError) Error() string { return "accepted run material has been retired" }

func AcceptedMutationReceipt(err error) (SegmentStream, bool) {
	if retired, ok := errors.AsType[*RetiredOpeningError](err); ok {
		return retired.receipt, true
	}
	accepted, ok := errors.AsType[*acceptedMutationError](err)
	if !ok {
		return SegmentStream{}, false
	}
	return accepted.receipt, true
}

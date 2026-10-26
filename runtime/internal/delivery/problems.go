package delivery

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func runtimeProduced(detail string, cause error) *protocol.Failure {
	return protocol.NewFailure(errors.Join(protocol.ErrInternalError, cause), "the runtime produced "+detail)
}
func failureFromData(data protocol.ProblemData) *protocol.Failure {
	failure, err := protocol.DecodeFailure(data)
	if err != nil {
		return protocol.NewFailure(protocol.ErrInternalError, "the runtime could not complete the request")
	}
	return failure
}

func projectError(err error) *protocol.Failure {
	if errors.Is(err, extension.ErrInvocationConflict) {
		return protocol.NewFailure(errors.Join(protocol.ErrIdempotencyConflict, err), "idempotency key already binds a different command")
	}
	return protocol.ProjectError(err)
}

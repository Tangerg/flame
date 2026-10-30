package delivery

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/portable"
)

func pluginProviderErrors(other ...string) []string {
	return append([]string{protocol.ErrPluginUnavailable.Error(), protocol.ErrPluginStale.Error()}, other...)
}

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
	if errors.Is(err, portable.ErrInvocationConflict) {
		return protocol.NewFailure(errors.Join(protocol.ErrIdempotencyConflict, err), "idempotency key already binds a different command")
	}
	if _, found := errors.AsType[*protocol.Failure](err); found {
		return protocol.ProjectError(err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return protocol.ProjectError(err)
	}
	return protocol.ProjectError(wirePluginError(err))
}

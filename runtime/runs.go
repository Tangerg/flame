package runtime

import (
	"context"
	"iter"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// StartRun opens a root Run and returns its live event stream.
func (r *binding) StartRun(ctx context.Context, request protocol.StartRunRequest, options RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return r.invokeStream[protocol.StartRunRequest, *protocol.StartRunResponse, protocol.RunEvent](ctx, delivery.RunsStart, request, runCommandOptions(options))
}

// ResumeRun answers a waiting Run tree and returns its next Segment stream.
func (r *binding) ResumeRun(ctx context.Context, request protocol.ResumeRunRequest, options RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return r.invokeStream[protocol.ResumeRunRequest, *protocol.ResumeRunResponse, protocol.RunEvent](ctx, delivery.RunsResume, request, runCommandOptions(options))
}

// SubscribeRun attaches to one live root Segment, optionally after a replay cursor.
func (r *binding) SubscribeRun(ctx context.Context, request protocol.SubscribeRunRequest, options RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return r.invokeStream[protocol.SubscribeRunRequest, *protocol.SubscribeRunResponse, protocol.RunEvent](ctx, delivery.RunsSubscribe, request, runSubscriptionOptions(options))
}

// CancelRun requests cancellation of a Run or Run subtree.
func (r *binding) CancelRun(ctx context.Context, request protocol.CancelRunRequest, options CommandOptions) (*protocol.CancelRunResponse, error) {
	return r.invoke[protocol.CancelRunRequest, *protocol.CancelRunResponse](ctx, delivery.RunsCancel, request, commandOptions(options))
}

// SteerRun queues an instruction at the addressed Segment's next safe boundary.
func (r *binding) SteerRun(ctx context.Context, request protocol.SteerRunRequest, options CommandOptions) (*protocol.SteerRunResponse, error) {
	return r.invoke[protocol.SteerRunRequest, *protocol.SteerRunResponse](ctx, delivery.RunsSteer, request, commandOptions(options))
}

// GetRun returns one Run projection by identity.
func (r *binding) GetRun(ctx context.Context, request protocol.GetRunRequest, options CallOptions) (*protocol.RunRef, error) {
	return r.invoke[protocol.GetRunRequest, *protocol.RunRef](ctx, delivery.RunsGet, request, callOptions(options))
}

// ListRuns returns one cursor page of Runs.
func (r *binding) ListRuns(ctx context.Context, request protocol.ListRunsRequest, options CallOptions) (*protocol.Page[protocol.RunRef], error) {
	return r.invoke[protocol.ListRunsRequest, *protocol.Page[protocol.RunRef]](ctx, delivery.RunsList, request, callOptions(options))
}

// ListModelInvocations returns one cursor page of recorded provider attempts for a Run.
func (r *binding) ListModelInvocations(ctx context.Context, request protocol.ListModelInvocationsRequest, options CallOptions) (*protocol.Page[protocol.ModelInvocation], error) {
	return r.invoke[protocol.ListModelInvocationsRequest, *protocol.Page[protocol.ModelInvocation]](ctx, delivery.ModelInvocationsList, request, callOptions(options))
}

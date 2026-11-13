package bootstrap

import (
	"context"

	"github.com/Tangerg/frame/process"
	"github.com/Tangerg/frame/process/runner"
)

type processRunner struct {
	process.Runner
	creating chan struct{}
}

func newProcessRunner(next process.Runner) *processRunner {
	return &processRunner{Runner: next, creating: make(chan struct{}, 1)}
}

func (r *processRunner) Launch(ctx context.Context, request runner.LaunchRequest) (runner.LaunchResult, error) {
	if ctx == nil || ctx.Err() != nil {
		return r.Runner.Launch(ctx, request)
	}
	// The authority serializes creation in its durable journal. Keep requests
	// cancelable here until their original RPC can own that same serial capacity.
	select {
	case r.creating <- struct{}{}:
		defer func() { <-r.creating }()
	case <-ctx.Done():
		return r.Runner.Launch(ctx, request)
	}
	if ctx.Err() != nil {
		return r.Runner.Launch(ctx, request)
	}
	// Frame has durably admitted this creation. Join its original local RPC so
	// cancellation cannot discard the acknowledgement before Frame retires it.
	return r.Runner.Launch(context.WithoutCancel(ctx), request)
}

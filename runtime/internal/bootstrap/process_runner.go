package bootstrap

import (
	"context"

	"github.com/Tangerg/frame/process"
	"github.com/Tangerg/frame/process/runner"
)

type processRunner struct{ process.Runner }

func (r processRunner) Launch(ctx context.Context, request runner.LaunchRequest) (runner.LaunchResult, error) {
	if ctx == nil || ctx.Err() != nil {
		return r.Runner.Launch(ctx, request)
	}
	// Frame has durably admitted this creation. Join its original local RPC so
	// cancellation cannot discard the acknowledgement before Frame retires it.
	return r.Runner.Launch(context.WithoutCancel(ctx), request)
}

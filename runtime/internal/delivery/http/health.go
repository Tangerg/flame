package http

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

type HealthStatus string

const (
	HealthOK        HealthStatus = "ok"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
)

// Detail never lands in the response body: an unauthenticated readiness caller
// learns only the status keyword.
type HealthCheck struct {
	Status HealthStatus
	Detail string
}

// Each probe runs at most one invocation at a time, so one that ignores
// cancellation strands one goroutine rather than one per request.
type HealthProbe struct {
	Name  string
	Probe func(ctx context.Context) HealthCheck
}

type healthProbeRunner struct {
	name  string
	probe func(context.Context) HealthCheck

	mu      sync.Mutex
	current *healthProbeInvocation
}

type healthProbeInvocation struct {
	done  chan struct{}
	check HealthCheck
}

func newHealthProbeRunners(probes []HealthProbe) []*healthProbeRunner {
	runners := make([]*healthProbeRunner, len(probes))
	for i, probe := range probes {
		runners[i] = &healthProbeRunner{name: probe.Name, probe: probe.Probe}
	}
	return runners
}

func (h *healthProbeRunner) start(ctx context.Context, budget time.Duration) *healthProbeInvocation {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.current != nil {
		return h.current
	}
	invocation := &healthProbeInvocation{done: make(chan struct{})}
	h.current = invocation
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), budget)
	go h.invoke(probeCtx, cancel, invocation)
	return invocation
}

func (h *healthProbeRunner) invoke(
	ctx context.Context,
	cancel context.CancelFunc,
	invocation *healthProbeInvocation,
) {
	defer cancel()
	result := HealthCheck{}
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.ErrorContext(ctx, "http: health probe panicked",
				telemetry.HealthProbe(h.name), telemetry.Panic(recovered))
			result = HealthCheck{Status: HealthUnhealthy, Detail: "probe panic"}
		}
		invocation.check = result
		h.mu.Lock()
		if h.current == invocation {
			h.current = nil
		}
		h.mu.Unlock()
		close(invocation.done)
	}()
	result = h.probe(ctx)
}

const healthBudget = 2 * time.Second

func runHealthProbes(ctx context.Context, probes []*healthProbeRunner) (HealthStatus, map[string]HealthStatus) {
	return runHealthProbesWithBudget(ctx, probes, healthBudget)
}

func runHealthProbesWithBudget(
	ctx context.Context,
	probes []*healthProbeRunner,
	budget time.Duration,
) (HealthStatus, map[string]HealthStatus) {
	if len(probes) == 0 {
		return HealthOK, nil
	}

	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	type probeResult struct {
		index int
		check HealthCheck
	}
	completed := make(chan probeResult, len(probes))
	results := make([]HealthCheck, len(probes))
	for index := range results {
		results[index] = HealthCheck{
			Status: HealthUnhealthy,
			Detail: "probe did not report within the health budget",
		}
	}
	for i, p := range probes {
		invocation := p.start(ctx, budget)
		go func() {
			select {
			case <-invocation.done:
				completed <- probeResult{index: i, check: invocation.check}
			case <-ctx.Done():
			}
		}()
	}
	for range probes {
		select {
		case result := <-completed:
			results[result.index] = result.check
		case <-ctx.Done():
			goto aggregate
		}
	}

aggregate:
	checks := make(map[string]HealthStatus, len(probes))
	overall := HealthOK
	for i, r := range results {
		status := normalizedHealth(r.Status)
		checks[probes[i].name] = status
		overall = worseHealth(overall, status)
		if status != HealthOK {
			slog.ErrorContext(ctx, "http: health probe is not ok",
				telemetry.HealthProbe(probes[i].name), slog.Any("status", string(status)), slog.Any("detail", r.Detail),
			)
		}
	}
	return overall, checks
}

func normalizedHealth(status HealthStatus) HealthStatus {
	switch status {
	case HealthOK, HealthDegraded, HealthUnhealthy:
		return status
	default:
		return HealthUnhealthy
	}
}

func worseHealth(a, b HealthStatus) HealthStatus {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

func (h HealthStatus) rank() int {
	switch h {
	case HealthUnhealthy:
		return 2
	case HealthDegraded:
		return 1
	default:
		return 0
	}
}

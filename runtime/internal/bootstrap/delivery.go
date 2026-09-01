package bootstrap

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/automation/schedules"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	"github.com/Tangerg/flame/runtime/protocol"
)

// hostApplication is the immutable application capsule owned by Host. Delivery
// receives its own consumer config; startup recovery and workers stay behind
// behavior methods instead of leaking a coordinator locator to Instance.
type hostApplication struct {
	delivery         delivery.HandlerConfig
	sessions         *sessions.Coordinator
	workers          hostWorkers
	idempotencyStore idempotency.Store
}

type hostWorkers struct {
	scheduler     *schedules.Firing
	recovery      *ownership.RecoveryCoordinator
	invalidations invalidation.Publish
}

type workerJoins struct {
	scheduler <-chan struct{}
	recovery  <-chan struct{}
}

func (h *hostApplication) recoverStartup(ctx context.Context) error {
	return h.sessions.RecoverWorkspaceMutations(ctx)
}

func (h *hostApplication) newDeliveryHandler(
	info protocol.ServerInfo,
	idempotencyNamespace string,
) (*delivery.Handler, error) {
	cfg := h.delivery
	cfg.ServerInfo = info
	cfg.IdempotencyLimits = protocol.IdempotencyLimits{
		RetentionSeconds: int(idempotency.Retention.Seconds()),
		Namespace:        idempotencyNamespace,
	}
	return delivery.NewHandler(cfg)
}

func (h *hostApplication) newDeliveryEndpoint(
	lifetime context.Context,
	info protocol.ServerInfo,
	idempotencyNamespace string,
) (*delivery.Endpoint, error) {
	handler, err := h.newDeliveryHandler(info, idempotencyNamespace)
	if err != nil {
		return nil, err
	}
	endpoint, err := delivery.NewEndpoint(handler, delivery.EndpointConfig{
		IdempotencyStore:     h.idempotencyStore,
		IdempotencyNamespace: idempotencyNamespace,
		Lifetime:             lifetime,
	})
	if err != nil {
		return nil, err
	}
	return endpoint, nil
}

func (h *hostApplication) notifyExternalChange() {
	h.workers.invalidations.Notify(invalidation.Notice{Resource: invalidation.Resync})
}

func (h *hostApplication) startWorkers(ctx context.Context) workerJoins {
	schedulerDone := make(chan struct{})
	go func() {
		defer close(schedulerDone)
		h.workers.scheduler.RunWorker(ctx)
	}()
	recoveryDone := make(chan struct{})
	go func() {
		defer close(recoveryDone)
		h.workers.recovery.RunWorker(ctx)
	}()
	return workerJoins{scheduler: schedulerDone, recovery: recoveryDone}
}

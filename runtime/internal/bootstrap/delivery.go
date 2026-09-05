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

// runtimeApplication is the immutable application capsule owned by Instance. Delivery
// receives its own consumer config; startup recovery and worker composition
// remain inside Bootstrap.
type runtimeApplication struct {
	delivery         delivery.HandlerConfig
	sessions         *sessions.Coordinator
	workers          runtimeWorkers
	idempotencyStore idempotency.Store
}

type runtimeWorkers struct {
	scheduler     *schedules.Firing
	recovery      *ownership.RecoveryCoordinator
	invalidations invalidation.Publish
}

type workerJoins struct {
	scheduler <-chan struct{}
	recovery  <-chan struct{}
}

func (h *runtimeApplication) recoverStartup(ctx context.Context) error {
	return h.sessions.RecoverWorkspaceMutations(ctx)
}

func (h *runtimeApplication) newDeliveryHandler(
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

func (h *runtimeApplication) newDeliveryEndpoint(
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

func (h *runtimeApplication) notifyExternalChange() {
	h.workers.invalidations.Notify(invalidation.Notice{Resource: invalidation.Resync})
}

func (h *runtimeApplication) startWorkers(ctx context.Context) workerJoins {
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

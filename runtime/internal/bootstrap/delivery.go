package bootstrap

import (
	"context"
	"net/http"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
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
	callbacks        *pluginsdk.Callbacks
}

type runtimeWorkers struct {
	scheduler     *pluginservices.Dispatcher
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
	rpc, err := flamehttp.NewRPCHandler(endpoint)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	for _, entry := range flamehttp.Contract().Endpoints {
		if entry.Kind == flamehttp.EndpointKindRPC {
			mux.Handle(entry.Method+" "+entry.Path, rpc)
		}
	}
	if err := h.callbacks.PublishRPC(mux); err != nil {
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

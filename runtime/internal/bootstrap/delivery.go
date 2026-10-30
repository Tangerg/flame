package bootstrap

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
)

// runtimeApplication is the immutable application capsule owned by Instance. Delivery
// receives its own consumer config; startup recovery and worker composition
// remain inside Bootstrap.
type runtimeApplication struct {
	pluginsReady     <-chan struct{}
	pluginControl    http.Handler
	delivery         delivery.HandlerConfig
	sessions         *sessions.Coordinator
	workers          runtimeWorkers
	idempotencyStore operation.ReplayStore
	callbacks        *qimumcp.Callbacks
	modelCallbacks   []qimumcp.Callback
	// recoverUncertainExecutions resolves crash-uncertain executions and
	// reconciles abandoned ownership. It queries live plugin sessions, which a
	// background plugin cannot serve until the callback RPC is published, so its
	// owner must invoke it only after publishCallbacks.
	recoverUncertainExecutions func(context.Context) error
}

type runtimeWorkers struct {
	recovery      *ownership.RecoveryCoordinator
	invalidations invalidation.Publish
}

type workerJoins struct {
	recovery <-chan struct{}
}

func (h *runtimeApplication) recoverStartup(ctx context.Context) error {
	if err := h.sessions.RecoverRetiredResources(ctx); err != nil {
		return err
	}
	return h.sessions.RecoverWorkspaceMutations(ctx)
}

func (h *runtimeApplication) newDeliveryHandler(
	info protocol.ServerInfo,
	idempotencyNamespace string,
) (*delivery.Handler, error) {
	cfg := h.delivery
	cfg.ServerInfo = info
	cfg.IdempotencyLimits = protocol.IdempotencyLimits{
		RetentionSeconds: int(operation.ReplayRetention.Seconds()),
		Namespace:        idempotencyNamespace,
	}
	return delivery.NewHandler(cfg)
}

func (h *runtimeApplication) newDeliveryEndpoint(
	lifetime context.Context,
	info protocol.ServerInfo,
	idempotencyNamespace string,
	validateDirectory func() error,
) (*delivery.Endpoint, error) {
	handler, err := h.newDeliveryHandler(info, idempotencyNamespace)
	if err != nil {
		return nil, err
	}
	endpoint, err := delivery.NewEndpoint(handler, delivery.EndpointConfig{
		IdempotencyStore:     h.idempotencyStore,
		IdempotencyNamespace: idempotencyNamespace,
		Lifetime:             lifetime,
		ValidateStorage:      validateDirectory,
	})
	if err != nil {
		return nil, err
	}
	if err := h.publishCallbacks(endpoint); err != nil {
		return nil, err
	}
	return endpoint, nil
}

func (h *runtimeApplication) publishCallbacks(endpoint *delivery.Endpoint) error {
	rpc, err := flamehttp.NewRPCHandler(endpoint)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /v2/plugin-resources/", flamehttp.NewPluginResourceHandler(endpoint))
	for _, entry := range flamehttp.Contract().Endpoints {
		if entry.Kind == flamehttp.EndpointKindRPC {
			mux.Handle(entry.Method+" "+entry.Path, rpc)
		}
	}
	return h.callbacks.PublishRPC(mux, h.modelCallbacks...)
}

func (h *runtimeApplication) startWorkers(ctx context.Context) workerJoins {
	recoveryDone := make(chan struct{})
	go func() {
		defer close(recoveryDone)
		select {
		case <-ctx.Done():
			return
		case <-h.pluginsReady:
		}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if err := h.recoverUncertainExecutions(ctx); err == nil {
				break
			} else if ctx.Err() == nil {
				slog.ErrorContext(ctx, "runtime: original startup recovery is pending", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		h.workers.recovery.RunWorker(ctx)
	}()
	return workerJoins{recovery: recoveryDone}
}

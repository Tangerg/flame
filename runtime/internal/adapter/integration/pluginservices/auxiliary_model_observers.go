package pluginservices

import (
	"context"
	"errors"
	"sync"

	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
)

// Auxiliary freezes observer bindings for the caller's whole use case. Memory
// retains one capture across extraction/curation; Complete releases one request.
func (o *ModelObservers) Auxiliary(resolve models.AuxiliaryResolver) models.AuxiliaryResolver {
	return func(ctx context.Context) (chat.Model, func(), error) {
		if resolve == nil {
			return nil, nil, errors.New("model observers: auxiliary resolver is required")
		}
		model, release, err := resolve(ctx)
		if err != nil {
			return nil, nil, err
		}
		if release == nil {
			return nil, nil, errors.New("model observers: auxiliary model release is required")
		}
		if dependency.Missing(model) {
			release()
			return nil, nil, errors.New("model observers: auxiliary model is required")
		}
		var capture *auxiliaryModelCapture
		err = o.admission.Under(ctx, func() error {
			bindings, err := o.Resolve(ctx)
			if err != nil {
				return err
			}
			bound, _ := o.Bind(model, nil, bindings)
			lifetime, cancel := context.WithCancelCause(ctx)
			capture = &auxiliaryModelCapture{owner: o, bindings: bindings, model: bound, lifetime: lifetime, cancel: cancel, release: release, done: make(chan struct{})}
			o.mu.Lock()
			o.auxiliary[capture] = struct{}{}
			o.mu.Unlock()
			return nil
		})
		if err != nil {
			release()
			return nil, nil, err
		}
		return capture, capture.close, nil
	}
}

func (o *ModelObservers) HeldDependencies() []plugin.Dependency {
	o.mu.Lock()
	defer o.mu.Unlock()
	var values []plugin.Dependency
	for capture := range o.auxiliary {
		for _, binding := range capture.bindings {
			values = append(values, plugin.Dependency{InstallationID: binding.InstallationID, Digest: binding.Digest})
		}
	}
	return plugin.CompactDependencies(values)
}

type auxiliaryModelCapture struct {
	owner    *ModelObservers
	bindings []plugins.ContributionBinding
	model    chat.Model
	lifetime context.Context
	cancel   context.CancelCauseFunc
	release  func()
	mu       sync.Mutex
	closed   bool
	active   sync.WaitGroup
	done     chan struct{}
}

func (c *auxiliaryModelCapture) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil || invocation.Target.Scope != "call" {
		return nil, errors.Join(err, errors.New("model observers: auxiliary call identity is required"))
	}
	c.mu.Lock()
	if c.closed || c.lifetime.Err() != nil {
		c.mu.Unlock()
		return nil, errors.Join(pluginsdk.ErrStaleCall, c.lifetime.Err(), context.Cause(c.lifetime))
	}
	c.active.Add(1)
	c.mu.Unlock()
	defer c.active.Done()
	calling, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(c.lifetime, func() { cancel(context.Cause(c.lifetime)) })
	defer stop()
	defer cancel(nil)
	return c.model.Call(calling, request)
}

func (c *auxiliaryModelCapture) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		<-c.done
		return
	}
	c.closed = true
	c.mu.Unlock()
	c.cancel(pluginsdk.ErrStaleCall)
	c.active.Wait()
	c.release()
	o := c.owner
	o.mu.Lock()
	delete(o.auxiliary, c)
	o.mu.Unlock()
	close(c.done)
}
